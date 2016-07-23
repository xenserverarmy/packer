package common

import (
	"crypto/tls"
	"fmt"
	"github.com/mitchellh/multistep"
	"github.com/mitchellh/packer/packer"
	xsclient "github.com/xenserver/go-xenserver-client"
	"io"
	"net/http"
	"os"
)

type StepExport struct {
	OutputFormat string
}

func downloadFile(url, filename string, ui packer.Ui) (err error) {

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

	var progress uint
	var total uint
	var percentage uint
	var marker_len uint

	progress = uint(0)
	total = uint(resp.ContentLength)
	percentage = uint(0)
	marker_len = uint(5)

	var buffer [4096]byte
	for {
		n, err := resp.Body.Read(buffer[:])
		if err != nil && err != io.EOF {
			return err
		}

		progress += uint(n)

		if _, write_err := fh.Write(buffer[:n]); write_err != nil {
			return write_err
		}

		if err == io.EOF {
			break
		}

		// Increment percentage in multiples of marker_len
		cur_percentage := ((progress * 100 / total) / marker_len) * marker_len
		if cur_percentage > percentage {
			percentage = cur_percentage
			ui.Message(fmt.Sprintf("Downloading... %d%%", percentage))
		}

	}

	return nil
}

func (self *StepExport) Run(state multistep.StateBag) multistep.StepAction {
	config := state.Get("commonconfig").(CommonConfig)
	ui := state.Get("ui").(packer.Ui)
	client := state.Get("client").(xsclient.XenAPIClient)
	instance_uuid := state.Get("instance_uuid").(string)
	suffix := ".vhd"
	extrauri := "&format=vhd"

	exportFiles := make([]string, 0, 1)

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
		// Before exporting, convert it to a template (if requested)
		// If a template, we assume all VIFs should be removed at export time,
		// to make it more generic and reusable
		if config.ConvertToTemplate {
			ui.Say("Converting VM to template before export")
			err = instance.SetIsATemplate(true)
			if err != nil {
				ui.Error(fmt.Sprintf("Error converting VM to a template prior to export: %s", err.Error()))
				return multistep.ActionHalt
			}
			vifs, _ := instance.GetVIFs()
			for _, vif := range vifs {
				ui.Say("Destroying VM network interfaces for template export")
				vif.Destroy()
			}
		}

		disks, err := instance.GetDisks()
		if err != nil {
			ui.Error(fmt.Sprintf("Could not get VM disks: %s", err.Error()))
			return multistep.ActionHalt
		}

		for i, disk := range disks {
			disk_uuid, err := disk.GetUuid()
			if err != nil {
				ui.Error(fmt.Sprintf("Could not get disk %d with UUID '%s': %s", i, disk_uuid, err.Error()))
				return multistep.ActionHalt
			}

			export_filename := fmt.Sprintf("%s/%s.%d.vhd", config.OutputDir, config.VMName, i)
			source_filename := fmt.Sprintf("%s/nfs/%s/%s.vhd", config.OutputDir, state.Get("instance_sr_uuid").(string), disk_uuid)

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

			exportFiles = append(exportFiles, export_filename)

			dst.Close()
		}

	case "xva":
		// Before exporting, convert it to a template (if requested)
		// If a template, we assume all VIFs should be removed at export time,
		// to make it more generic and reusable
		if config.ConvertToTemplate {
			ui.Say("Converting VM to template before export")
			err = instance.SetIsATemplate(true)
			if err != nil {
				ui.Error(fmt.Sprintf("Error converting VM to a template prior to export: %s", err.Error()))
				return multistep.ActionHalt
			}
			vifs, _ := instance.GetVIFs()
			for _, vif := range vifs {
				ui.Say("Destroying VM network interfaces for template export")
				vif.Destroy()
			}
		}
		export_url := fmt.Sprintf("https://%s/export?uuid=%s&session_id=%s",
			client.Host,
			instance_uuid,
			client.Session.(string),
		)

		export_filename := fmt.Sprintf("%s/%s.xva", config.OutputDir, config.VMName)

		ui.Say("Getting XVA " + export_url)
		err = downloadFile(export_url, export_filename, ui)
		if err != nil {
			ui.Error(fmt.Sprintf("Could not download XVA: %s", err.Error()))
			return multistep.ActionHalt
		}

		exportFiles = append(exportFiles, export_filename)

	case "vdi_raw":
		suffix = ".raw"
		extrauri = ""
		fallthrough
	case "vdi_vhd":
		// Before exporting, convert it to a template (if requested)
		// If a template, we assume all VIFs should be removed at export time,
		// to make it more generic and reusable
		if config.ConvertToTemplate {
			ui.Say("Converting VM to template before export")
			err = instance.SetIsATemplate(true)
			if err != nil {
				ui.Error(fmt.Sprintf("Error converting VM to a template prior to export: %s", err.Error()))
				return multistep.ActionHalt
			}
			vifs, _ := instance.GetVIFs()
			for _, vif := range vifs {
				ui.Say("Destroying VM network interfaces for template export")
				vif.Destroy()
			}
		}
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

			// Work out XenServer version
			hosts, err := client.GetHosts()

			if err != nil {
				ui.Error(fmt.Sprintf("Could not retrieve hosts in the pool: %s", err.Error()))
				return multistep.ActionHalt
			}
			host := hosts[0]
			host_software_versions, err := host.GetSoftwareVersion()
			xs_version := host_software_versions["product_version"].(string)

			if err != nil {
				ui.Error(fmt.Sprintf("Could not get the software version: %s", err.Error()))
				return multistep.ActionHalt
			}

			var disk_export_url string

			// @todo: check for 6.5 SP1
			if xs_version <= "6.5.0" && config.Format == "vdi_vhd" {
				// Export the VHD using a Transfer VM

				disk_export_url, err = disk.Expose("vhd")

				if err != nil {
					ui.Error(fmt.Sprintf("Failed to expose disk %s: %s", disk_uuid, err.Error()))
					return multistep.ActionHalt
				}

			} else {

				// Use the preferred direct export from XAPI
				// Basic auth in URL request is required as session token is not
				// accepted for some reason.
				// @todo: raise with XAPI team.
				disk_export_url = fmt.Sprintf("https://%s:%s@%s/export_raw_vdi?vdi=%s%s",
					client.Username,
					client.Password,
					client.Host,
					disk_uuid,
					extrauri)

			}

			disk_export_filename := fmt.Sprintf("%s/%s%s", config.OutputDir, disk_uuid, suffix)

			ui.Say("Getting VDI " + disk_export_url)
			err = downloadFile(disk_export_url, disk_export_filename, ui)
			if err != nil {
				ui.Error(fmt.Sprintf("Could not download VDI: %s", err.Error()))
				return multistep.ActionHalt
			}

			// Call unexpose in case a TVM was used. The call is harmless
			// if that is not the case.
			disk.Unexpose()
		}

	default:
		panic(fmt.Sprintf("Unknown export format '%s'", self.OutputFormat))
	}

	state.Put("export_files", exportFiles)

	ui.Say("Download completed: " + config.OutputDir)

	return multistep.ActionContinue
}

func (StepExport) Cleanup(state multistep.StateBag) {}
