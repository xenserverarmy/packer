package common

import (
	"crypto/tls"
	"fmt"
	"github.com/mitchellh/multistep"
	"github.com/mitchellh/packer/packer"
	"io"
	"net/http"
	"os"
)

type StepExport struct {
	OutputFormat string
}

func downloadFile(url, filename string) (err error) {

	// Create the file
	fh, err := os.Create(filename)
	if err != nil {
		return err
	}

	// Define a new transport which allows self-signed certs
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	// Create a client
	client := &http.Client{Transport: tr}

	// Create request and download file

	resp, err := client.Get(url)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	io.Copy(fh, resp.Body)

	return nil
}

func (self *StepExport) Run(state multistep.StateBag) multistep.StepAction {
	config := state.Get("commonconfig").(CommonConfig)
	ui := state.Get("ui").(packer.Ui)
	client := state.Get("client").(XenAPIClient)
	instance_uuid := state.Get("instance_uuid").(string)

	instance, err := client.GetVMByUuid(instance_uuid)
	if err != nil {
		ui.Error(fmt.Sprintf("Could not get VM with UUID '%s': %s", instance_uuid, err.Error()))
		return multistep.ActionHalt
	}

	ui.Say("Step: export artifact")

	if self.OutputFormat == "" {
		self.OutputFormat = config.Format
	}

	switch self.OutputFormat {
	case "none":
		ui.Say("Skipping export")
		return multistep.ActionContinue

	case "vhd":
		export_filename := fmt.Sprintf("%s/%s.vhd", config.OutputDir, config.VMName)
		source_filename := fmt.Sprintf("%s/nfs/%s/%s.vhd", config.OutputDir, state.Get("instance_sr_uuid").(string), state.Get("instance_vdi_uuid").(string))

		ui.Say("Getting VHD " + source_filename)

		// VM is stopped -- no writing issues on the VM unless someone starts it ;)
		src, err := os.Open(source_filename)
		if err != nil {
			ui.Error(fmt.Sprintf("Could not open source VHD: %s", err.Error()))
			return multistep.ActionHalt
		}

		defer src.Close() // make certain we don't lock
		dst, err := os.Create(export_filename)
		if err != nil {
			ui.Error(fmt.Sprintf("Could not create destination VHD: %s", err.Error()))
			return multistep.ActionHalt
		}
		if _, err := io.Copy(dst, src); err != nil {
			dst.Close()
			ui.Error(fmt.Sprintf("Error copying VHD: %s", err.Error()))
			return multistep.ActionHalt
		}
		
		dst.Close()

		return multistep.ActionContinue

	case "xva":
		// export the VM

		export_url := fmt.Sprintf("https://%s/export?uuid=%s&session_id=%s",
			client.Host,
			instance_uuid,
			client.Session.(string),
		)

		export_filename := fmt.Sprintf("%s/%s.xva", config.OutputDir, config.VMName)

		ui.Say("Getting XVA " + export_url)
		err = downloadFile(export_url, export_filename)
		if err != nil {
			ui.Error(fmt.Sprintf("Could not download XVA: %s", err.Error()))
			return multistep.ActionHalt
		}

	case "vdi_raw":
		// export the disks

		disks, err := instance.GetDisks()
		if err != nil {
			ui.Error(fmt.Sprintf("Could not get VM disks: %s", err.Error()))
			return multistep.ActionHalt
		}
		for _, disk := range disks {
			disk_uuid, err := disk.GetUuid()
			if err != nil {
				ui.Error(fmt.Sprintf("Could not get disk with UUID '%s': %s", disk_uuid, err.Error()))
				return multistep.ActionHalt
			}

			// Basic auth in URL request is required as session token is not
			// accepted for some reason.
			// @todo: raise with XAPI team.
			disk_export_url := fmt.Sprintf("https://%s:%s@%s/export_raw_vdi?vdi=%s",
				client.Username,
				client.Password,
				client.Host,
				disk_uuid,
			)

			disk_export_filename := fmt.Sprintf("%s/%s.raw", config.OutputDir, disk_uuid)

			ui.Say("Getting VDI " + disk_export_url)
			err = downloadFile(disk_export_url, disk_export_filename)
			if err != nil {
				ui.Error(fmt.Sprintf("Could not download VDI: %s", err.Error()))
				return multistep.ActionHalt
			}
		}

	default:
		panic(fmt.Sprintf("Unknown export format '%s'", self.OutputFormat ))
	}

	ui.Say("Download completed: " + config.OutputDir)

	return multistep.ActionContinue
}

func (StepExport) Cleanup(state multistep.StateBag) {}
