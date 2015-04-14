package vm

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/mitchellh/multistep"
	"github.com/mitchellh/packer/common"
	"github.com/mitchellh/packer/packer"
	xscommon "github.com/xenserverarmy/packer/builder/xenserver/common"

	xsclient "github.com/xenserverarmy/go-xenserver-client"
)

type config struct {
	common.PackerConfig   `mapstructure:",squash"`
	xscommon.CommonConfig `mapstructure:",squash"`

	SourceVm 	string  `mapstructure:"source_vm"`
	NfsMount	string	 `mapstructure:"nfs_mount"`

	ScriptUrl       string   `mapstructure:"script_url"`

	RawBootTimeout string        `mapstructure:"boot_timeout"`
	BootTimeout    time.Duration ``
	TemporaryVm	string	 ``	

	tpl *packer.ConfigTemplate
}

type Builder struct {
	config config
	runner multistep.Runner
}

func (self *Builder) Prepare(raws ...interface{}) (params []string, retErr error) {

	md, err := common.DecodeConfig(&self.config, raws...)
	if err != nil {
		return nil, err
	}

	self.config.tpl, err = packer.NewConfigTemplate()
	if err != nil {
		return nil, err
	}
	self.config.tpl.UserVars = self.config.PackerUserVars

	errs := common.CheckUnusedConfig(md)
	errs = packer.MultiErrorAppend( errs, self.config.CommonConfig.Prepare(self.config.tpl, &self.config.PackerConfig)...)

	// Set default values

	if self.config.RawBootTimeout == "" {
		self.config.RawBootTimeout = "200m"
	}

	// Template and environment substitution
	templates := map[string]*string{
		"source_vm":    	&self.config.SourceVm,
		"network_name":      &self.config.NetworkName,
		"boot_timeout":   	&self.config.RawBootTimeout,
	}
	for n, ptr := range templates {
		var err error
		*ptr, err = self.config.tpl.Process(*ptr, nil)
		if err != nil {
			errs = packer.MultiErrorAppend(errs, fmt.Errorf("Error processing %s: %s", n, err))
		}
	}

	// Validation

	self.config.BootTimeout, err = time.ParseDuration(self.config.RawBootTimeout)
	if err != nil {
		errs = packer.MultiErrorAppend(
			errs, fmt.Errorf("Failed to parse boot_timeout: %s", err))
	}
       
	self.config.TemporaryVm = self.config.VMName + "_packer_snap"

	if len(errs.Errors) > 0 {
		retErr = errors.New(errs.Error())
	}

	return nil, retErr

}

func (self *Builder) Run(ui packer.Ui, hook packer.Hook, cache packer.Cache) (packer.Artifact, error) {
	//Setup XAPI client
	client := xsclient.NewXenAPIClient(self.config.HostIp, self.config.Username, self.config.Password)
	artifactState := make(map[string]interface{})

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
		&xscommon.StepHTTPServer{
			Chan: httpReqChan,
		},
		new(stepSnapshotInstance),
		&xscommon.StepStartOnHIMN {
			PingTest:	false,
		},
		new(xscommon.StepGetVNCPort),
		&xscommon.StepForwardPortOverSSH{
			RemotePort:  xscommon.InstanceVNCPort,
			RemoteDest:  xscommon.InstanceVNCIP,
			HostPortMin: self.config.HostPortMin,
			HostPortMax: self.config.HostPortMax,
			ResultKey:   "local_vnc_port",
		},
		&stepCopyCleanScript{
			ScriptUrl: self.config.ScriptUrl,
		},
		new(xscommon.StepBootWait),
		&xscommon.StepTypeBootCommand{
			Tpl: self.config.tpl,
		},
		new(xscommon.StepWaitForShutdown),
		new(stepRestoreNetwork),
		new(xscommon.StepStartVm),
		&xscommon.StepWaitForIP{
			Chan:    httpReqChan,
			Timeout: self.config.BootTimeout, // @todo change this
		},
		&xscommon.StepForwardPortOverSSH{ // do this again as could have new host and IP
			RemotePort:  xscommon.InstanceSSHPort,
			RemoteDest:  xscommon.InstanceSSHIP,
			HostPortMin: self.config.HostPortMin,
			HostPortMax: self.config.HostPortMax,
			ResultKey:   "local_ssh_port",
		},
		//new(xscommon.StepBootWait),
		&common.StepConnectSSH{
			SSHAddress:     xscommon.SSHLocalAddress,
			SSHConfig:      xscommon.SSHConfig,
			SSHWaitTimeout: self.config.SSHWaitTimeout,
		},
		new(common.StepProvision),
		new(xscommon.StepShutdown),
		&xscommon.StepExport{
			OutputFormat : self.config.Format,
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


	if len(state.Get("virtualization_type").(string)) == 0 {
		artifactState["virtualizationType"] = "PV"
	} else {
		artifactState["virtualizationType"] = "HVM"
	}

	value, found := state.Get("configured_disk").(string)
	if found {
		artifactState["diskSize"] = value
	} 

	value, found = state.Get("configured_ram").(string)
	if found {
		artifactState["ramSize"] = value
	}

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
