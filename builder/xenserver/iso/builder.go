package iso

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/mitchellh/multistep"
	"github.com/mitchellh/packer/common"
	"github.com/mitchellh/packer/helper/communicator"
	hconfig "github.com/mitchellh/packer/helper/config"
	"github.com/mitchellh/packer/packer"
	"github.com/mitchellh/packer/template/interpolate"
	xsclient "github.com/xenserver/go-xenserver-client"
	xscommon "github.com/xenserverarmy/packer/builder/xenserver/common"
)

type config struct {
	common.PackerConfig   `mapstructure:",squash"`
	xscommon.CommonConfig `mapstructure:",squash"`

	VMMemory uint `mapstructure:"vm_memory"`
	VMVCpus  uint `mapstructure:"vm_vcpus"`
	// VMDisks uses a nested slice to enforce strict ordering of disk creation.
	// This can be important for matching disk sizes to device names in Kickstart scripts,
	// for example. maps make for a slightly prettier config syntax, but have a
	// random iteration order which is not desirable here.
	VMDisks       [][]string `mapstructure:"vm_disks"`
	DiskSize      uint       `mapstructure:"disk_size"`
	CloneTemplate string     `mapstructure:"clone_template"`

	ISOName   string `mapstructure:"iso_name"`
	ISOSRName string `mapstructure:"iso_sr"`
	NfsMount  string `mapstructure:"nfs_mount"`

	ISOUrl       string            `mapstructure:"iso_url"`
	ScriptUrl    string            `mapstructure:"script_url"`
	PlatformArgs map[string]string `mapstructure:"platform_args"`

	RawInstallTimeout string        `mapstructure:"install_timeout"`
	InstallTimeout    time.Duration ``

	ctx interpolate.Context
}

type Builder struct {
	config config
	runner multistep.Runner
}

func (self *Builder) Prepare(raws ...interface{}) (params []string, retErr error) {

	var errs *packer.MultiError

	err := hconfig.Decode(&self.config, &hconfig.DecodeOpts{
		Interpolate: true,
		InterpolateFilter: &interpolate.RenderFilter{
			Exclude: []string{
				"boot_command",
			},
		},
	}, raws...)

	if err != nil {
		packer.MultiErrorAppend(errs, err)
	}

	errs = packer.MultiErrorAppend(
		errs, self.config.CommonConfig.Prepare(&self.config.ctx, &self.config.PackerConfig)...)
	errs = packer.MultiErrorAppend(errs, self.config.SSHConfig.Prepare(&self.config.ctx)...)

	// Set default values
	if self.config.RawInstallTimeout == "" {
		self.config.RawInstallTimeout = "200m"
	}

	// For backwards compatibility, allow the existing disk_size option to be passed
	// and to override the newer vm_disks map, if it's also found
	if self.config.DiskSize > 0 {
		self.config.VMDisks = append ( self.config.VMDisks, []string{"Packer-disk", strconv.FormatUint(uint64(self.config.DiskSize), 10)} )
	}

	// If no disk info whatsoever is provided, fall back to the earlier standard of
	// one 40GB volume named Packer-disk
	if self.config.VMDisks == nil && self.config.DiskSize == 0 {
		self.config.VMDisks = append ( self.config.VMDisks, []string{"Packer-disk", "40000"} )
	}

	if self.config.VMMemory == 0 {
		self.config.VMMemory = 1024
	}

	if self.config.VMVCpus == 0 {
		self.config.VMVCpus = 1
	}

	if self.config.CloneTemplate == "" {
		self.config.CloneTemplate = "Other install media"
	}

	if len(self.config.PlatformArgs) == 0 {
		pargs := make(map[string]string)
		pargs["viridian"] = "false"
		pargs["nx"] = "true"
		pargs["pae"] = "true"
		pargs["apic"] = "true"
		pargs["timeoffset"] = "0"
		pargs["acpi"] = "1"
		pargs["cores_per_socket"] = "1"
		self.config.PlatformArgs = pargs
	}

	// Template and environment substitution
	/*	templates := map[string]*string{
			"clone_template":    &self.config.CloneTemplate,
			"network_name":      &self.config.NetworkName,
			"iso_name":          &self.config.ISOName,
			"iso_url":           &self.config.ISOUrl,
			"install_timeout":   &self.config.RawInstallTimeout,
		}
	*/

	// Validation
	self.config.InstallTimeout, err = time.ParseDuration(self.config.RawInstallTimeout)
	if err != nil {
		errs = packer.MultiErrorAppend(
			errs, fmt.Errorf("Failed to parse install_timeout: %s", err))
	}

	if self.config.ISOName == "" {
		errs = packer.MultiErrorAppend(
			errs, errors.New("You must specify the ISO name"))
	}

	if self.config.ISOUrl != "" {
		if self.config.ScriptUrl == "" {
			errs = packer.MultiErrorAppend(
				errs, errors.New("You must specify the URL of the copyiso script"))
		}

		if self.config.ISOSRName == "" {
			errs = packer.MultiErrorAppend(
				errs, errors.New("You must specify the SR for the ISO"))
		}
	}

	if len(errs.Errors) > 0 {
		retErr = errors.New(errs.Error())
	}

	return nil, retErr

}

func (self *Builder) Run(ui packer.Ui, hook packer.Hook, cache packer.Cache) (packer.Artifact, error) {
	//Setup XAPI client
	client := xsclient.NewXenAPIClient(self.config.HostIp, self.config.Username, self.config.Password)

	err := client.Login()
	if err != nil {
		return nil, err.(error)
	}
	ui.Say("XAPI client session established")

	client.GetHosts()

	//Share state between the other steps using a statebag
	state := new(multistep.BasicStateBag)
	state.Put("cache", cache)
	state.Put("client", client)
	state.Put("config", self.config)
	state.Put("commonconfig", self.config.CommonConfig)
	state.Put("hook", hook)
	state.Put("ui", ui)

	httpReqChan := make(chan string, 1)

	//Build the steps
	steps := []multistep.Step{
		&xscommon.StepPrepareOutputDir{
			Force: self.config.PackerForce,
			Path:  self.config.OutputDir,
		},
		&xscommon.StepPrepareNfsExport{
			NfsMount: self.config.NfsMount,
		},
		&xscommon.StepIsoDownload{
			IsoName:   self.config.ISOName,
			SrName:    self.config.ISOSRName,
			DlUrl:     self.config.ISOUrl,
			ScriptUrl: self.config.ScriptUrl,
		},
		&common.StepCreateFloppy{
			Files: self.config.FloppyFiles,
		},
		&xscommon.StepHTTPServer{
			Chan: httpReqChan,
		},
		&xscommon.StepUploadVdi{
			VdiNameFunc: func() string {
				return "Packer-floppy-disk"
			},
			ImagePathFunc: func() string {
				if floppyPath, ok := state.GetOk("floppy_path"); ok {
					return floppyPath.(string)
				}
				return ""
			},
			VdiUuidKey: "floppy_vdi_uuid",
		},
		&xscommon.StepFindVdi{
			VdiName:    self.config.ToolsIsoName,
			VdiUuidKey: "tools_vdi_uuid",
		},
		&xscommon.StepFindVdi{
			VdiName:    self.config.ISOName,
			VdiUuidKey: "iso_vdi_uuid",
		},
		new(stepCreateInstance),
		&xscommon.StepAttachVdi{
			VdiUuidKey: "floppy_vdi_uuid",
			VdiType:    xsclient.Floppy,
		},
		&xscommon.StepAttachVdi{
			VdiUuidKey: "iso_vdi_uuid",
			VdiType:    xsclient.CD,
		},
		new(xscommon.StepStartVmPaused),
		new(xscommon.StepGetVNCPort),
		&xscommon.StepForwardPortOverSSH{
			RemotePort:  xscommon.InstanceVNCPort,
			RemoteDest:  xscommon.InstanceVNCIP,
			HostPortMin: self.config.HostPortMin,
			HostPortMax: self.config.HostPortMax,
			ResultKey:   "local_vnc_port",
		},
		new(xscommon.StepBootWait),
		&xscommon.StepTypeBootCommand{
			Ctx: self.config.ctx,
		},
		new(xscommon.StepWaitForShutdown),
		&xscommon.StepDetachVdi{
			VdiUuidKey: "iso_vdi_uuid",
		},
		&xscommon.StepAttachVdi{
			VdiUuidKey: "tools_vdi_uuid",
			VdiType:    xsclient.CD,
		},
		new(xscommon.StepStartVmPaused),
		new(xscommon.StepBootWait),
		&xscommon.StepWaitForIP{ // do this again as could have new host and IP
			Chan:    httpReqChan,
			Timeout: self.config.InstallTimeout, // @todo change this
		},
		&xscommon.StepForwardPortOverSSH{
			RemotePort:  xscommon.InstanceSSHPort,
			RemoteDest:  xscommon.InstanceSSHIP,
			HostPortMin: self.config.HostPortMin,
			HostPortMax: self.config.HostPortMax,
			ResultKey:   "local_ssh_port",
		},
		&communicator.StepConnect{
			Config:    &self.config.SSHConfig.Comm,
			Host:      xscommon.CommHost,
			SSHConfig: xscommon.SSHConfigFunc(self.config.CommonConfig.SSHConfig),
			SSHPort:   xscommon.SSHPort,
		},
		new(xscommon.StepShutdown),
		&xscommon.StepDetachVdi{
			VdiUuidKey: "floppy_vdi_uuid",
		},
		new(xscommon.StepStartVmPaused),
		new(xscommon.StepBootWait),
		&xscommon.StepWaitForIP{ // do this again as could have new host and IP
			Chan:    httpReqChan,
			Timeout: self.config.InstallTimeout, // @todo change this
		},
		&xscommon.StepForwardPortOverSSH{
			RemotePort:  xscommon.InstanceSSHPort,
			RemoteDest:  xscommon.InstanceSSHIP,
			HostPortMin: self.config.HostPortMin,
			HostPortMax: self.config.HostPortMax,
			ResultKey:   "local_ssh_port",
		},
		/*&common.StepConnectSSH{
			SSHAddress:     xscommon.SSHLocalAddress,
			SSHConfig:      xscommon.SSHConfig,
			SSHWaitTimeout: self.config.SSHWaitTimeout,
		},*/
		&communicator.StepConnectSSH{
			Config:    &self.config.SSHConfig.Comm,
			Host:      xscommon.CommHost,
			SSHConfig: xscommon.SSHConfigFunc(self.config.CommonConfig.SSHConfig),
			SSHPort:   xscommon.SSHPort,
		},
		new(common.StepProvision),
		new(xscommon.StepShutdown),
		&xscommon.StepDetachVdi{
			VdiUuidKey: "tools_vdi_uuid",
		},
		&xscommon.StepExport{
			OutputFormat: self.config.Format,
		},
	}

	self.runner = &multistep.BasicRunner{Steps: steps}
	self.runner.Run(state)

	if rawErr, ok := state.GetOk("error"); ok {
		return nil, rawErr.(error)
	}

	// If we were interrupted or cancelled, then just exit.
	if _, ok := state.GetOk(multistep.StateCancelled); ok {
		return nil, errors.New("Build was cancelled.")
	}
	if _, ok := state.GetOk(multistep.StateHalted); ok {
		return nil, errors.New("Build was halted.")
	}

	artifactState := make(map[string]interface{})
	if len(state.Get("virtualization_type").(string)) == 0 {
		artifactState["virtualizationType"] = "PV"
	} else {
		artifactState["virtualizationType"] = "HVM"
	}

	for diskname, disksize := range self.config.VMDisks {
		artifactState[fmt.Sprintf("diskSize_%s", diskname)] = fmt.Sprintf("%d", disksize)
	}
	artifactState["ramSize"] = fmt.Sprintf("%d", self.config.VMMemory)
	artifactState["vm_name"] = self.config.VMName

	artifact, _ := xscommon.NewArtifact(self.config.OutputDir, artifactState, state.Get("export_files").([]string))

	return artifact, nil
}

func (self *Builder) Cancel() {
	if self.runner != nil {
		log.Println("Cancelling the step runner...")
		self.runner.Cancel()
	}
	fmt.Println("Cancelling the builder")
}
