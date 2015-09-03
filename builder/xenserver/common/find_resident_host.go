package common

import (
	"fmt"
	"github.com/mitchellh/multistep"
	"github.com/mitchellh/packer/packer"
	xsclient "github.com/xenserver/go-xenserver-client"
)

func FindResidentHost (state multistep.StateBag, instance *xsclient.VM, uuid string) (err error) {

	ui := state.Get("ui").(packer.Ui)

	domid, err := instance.GetDomainId()
	if err != nil {
		ui.Error(fmt.Sprintf("Unable to get domid of VM with UUID '%s': %s", uuid, err.Error()))
		return err
	}
	state.Put("domid", domid)

	// we are connected to a given host, but that might not be where the VM is running
	host, err := instance.GetResidentOn()
	if err != nil {
		ui.Error(fmt.Sprintf("Unable to determine what host the VM is running on: %s", err.Error()))
		return err
	}

	hostAddress, err := host.GetAddress()
	if err != nil {
		ui.Error(fmt.Sprintf("Unable to determine the IP of the host: %s", err.Error()))
		return err
	}


	ui.Message(fmt.Sprintf("VM '%s' is running on host with address '%s'", uuid, hostAddress))

	config := state.Get("commonconfig").(CommonConfig)
	config.HostIp = hostAddress
	state.Put("commonconfig", config)

	return nil

}

