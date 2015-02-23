package main

import (
	"github.com/mitchellh/packer/packer/plugin"
	"github.com/xenserverarmy/packer/post-processor/cloudstack/xenserver"
)

func main() {
	server, err := plugin.Server()
	if err != nil {
		panic(err)
	}
	server.RegisterPostProcessor(new(xenserver.PostProcessor))
	server.Serve()
}
