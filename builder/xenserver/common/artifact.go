package common

import (
	"fmt"
	"github.com/mitchellh/packer/packer"
	"os"
)

// This is the common builder ID to all of these artifacts.
const BuilderId = "packer.xenserver"

type LocalArtifact struct {
	dir string
	f   []string
	state map[string]interface{}
}

func NewArtifact(dir string, state map[string]interface{}, files []string) (packer.Artifact, error) {
	return &LocalArtifact{
		dir: dir,
		f:   files,
		state: state,
	}, nil
}

func (*LocalArtifact) BuilderId() string {
	return BuilderId
}

func (a *LocalArtifact) Files() []string {
	return a.f
}

func (*LocalArtifact) Id() string {
	return "VM"
}

func (a *LocalArtifact) String() string {
	return fmt.Sprintf("VM files in directory: %s", a.dir)
}

func (a *LocalArtifact) State(name string) interface{} {
	return a.state[name]
}

func (a *LocalArtifact) Destroy() error {
	return os.RemoveAll(a.dir)
}
