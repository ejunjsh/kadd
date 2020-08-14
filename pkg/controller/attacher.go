package controller

import (
	"context"
	dockerclient "github.com/docker/docker/client"
	"io"
	kubetype "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/remotecommand"
)

type attacher struct {
	targetId string
	cmd      string
	image    string
	context  context.Context
}

func (a *attacher) AttachContainer(name string, uid kubetype.UID, container string, in io.Reader, out, err io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	client, error := dockerclient.NewClient("unix:///var/run/docker.sock", "", nil, nil)
	if error != nil {
		return error
	}
	runtime := &DockerContainerRuntime{
		Client: client,
	}

	error = runtime.PullImage(a.context, a.image, "", out)
	if error != nil {
		return error
	}

	return runtime.RunDebugContainer(cfg)
}
