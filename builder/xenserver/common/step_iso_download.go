package common

import (
	"fmt"
	"github.com/mitchellh/multistep"
	"github.com/mitchellh/packer/packer"

	xsclient "github.com/xenserverarmy/go-xenserver-client"
)

type StepIsoDownload struct {
	IsoName	string
	SrName		string
	DlUrl		string
	ScriptUrl	string
	
}

func (self *StepIsoDownload) Run(state multistep.StateBag) multistep.StepAction {
	config := state.Get("commonconfig").(CommonConfig)
	ui := state.Get("ui").(packer.Ui)
	client := state.Get("client").(xsclient.XenAPIClient)

	ui.Say("Downloading ISO " + self.IsoName)
	// first step is to find out if the ISO already exists in the SR
	vdis, _ := client.GetVdiByNameLabel(self.IsoName)

	switch {
	case len(vdis) == 0:
		// this is what we expect to find, now check for the ISO SR since we'll be loading into that
		srs, _ := client.GetSRByNameLabel(self.SrName)

		switch {
		case len(srs) == 0:
			ui.Error(fmt.Sprintf("Unable to find SR with name '%s'.", self.SrName))
			return multistep.ActionHalt

		case len(srs) > 1:
			ui.Error(fmt.Sprintf("Found more than one SR with name '%s'. Name must be unique", self.SrName))
			return multistep.ActionHalt
		}
	case len(vdis) > 1:
		ui.Error(fmt.Sprintf("Found more than one ISO with name '%s'. Name must be unique", self.IsoName))
		return multistep.ActionHalt

	case len(vdis) == 1:
		// we have the ISO already in place.
		ui.Say("ISO already in ISO library")
		return multistep.ActionContinue 
	}

	if self.DlUrl == "" {
		ui.Error(fmt.Sprintf("ISO '%s' not in SR, but no download URL specified. Aborting.", self.IsoName))
		return multistep.ActionHalt		
	}

	// we get here because the ISO doesn't exist, but the SR does
	// time to download it, but we do that via SSH into the host
	
	cmds := []string {
		"rm -f ./copyiso.sh",
		fmt.Sprintf ("wget %scopyiso.sh", self.ScriptUrl),
		"chmod +x ./copyiso.sh",
		fmt.Sprintf ( "./copyiso.sh '%s' '%s' '%s' ", self.SrName, self.IsoName, self.DlUrl ),
		"rm -f ./copyiso.sh" }

	_, err := ExecuteHostSSHCmds (state, cmds )
	if err != nil {
		ui.Error(fmt.Sprintf("Error running scripts on XenServer host '%s'.", err))
		return multistep.ActionHalt
	}
	


	ui.Say("Download completed: " + config.OutputDir)

	return multistep.ActionContinue 
}

func (StepIsoDownload) Cleanup(state multistep.StateBag) {}
