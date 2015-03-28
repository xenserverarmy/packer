package main

import (
	"github.com/mitchellh/packer/packer/plugin"
	"github.com/xenserverarmy/packer/builder/xenserver/vm"
)

func main() {
	server, err := plugin.Server()
	if err != nil {
		panic(err)
	}
	server.RegisterBuilder(new(vm.Builder))
	server.Serve()
}
