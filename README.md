# XenServer Packer builder

This builder plugin extends packer to support building images for XenServer. 

You can check out packer [here](https://packer.io).


## Dependencies
* Packer >= 0.7.2 (https://packer.io)
* XenServer > 6.2 (http://xenserver.org)
* Golang (tested with 1.2.1) 


## Install Go

Follow these instructions and install golang on your system:
* https://golang.org/doc/install

## Install Packer

Clone the Packer repository:

```shell
git clone https://github.com/mitchellh/packer.git
```

Then follow the [instructions to build and install a development version of Packer](https://github.com/mitchellh/packer#developing-packer).

## Compile the plugin

Once you have installed Packer, you must compile this plugin and install the resulting binary.

```shell
cd $GOROOT
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

## CentOS 7 Example

Once you've setup the above, you are good to go with an example. 

To get you started, there is an example config file which you can use:
[`examples/centos-7.json`](https://github.com/xenserverarmy/packer/blob/master/examples/centos-7.json)

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
packer build centos-7.json
```

## Testing

This code was built on CentOS 7 and has been tested with a stock XenServer 6.5.  There is no known limitation which would prevent its use with XenServer 6.2, or other Linux variants.
