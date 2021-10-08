package main

import (
	"fmt"
	"strconv"

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

	vms := [6]string{"master-1", "master-2", "master-3", "node-1", "node-2", "node-3"}

	for _, vm := range vms {
		create(vm, 100)

		if err != nil {
			fmt.Printf("[Error] Couldn't create VM %s: %s\n", vm, err)
		}
	}

	list()

	for _, vm := range vms {
		destroy(vm)

		if err != nil {
			fmt.Printf("[Error] Couldn't destroy VM %s: %s\n", vm, err)
		}
	}
}

type Machine struct {
	name      string // name of VM
	ncpu      int    // number of VM vCPUs
	memory    int    // memory in MB
	loader    string // VM loader eg. bhyveload
	uuid      string
	autostart string
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

func create(name string, volume_size uint64) (*Machine, error) {
	vm_dataset := dataset + "/" + name
	vol_name := dataset + "/" + name + "/" + "disk0"

	machine := &Machine{name: name, ncpu: 1, memory: 1024, loader: "bhyveload", uuid: uuid.NewString()}

	zfs_props := map[string]string{
		"runhyve:vm:name":          machine.name,
		"runhyve:vm:uuid":          machine.uuid,
		"runhyve:vm:loader":        machine.loader,
		"runhyve:vm:autostart":     machine.autostart,
		"runhyve:vm:ncpu":          strconv.Itoa(machine.ncpu),
		"runhyve:vm:memory":        strconv.Itoa(machine.memory),
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

	fmt.Printf("[Info] Created volume for VM %s\n", name)

	return machine, nil
}

func write_image(image_path string, volume_path string) error {
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
