package common

import (
//	"crypto/tls"
	"fmt"
	"github.com/mitchellh/multistep"
	"github.com/mitchellh/packer/packer"
//	"io"
//	"net/http"
	"os"
	"os/exec"
)

type StepPrepareNfsExport struct {
	NfsMount 	string
}



func (self *StepPrepareNfsExport) Run(state multistep.StateBag) multistep.StepAction {
	config := state.Get("commonconfig").(CommonConfig)
	ui := state.Get("ui").(packer.Ui)
	ui.Say("Step: Mounting NFS export")

	NfsMountPoint := config.OutputDir + "/nfs"

	if _, err := os.Stat(NfsMountPoint); err == nil {
		ui.Say("Deleting previous mount point directory...")
		os.RemoveAll(NfsMountPoint)
	}

	if err := os.MkdirAll(NfsMountPoint, 0755); err != nil {
		state.Put("error", err)
		return multistep.ActionHalt
	}


	cmd := exec.Command ("mount", self.NfsMount, NfsMountPoint  )
	if err := cmd.Start(); err != nil {
		ui.Error(fmt.Sprintf("Unable to mount NFS SR '%s' locally: %s", self.NfsMount, err.Error()))
		return multistep.ActionHalt
	}

	if err := cmd.Wait(); err != nil {
		ui.Error(fmt.Sprintf("Unable to mount NFS SR '%s' locally: %s", self.NfsMount, err.Error()))
		return multistep.ActionHalt
	}


	return multistep.ActionContinue
}

func (StepPrepareNfsExport) Cleanup(state multistep.StateBag) {
	config := state.Get("commonconfig").(CommonConfig)
	ui := state.Get("ui").(packer.Ui)

	ui.Say("Deleting mount point ...")
	NfsMountPoint := config.OutputDir + "/nfs"

	cmd := exec.Command ("umount", NfsMountPoint  )
	if err := cmd.Start(); err != nil {
		ui.Error(fmt.Sprintf("Unable to unmount NFS SR '%s' locally: %s", NfsMountPoint, err.Error()))
		return
	}

	if err := cmd.Wait(); err != nil {
		ui.Error(fmt.Sprintf("Unable to unmount NFS SR '%s' locally: %s", NfsMountPoint, err.Error()))
		return
	}

	os.RemoveAll(NfsMountPoint)

}
