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

func vm_boot_volume_path(name string) string {
	return "/dev/zvol/" + dataset + "/" + name + "/" + "disk0"
}

func vm_dataset(name string) string {
	return dataset + "/" + name
}

func get_vm_property(name string, property string) (string, error) {
	vm_dataset := vm_dataset(name)
	fs, _ := zfs.GetDataset(vm_dataset)
	property, err := fs.GetProperty("runhyve:vm:" + property)

	if err != nil {
		return "", err
	}

	return property, nil
}
func vm_load(name string) (*Machine, error) {
	vm_name, err := get_vm_property(name, "name")

	if err != nil {
		return nil, fmt.Errorf("couldn't load VM %s configuration", name)
	}

	ncpu, _ := get_vm_property(name, "ncpu")
	memory, _ := get_vm_property(name, "memory")
	loader, _ := get_vm_property(name, "loader")
	uuid, _ := get_vm_property(name, "uuid")
	autostart, _ := get_vm_property(name, "autostart")

	return &Machine{
		name:      vm_name,
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
		vm_name, err := fs.GetProperty("runhyve:vm:name")

		if err == nil && vm_name != "-" {
			ncpu, _ := fs.GetProperty("runhyve:vm:ncpu")
			memory, _ := fs.GetProperty("runhyve:vm:memory")
			loader, _ := fs.GetProperty("runhyve:vm:loader")
			fmt.Printf("%s %s %s %s\n", vm_name, ncpu, memory, loader)
		}
	}

	return datasets, nil
}

func create(name string, volume_size uint64, ncpu int, memory int) (*Machine, error) {
	vm_dataset := dataset + "/" + name
	vol_name := vm_boot_volume_path(name)

	zfs_props := map[string]string{
		"runhyve:vm:name":          name,
		"runhyve:vm:uuid":          uuid.NewString(),
		"runhyve:vm:loader":        "bhyveload",
		"runhyve:vm:autostart":     "on",
		"runhyve:vm:ncpu":          string(ncpu),
		"runhyve:vm:memory":        string(memory),
		"runhyve:vm:volumes:disk0": strconv.FormatUint(volume_size, 10),
	}

	_, err := zfs.CreateFilesystem(vm_dataset, zfs_props)

	if err != nil {
		return nil, fmt.Errorf("couldn't create dataset %s: %s", vm_dataset, err)
	}
	// TODO: create sparse volume, seems it is not supported by go-zfs: https://github.com/mistifyio/go-zfs/issues/77
	_, err = zfs.CreateVolume(vol_name, volume_size, map[string]string{"volmode": "dev"})

	if err != nil {
		return nil, fmt.Errorf("couldn't create volume %s: %s", vol_name, err)
	}

	volume_path := "/dev/zvol/" + vol_name
	image_path := "/home/kwiat/Development/runhyve/runhyve-cli/debian-10-openstack-amd64.qcow2"

	err = write_image(image_path, volume_path)

	if err != nil {
		return nil, fmt.Errorf("couldn't write volume %s with image %s: %s", image_path, volume_path, err)
	}

	machine, err := vm_load(name)
	fmt.Printf("[Info] Created volume for VM %s\n", name)

	return machine, nil
}

func start(name string) error {
	machine, err := vm_load(name)

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
		"-s", "4:0,virtio-blk," + vm_boot_volume_path(name),
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

func write_image(image_path string, volume_path string) error {
	fmt.Printf("[Info] Writing image %s to volume %s... \n", image_path, volume_path)
	command := "qemu-img"
	cmd := exec.Command(command, "dd", "-O", "raw", "if="+image_path, "of="+volume_path, "bs=1M")
	stdout, err := cmd.CombinedOutput()

	if err != nil {
		fmt.Println(stdout)
		return err
	}
	return nil
}

func destroy(name string) error {
	vm_dataset := dataset + "/" + name

	ds, err := zfs.GetDataset(vm_dataset)

	if err != nil {
		return fmt.Errorf("couln't open dataset %s: %s", vm_dataset, err)
	}

	fmt.Printf("[Info] Destroying VM %s\n", name)

	return ds.Destroy(zfs.DestroyRecursive)
}
