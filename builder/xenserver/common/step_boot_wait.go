package common

import (
	"fmt"
	"strings"
	"github.com/mitchellh/multistep"
	"github.com/mitchellh/packer/packer"
	xsclient "github.com/xenserver/go-xenserver-client"
)

type StepBootWait struct{}

func (self *StepBootWait) Run(state multistep.StateBag) multistep.StepAction {
	client := state.Get("client").(xsclient.XenAPIClient)
	config := state.Get("commonconfig").(CommonConfig)
	ui := state.Get("ui").(packer.Ui)

	instance, _ := client.GetVMByUuid(state.Get("instance_uuid").(string))
	
	powerState, _ := instance.GetPowerState()
	switch strings.ToLower(powerState) {
		case "halted":
			ui.Say("Starting VM " + state.Get("instance_uuid").(string))
			instance.Start(false, false)

		case "paused":
			ui.Say("Unpaused VM " + state.Get("instance_uuid").(string))
			instance.Unpause()
		
		case "suspended":
			ui.Say("Resuming VM " + state.Get("instance_uuid").(string))
			instance.Resume(false, false)	

/*		case "running":
			ui.Error("VM " + state.Get("instance_uuid").(string) + " is already running")
			instance.Unpause()			
*/

	}

	if int64(config.BootWait) > 0 {
		ui.Say(fmt.Sprintf("Waiting %s for boot...", config.BootWait))
		err := InterruptibleWait{Timeout: config.BootWait}.Wait(state)
		if err != nil {
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
	}
	return multistep.ActionContinue
}

func (self *StepBootWait) Cleanup(state multistep.StateBag) {}
