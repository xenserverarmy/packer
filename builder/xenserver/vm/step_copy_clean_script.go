package vm

import (
	"fmt"

	"github.com/mitchellh/multistep"
	"github.com/mitchellh/packer/packer"
	xscommon "github.com/xenserverarmy/packer/builder/xenserver/common"
)

type stepCopyCleanScript struct {
	ScriptUrl	string
}

func (self *stepCopyCleanScript) Run(state multistep.StateBag) multistep.StepAction {

	ui := state.Get("ui").(packer.Ui)

	ui.Say("Step: Copy clean script")

	cmds := []string {
		"rm -f ./packer-clean.sh",
		"rm -f /opt/xensource/www/packer-clean.sh",
		fmt.Sprintf ("wget %spacker-clean.sh", self.ScriptUrl),
		"cp ./packer-clean.sh /opt/xensource/www/packer-clean.sh",
		"rm -f ./packer-clean.sh",
	}

	_, err := xscommon.ExecuteHostSSHCmds (state, cmds )
	if err != nil {
		ui.Error(fmt.Sprintf("Error saving script on XenServer host '%s'.", err))
		return multistep.ActionHalt
	}
	
	ui.Say("Script copy complete")

	return multistep.ActionContinue 

}

func (self *stepCopyCleanScript) Cleanup(state multistep.StateBag) {

	ui := state.Get("ui").(packer.Ui)

	ui.Say("Step: Cleanup copy script")

	cmds := []string {
		"rm -f ./packer-clean.sh",
		"rm -f /opt/xensource/www/packer-clean.sh",
	}

	_, err := xscommon.ExecuteHostSSHCmds (state, cmds )
	if err != nil {
		ui.Error(fmt.Sprintf("Error removing script on XenServer host '%s'.", err))
		return 
	}
	
	ui.Say("Script removal complete")

	return 
}

