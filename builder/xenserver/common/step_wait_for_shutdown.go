package common

import (
	"fmt"
	"github.com/mitchellh/multistep"
	"github.com/mitchellh/packer/packer"
	"time"

	xsclient "github.com/xenserverarmy/go-xenserver-client"
)

type StepWaitForShutdown struct{}

func (StepWaitForShutdown) Run(state multistep.StateBag) multistep.StepAction {
	//config := state.Get("commonconfig").(CommonConfig)
	ui := state.Get("ui").(packer.Ui)
	client := state.Get("client").(xsclient.XenAPIClient)
	instance_uuid := state.Get("instance_uuid").(string)

	instance, err := client.GetVMByUuid(instance_uuid)
	if err != nil {
		ui.Error(fmt.Sprintf("Could not get VM with UUID '%s': %s", instance_uuid, err.Error()))
		return multistep.ActionHalt
	}

	ui.Say("Step: Waiting for installer to shutdown VM")


	func() {
		err = InterruptibleWait{
			Predicate: func() (bool, error) {
				power_state, err := instance.GetPowerState()
				return power_state == "Halted", err
			},
			PredicateInterval: 20 * time.Second,
			Timeout:           3000 * time.Second,
		}.Wait(state)

		if err != nil {
			ui.Error(fmt.Sprintf("Error waiting for VM installer to halt: %s", err.Error()))
		}

	}()

	return multistep.ActionContinue
}

func (StepWaitForShutdown) Cleanup(state multistep.StateBag) {}
