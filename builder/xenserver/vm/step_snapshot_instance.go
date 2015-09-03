package vm

import (
	"fmt"
	"strconv"

	"github.com/mitchellh/multistep"
	"github.com/mitchellh/packer/packer"
	xsclient "github.com/xenserver/go-xenserver-client"
)

type stepSnapshotInstance struct {
	instance 		*xsclient.VM
	snapshot_instance 	*xsclient.VM
	clone_instance 	*xsclient.VM
	temp_network 		*xsclient.Network
}

func (self *stepSnapshotInstance) Run(state multistep.StateBag) multistep.StepAction {

	client := state.Get("client").(xsclient.XenAPIClient)
	config := state.Get("config").(config)
	ui := state.Get("ui").(packer.Ui)

	ui.Say("Step: Snapshot Instance")

	// Get the template to clone from

	vms, err := client.GetVMByNameLabel(config.SourceVm )

	switch {
	case len(vms) == 0:
		ui.Error(fmt.Sprintf("Couldn't find a source VM with the name-label '%s'. Aborting.", config.SourceVm ))
		return multistep.ActionHalt
	case len(vms) > 1:
		ui.Error(fmt.Sprintf("Found more than one source VM with the name '%s'. The name must be unique. Aborting.", config.SourceVm ))
		return multistep.ActionHalt
	}

	vm := vms[0]

	runningInstanceId, err := vm.GetUuid()
	if err != nil {
		ui.Error(fmt.Sprintf("Unable to get VM UUID: %s", err.Error()))
		return multistep.ActionHalt
	}

	ui.Message(fmt.Sprintf("Performing snapshot of source VM '%s'", runningInstanceId))

	// Create a running VM snapshot so we have something to work from
	snapshot, err := vm.Snapshot(config.TemporaryVm)
	if err != nil {
		ui.Error(fmt.Sprintf("Error performing snapshot of source VM: %s", err.Error()))
		return multistep.ActionHalt
	}

	self.snapshot_instance = snapshot

	ui.Message("Creating template from snapshot")

	clone, err := snapshot.Clone("packer-clone-" + config.SourceVm)
	if err != nil {
		ui.Error(fmt.Sprintf("Error creating a clone to templatize: %s", err.Error()))
		return multistep.ActionHalt
	}

	self.clone_instance = clone

	sr, err := config.GetSrByName(client, config.SrName)
	if err != nil {
		ui.Error(fmt.Sprintf("Unable to get SR: %s", err.Error()))
		return multistep.ActionHalt
	}

	ui.Message("Cloning template onto target storage")

	instance, err := clone.Copy(config.VMName, sr)
	if err != nil {
		ui.Error(fmt.Sprintf("Error performing clone of template VM: %s", err.Error()))
		return multistep.ActionHalt
	}

	self.instance = instance

	// no longer want this to be a template
	err = instance.SetIsATemplate(false)
	if err != nil {
		ui.Error(fmt.Sprintf("Error setting is_a_template=false: %s", err.Error()))
		return multistep.ActionHalt
	}


	ui.Message("Removing source template")

	err = self.removeInstance ( self.clone_instance, ui )
	if err != nil {
		ui.Error(fmt.Sprintf("Error removing source template: %s", err.Error()))
		return multistep.ActionHalt
	}

	self.clone_instance = nil

	ui.Message("Removing source snapshot")
	err = self.removeInstance ( self.snapshot_instance, ui )
	if err != nil {
		ui.Error(fmt.Sprintf("Error removing snapshot: %s", err.Error()))
		return multistep.ActionHalt
	}

	self.snapshot_instance = nil

	// now that we have a cleansed instance, lets make certain there is only one disk
	vdis, err := instance.GetDisks()
	if err != nil {
		ui.Error(fmt.Sprintf("Error getting list of disks: %s", err.Error()))
		return multistep.ActionHalt
	}		

	if len(vdis) > 1 {
		ui.Error(fmt.Sprintf("Only VMs with one disk can be processed.  This VM has %d.", len(vdis)))
		return multistep.ActionHalt
	}

	diskSizeString, err := vdis[0].GetVirtualSize()

	if err != nil {
		ui.Error(fmt.Sprintf("Error determining disk size: %s", err.Error()))
		return multistep.ActionHalt
	}

	// diskSizeString is in bytes make it GB; but int64
	diskSizeInt64, _ := strconv.ParseInt ( diskSizeString, 10, 64 )
	diskSizeInt64 = diskSizeInt64 / 1024 / 1024 / 1024
	
	diskSize := fmt.Sprintf ("%d", diskSizeInt64 )
	ui.Message("Found disk size of: " + diskSize)
	state.Put("configured_disk", diskSize)
	
	// Connect isolated network to avoid machine collision

	ui.Message("Creating temporary isolated network...")
	network , err := client.CreateNetwork("Packer isolated", "An internal network to prevent machine collisons in Packer", "")
	if err != nil {
		ui.Error(fmt.Sprintf("Error creating temporary network: %s", err.Error()))
		return multistep.ActionHalt
	}
	self.temp_network = network 

	vifs, err := instance.GetVIFs ()
	if err != nil {
		ui.Error(fmt.Sprintf("Unable to obtain the list of VIFs: %s", err.Error()))
		return multistep.ActionHalt
	}

	networks := make([]*xsclient.Network, len(vifs))

	ui.Message(fmt.Sprintf("Saving %d networks", len(vifs)))

	// save existing networks for later reinstall
	for i := 0; i < len(vifs); i++ {

		network, err := vifs[i].GetNetwork()
		if err != nil {
			ui.Error(fmt.Sprintf("Unable to locate network from vif %d: %s", i, err.Error()))
			return multistep.ActionHalt
		}

		networks[i] = network
	}


	for i := 0; i < len(vifs); i++ {
		err = vifs[i].Destroy()
		if err != nil {
			ui.Error(fmt.Sprintf("Unable to remove interface %d from VM: %s", i, err.Error()))
			return multistep.ActionHalt
		}
	}

	for i := 0; i < len(vifs); i++ {

		if i == 0 {
			ui.Message("Skipping plug of network device 0 since that will be HIMN")
			continue
		}

		_, err = instance.ConnectNetwork(self.temp_network, fmt.Sprintf("%d", i))

		if err != nil {
			ui.Error(fmt.Sprintf("Unable to connect interface %d the temporary network: %s", i, err.Error()))
			return multistep.ActionHalt
		}
	}

	instanceId, err := instance.GetUuid()
	if err != nil {
		ui.Error(fmt.Sprintf("Unable to get VM UUID: %s", err.Error()))
		return multistep.ActionHalt
	}

	bootOrder, err := instance.GetHVMBootPolicy()
	if err != nil {
		ui.Error(fmt.Sprintf("Unable to determine if VM is HVM or PV: %s", err.Error()))
		return multistep.ActionHalt
	}

	state.Put("virtualization_type", bootOrder)
	state.Put("instance_uuid", instanceId)
	state.Put("original_networks", networks)

	ui.Say(fmt.Sprintf("Created instance '%s'", instanceId))

	srId, err := sr.GetUuid()
	if err != nil {
		ui.Error(fmt.Sprintf("Unable to get VDI SR UUID: %s", err.Error()))
		return multistep.ActionHalt
	}

	state.Put("instance_sr_uuid", srId)
	ui.Say(fmt.Sprintf("Using SR '%s'", srId))

	return multistep.ActionContinue
}

func (self *stepSnapshotInstance) Cleanup(state multistep.StateBag) {
	config := state.Get("config").(config)
	if config.ShouldKeepVM(state) {
		return
	}

	ui := state.Get("ui").(packer.Ui)

	err := self.removeInstance ( self.clone_instance, ui )
	if err != nil {
		ui.Error(err.Error())
	}

	err = self.removeInstance ( self.snapshot_instance, ui )
	if err != nil {
		ui.Error(err.Error())
	}

	err = self.removeInstance ( self.instance, ui )
	if err != nil {
		ui.Error(err.Error())
	}

	if self.temp_network != nil {
		ui.Say("Destroying temporary network")
		err := self.temp_network.Destroy()
		if err != nil {
			ui.Error(err.Error())
		}
	}

}

func (self *stepSnapshotInstance) removeInstance(instance *xsclient.VM, ui packer.Ui) (err error) {

	if instance != nil {
		uuid, _ := instance.GetUuid()
		ui.Message(fmt.Sprintf("Removing instance '%s'", uuid))
		_ = instance.HardShutdown() // redundant, just in case

		vdis, err := instance.GetDisks()
		if err != nil {
			return err
		}		

		for _, vdi := range vdis {
			vdi_uuid, _ := vdi.GetUuid()

			ui.Message(fmt.Sprintf("Destroying vdi '%s'", vdi_uuid))		
			err = vdi.Destroy()
			if err != nil {
				return err
			}
		}

		ui.Message(fmt.Sprintf("Destroying instance '%s'", uuid))
		err = instance.Destroy()
		if err != nil {
			return err
		}
	}

	return nil

}