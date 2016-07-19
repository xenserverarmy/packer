# XenServer Packer builder and post-processors

The builder plugin extends packer to support building images for XenServer. 
The post-processor plug extends packer to support registration of XenServer images within various providers (Apache CloudStack for the moment)

You can check out packer [here](https://packer.io).


## Dependencies
* Packer >= 0.10.1 (https://packer.io)
* XenServer > 6.2 (http://xenserver.org)
* Apache CloudStack > 4.2 (http://cloudstack.apache.org/) for ACS plugin
* OpenStack Havana or later for OpenStack plugin
* Golang (tested with 1.6.2) 


## Install Go

Follow these instructions and install golang on your system:
* https://golang.org/doc/install

## Install Packer

Follow the directions and install packer on your system:
* https://www.packer.io/downloads.html

Note that if you're upgrading from a version of packer prior to 0.9.0, you must remove the packer-* files lest things get confused.

> Important: CentOS 7 and RHEL 7 both have a symlink for packer.
> The symlink points to cracklib-packer, and you're probably
> going to want to keep that as is. 
> 
> To keep things simple, in the examples below, the packer
> binary has been fully qualified with $GOPATH/bin since
> I installed packer to that location. Please adjust as
> your requirements dictate.


## Compile the plugin

Once you have installed Packer, you must compile this plugin and install the resulting binary.

```shell
cd $GOPATH
mkdir -p src/github.com/xenserverarmy/
cd src/github.com/xenserverarmy
git clone https://github.com/xenserverarmy/packer.git
cd packer
./build.sh
```

If the build is successful, you should now have `packer-builder-xenserver-iso` and
`packer-builder-xenserver-xva` binaries in your `$GOPATH/bin` directory and you are
ready to get going with packer; skip to the CentOS 7 example below.

In order to do a cross-compile, run instead:
```shell
XC_OS="windows linux" XC_ARCH="386 amd64" ./build.sh
```
This builds 32 and 64 bit binaries for both Windows and Linux. Native binaries will
be installed in `$GOPATH/bin` as above, and cross-compiled ones in the `pkg/` directory.

Don't forget to also cross compile Packer, by running
```shell
XC_OS="windows linux" XC_ARCH="386 amd64" make bin
```
(instead of `make dev`) in the directory where you checked out Packer.

## CentOS 7 Builder Example

Once you've setup the above, you are good to go with an example. 

To get you started, there is an example config file which you can use:
[`examples/centos-7.json`](https://github.com/xenserverarmy/packer/blob/master/examples/centos-7.json)

The example is functional, once suitable `remote_host`, `remote_username` and `remote_password` configurations have been substituted.

A brief explanation of what the config parameters mean:
 * `type` - specifies the builder type. This is 'xenserver-iso', for installing
   a VM from scratch, 'xenserver-xva' to import existing XVA as a starting
   point, or 'xenserver-vm' to use a runnning VM as source
 * `remote_host` - the IP for the XenServer host being used.
 * `remote_username` - the username for the XenServer host being used.
 * `remote_password` - the password for the XenServer host being used.
 * `boot_command` - a list of commands to be sent to the instance over XenServer VNC connection to VM.
 * `boot_wait` - how long to wait for the VM isntance to initially start
 * `disk_size` - the size of the disk the VM should be created with, in MB.
 * `iso_url` - the url from which to download the ISO and place it in the iso_sr
 * `iso_name` - the name of the ISO visible on a ISO SR connected to the XenServer host, or the name to assign to it upon download.
 * `iso_sr` - the name of the ISO SR a downloaded ISO should be placed in
 * `script_url` - the url from where XenServer Packer scripts are located
 * `output_directory` - the path relative to 'packer build' that output will be located
 * `format` - the output artifact type.  Valid values are 'vhd', 'vdi_raw', and 'xva'
 * `shutdown_command` - reserved -- leave blank
 * `ssh_username` - the username set by the installer for the instance; used for validation and in post-processors
 * `ssh_password` - the password set by the installer for the instance; used for validation and in post-processors
 * `sr_name` - the name of the SR for the VM instance.  For vhd artifacts, this must be NFS
 * `vm_name` - the name that should be given to the created VM.
 * `vm_memory` - the static memory configuration for the VM, in MB.
 * `vm_vcpus` - the number of vCPUs to assign during build
 * `nfs_mount` - Used for VHD artifacts, the NFS mount for the sr_name

Once you've updated the config file with your own parameters, you can use packer to build this VM with the following command:

```
$GOPATH/bin/packer build ./examples/centos-7.json
```

## CentOS 7 Builder from running VM Example
The idea behind using a running VM as a starting point is to test patching/conversion
of existing infrastructure; without incurring downtime.  Using this builder, you can 
clone, perform the tests on the clone until satisfied, and then apply them.  Source VM
experiences no downtime, and there is no machine collision (if you pay attention). 

To facilitate this process, the clone is launched on an internal network and then basic
commands (contained in boot_command) are executed to provide a clean network which is 
then used to copy in packer_clean.sh from the script location.  You modify that script 
to do what you need to make your VM unique, and then have it perform a shutdown.  The
remaining work you'd want to do to make the patch/update/convert/process/whatever the
Vm is done using Packer Provisioners.

To get you started, there is an example config file which you can use:
[`examples/centos-7vm.json`](https://github.com/xenserverarmy/packer/blob/master/examples/centos-7vm.json)

The example is functional, once suitable `remote_host`, `remote_username` and `remote_password` configurations have been substituted.

A brief explanation of what the config parameters mean:
 * `type` - specifies the builder type. This is 'xenserver-iso', for installing
   a VM from scratch, or 'xenserver-xva' to import existing XVA as a starting
   point.
 * `remote_host` - the IP for the XenServer host being used.
 * `remote_username` - the username for the XenServer host being used.
 * `remote_password` - the password for the XenServer host being used.
 * `boot_command` - a list of commands to be sent to the instance over XenServer VNC connection to VM. 
 * `boot_wait` - how long to wait for the VM isntance to initially start
 * `script_url` - the url from where XenServer Packer scripts are located
 * `output_directory` - the path relative to 'packer build' that output will be located
 * `format` - the output artifact type.  Valid values are 'vhd', 'vdi_raw', and 'xva'
 * `shutdown_command` - reserved -- leave blank
 * `ssh_username` - the username set by the installer for the instance; used for validation and in post-processors
 * `ssh_password` - the password set by the installer for the instance; used for validation and in post-processors
 * `sr_name` - the name of the SR for the VM instance.  For vhd artifacts, this must be NFS
 * `vm_name` - the name that should be given to the created VM.
 * `source_vm` - the name of the VM to clone and operate on
 * `nfs_mount` - Used for VHD artifacts, the NFS mount for the sr_name

Once you've updated the config file with your own parameters, you can use packer to build this VM with the following command:

```
$GOPATH/bin/packer build ./examples/centos-7vm.json
```


## Apache CloudStack Post-processor Example with CentOS 7

Once you've setup the above, you are good to go with an example. 

To get you started, there is an example config file which you can use:
[`examples/centos-7acs.json`](https://github.com/xenserverarmy/packer/blob/master/examples/centos-7acs.json)

The builder parameters are described in the CentOS 7 builder example.

This example is functional, once suitable `apiurl`, `apikey` and `secret` configurations have been substituted.

A brief explanation of what the config parameters mean:
 * `type` - This is 'cloudstack-xenserver'
 * `only` - This is 'xenserver-iso', since the output VHD is required.
 * `apiurl` - The API endpoint for the CloudStack instance
 * `apikey` - The API key to use for authentication
 * `secret` - The API secret to use for authentication
 * `display_text` - How the template should be described
 * `template_name` - The name to assign to the template
 * `os_type` - The name assigned to the CloudStack OS type for this template
 * `download_url` - the url from which CloudStack should download the VHD artifact produced by the builder
 * `zone` - The ability zone where the template will exist
 * `account` - the name of the users account which will have the instance.  If blank, then the account associated with the apikey will be used
 * `domain` - If account is specified, the domain must also be specified
 * `password_enabled` - Set to 'true' if the CloudStack password management script is in the artifact
 * `ssh_enabled` - Set to 'true' if the CloudStack SSH key script is in the artifact
 * `has_tools` - Set to 'true' if the XenServer tools have been installed in the artifact

Once you've updated the config file with your own parameters, you can use packer to build this VM, and upload it to your CloudStack instance with the following command:

```
$GOPATH/bin/packer build ./examples/centos-7acs.json
```


## Testing

This code was built on CentOS 7 and has been tested with a stock XenServer 6.5 and Apache CloudStack 4.3.  There is no known limitation which would prevent its use with XenServer 6.2, or other Linux variants.  The post-processor uses stable CloudStack APIs which should allow for its use on most CloudStack versions.
