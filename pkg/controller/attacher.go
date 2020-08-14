package controller

import (
	dockerclient "github.com/docker/docker/client"
	runtime2 "github.com/ejunjsh/kps/pkg/runtime"
	"io"
	kubetype "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/remotecommand"
)

type attacher struct {
	targetId string
	cmd string
}

func (a *attacher) AttachContainer(name string, uid kubetype.UID, container string, in io.Reader, out, err io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	client,error :=  dockerclient.NewClient("unix:///var/run/docker.sock","",nil,nil)
	if error!=nil {
		return error
	}
	runtime := &runtime2.DockerContainerRuntime{
		Client: client,
	}

	runtime.
}