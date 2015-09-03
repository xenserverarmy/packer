package vm

import (
	"fmt"

	"github.com/mitchellh/multistep"
	"github.com/mitchellh/packer/packer"
	xsclient "github.com/xenserver/go-xenserver-client"
)

type stepRestoreNetwork struct {
	ScriptUrl	string
}

func (self *stepRestoreNetwork) Run(state multistep.StateBag) multistep.StepAction {

	client := state.Get("client").(xsclient.XenAPIClient)
	ui := state.Get("ui").(packer.Ui)

	ui.Say("Step: Restoring network mapping")


	uuid := state.Get("instance_uuid").(string)
	instance, err := client.GetVMByUuid(uuid)
	if err != nil {
		ui.Error(fmt.Sprintf("Unable to get VM from UUID '%s': %s", uuid, err.Error()))
		return multistep.ActionHalt
	}

	networks := state.Get("original_networks").([]*xsclient.Network)

	ui.Message(fmt.Sprintf("Found %d networks to restore", len(networks)))

	vifs, err := instance.GetVIFs ()
	if err != nil {
		ui.Error(fmt.Sprintf("Unable to obtain the list of VIFs: %s", err.Error()))
		return multistep.ActionHalt
	}

	// connect all vifs to the same temporary network. let's hope vm has no loops
	for i := 0; i < len(networks); i++ {
		network := networks[i]

		if network == nil {
			ui.Message (fmt.Sprintf ("Skipping restore of network %d due to nil pointer", i))
			continue
		}

		err = vifs[i].Destroy()
		if err != nil {
			ui.Error(fmt.Sprintf("Unable to remove interface %d from VM: %s", i, err.Error()))
			return multistep.ActionHalt
		}

		_, err = instance.ConnectNetwork(network, fmt.Sprintf("%d", i))

		if err != nil {
			ui.Error(fmt.Sprintf("Unable to restore interface %d: %s", i, err.Error()))
			return multistep.ActionHalt
		}
	}

	return multistep.ActionContinue
}

func (self *stepRestoreNetwork) Cleanup(state multistep.StateBag) {
}
