package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/google/uuid"

	zfs "github.com/mistifyio/go-zfs"
)

const (
	dataset = "zroot/vm"
)

func main() {
	fmt.Println("go-bhyve v0.0.1")

	_, err := zfs.GetDataset(dataset)

	if err != nil {
		fmt.Printf("[Error] Coulnd't get ZFS dataset: %s", err)
	}

	//vms := [6]string{"master-1", "master-2", "master-3", "node-1", "node-2", "node-3"}
	vms := [1]string{"master-1"}

	/*for _, vm := range vms {
		_, err := create(vm, 1024*1024*1024*10, 1, 1024) //10GB disk, 1 vCPU, 1024MB RAM

		if err != nil {
			fmt.Printf("[Error] Couldn't create VM %s: %s\n", vm, err)
		}
	}*/

	list()
	for _, vm := range vms {
		err := start(vm)

		if err != nil {
			fmt.Printf("[Error] Couldn't create VM %s: %s\n", vm, err)
		}
	}

	/*for _, vm := range vms {
		destroy(vm)

		if err != nil {
			fmt.Printf("[Error] Couldn't destroy VM %s: %s\n", vm, err)
		}
	}*/
}

type Machine struct {
	name      string // name of VM
	ncpu      string // number of VM vCPUs
	memory    string // memory in MB
	loader    string // VM loader eg. bhyveload
	uuid      string
	autostart string
}

func vmBootVolumePath(name string) string {
	return "/dev/zvol/" + dataset + "/" + name + "/" + "disk0"
}

func vmDataset(name string) string {
	return dataset + "/" + name
}

func getVmProperty(name string, property string) (string, error) {
	vmDataset := vmDataset(name)
	fs, _ := zfs.GetDataset(vmDataset)
	property, err := fs.GetProperty("runhyve:vm:" + property)

	if err != nil {
		return "", err
	}

	return property, nil
}
func vmLoad(name string) (*Machine, error) {
	vmName, err := getVmProperty(name, "name")

	if err != nil {
		return nil, fmt.Errorf("couldn't load VM %s configuration", name)
	}

	ncpu, _ := getVmProperty(name, "ncpu")
	memory, _ := getVmProperty(name, "memory")
	loader, _ := getVmProperty(name, "loader")
	uuid, _ := getVmProperty(name, "uuid")
	autostart, _ := getVmProperty(name, "autostart")

	return &Machine{
		name:      vmName,
		ncpu:      ncpu,
		memory:    memory,
		loader:    loader,
		uuid:      uuid,
		autostart: autostart,
	}, nil
}

func list() ([]*zfs.Dataset, error) {
	datasets, err := zfs.Filesystems(dataset)
	if err != nil {
		return nil, fmt.Errorf("couldn't get list of datasets: %s", err)
	}

	fmt.Println("NAME CPUs MEMORY LOADER")
	for _, fs := range datasets {
		vmName, err := fs.GetProperty("runhyve:vm:name")

		if err == nil && vmName != "-" {
			ncpu, _ := fs.GetProperty("runhyve:vm:ncpu")
			memory, _ := fs.GetProperty("runhyve:vm:memory")
			loader, _ := fs.GetProperty("runhyve:vm:loader")
			fmt.Printf("%s %s %s %s\n", vmName, ncpu, memory, loader)
		}
	}

	return datasets, nil
}

func create(name string, volumeSize uint64, ncpu int, memory int) (*Machine, error) {
	vmDataset := dataset + "/" + name
	volName := vmBootVolumePath(name)

	zfsProps := map[string]string{
		"runhyve:vm:name":          name,
		"runhyve:vm:uuid":          uuid.NewString(),
		"runhyve:vm:loader":        "bhyveload",
		"runhyve:vm:autostart":     "on",
		"runhyve:vm:ncpu":          string(ncpu),
		"runhyve:vm:memory":        string(memory),
		"runhyve:vm:volumes:disk0": strconv.FormatUint(volumeSize, 10),
	}

	_, err := zfs.CreateFilesystem(vmDataset, zfsProps)

	if err != nil {
		return nil, fmt.Errorf("couldn't create dataset %s: %s", vmDataset, err)
	}
	// TODO: create sparse volume, seems it is not supported by go-zfs: https://github.com/mistifyio/go-zfs/issues/77
	_, err = zfs.CreateVolume(volName, volumeSize, map[string]string{"volmode": "dev"})

	if err != nil {
		return nil, fmt.Errorf("couldn't create volume %s: %s", volName, err)
	}

	volumePath := "/dev/zvol/" + volName
	imagePath := "/home/kwiat/Development/runhyve/runhyve-cli/debian-10-openstack-amd64.qcow2"

	err = writeImage(imagePath, volumePath)

	if err != nil {
		return nil, fmt.Errorf("couldn't write volume %s with image %s: %s", imagePath, volumePath, err)
	}

	machine, err := vmLoad(name)
	fmt.Printf("[Info] Created volume for VM %s\n", name)

	return machine, nil
}

func start(name string) error {
	machine, err := vmLoad(name)

	if err != nil {
		return err
	}
	command := "bhyve"
	//Jun 24 17:45:51:  [bhyve options: -c 1 -m 1024M -AHP -U d622261d-05ce-4f5d-aeb2-d9e19bbf731e -u]
	//Jun 24 17:45:51:  [bhyve devices: -s 0,hostbridge -s 31,lpc -s 4:0,virtio-blk,/dev/zvol/zroot/vm/10_hermes/disk0 -s 4:1,ahci-cd,/vm/10_hermes/seed.iso -s 5:0,e1000,tap5,mac=58:9c:fc:01:e2:2f]
	//Jun 24 17:45:51:  [bhyve console: -l com1,/dev/nmdm-10_hermes.1A]

	// /usr/local/sbin/grub-bhyve -c /dev/nmdm-10_hermes.1A -m /vm/10_hermes/device.map -M 1024M -r hd0,1 -d /grub 10_hermes

	args := []string{"-c", machine.ncpu,
		"-m", machine.memory + "M",
		"-AHP", "-u",
		"-U", machine.uuid,
		"-s", "0,hostbridge",
		"-s", "31,lpc",
		"-s", "4:0,virtio-blk," + vmBootVolumePath(name),
		"-l", "com1,/dev/nmdm-" + name + ".1A",
		name}
	fmt.Printf("[Info] Starting vm %s: %s %s\n", name, command, strings.Join(args, " "))
	cmd := exec.Command(command, args...)
	stdout, err := cmd.CombinedOutput()

	if err != nil {
		fmt.Println(string(stdout))
		return err
	}

	return nil
}

func writeImage(imagePath string, volumePath string) error {
	fmt.Printf("[Info] Writing image %s to volume %s... \n", imagePath, volumePath)
	command := "qemu-img"
	cmd := exec.Command(command, "dd", "-O", "raw", "if="+imagePath, "of="+volumePath, "bs=1M")
	stdout, err := cmd.CombinedOutput()

	if err != nil {
		fmt.Println(stdout)
		return err
	}
	return nil
}

func destroy(name string) error {
	vmDataset := dataset + "/" + name

	ds, err := zfs.GetDataset(vmDataset)

	if err != nil {
		return fmt.Errorf("couln't open dataset %s: %s", vmDataset, err)
	}

	fmt.Printf("[Info] Destroying VM %s\n", name)

	return ds.Destroy(zfs.DestroyRecursive)
}
