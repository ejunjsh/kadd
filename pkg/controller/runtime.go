package controller

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/strslice"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"io"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/remotecommand"
	"log"
	"time"
)

type DockerContainerRuntime struct {
	Client *dockerclient.Client
}

type RunConfig struct {
	context  context.Context
	timeout  time.Duration
	targetId string
	image    string
	command  []string
	stdin    io.Reader
	stdout   io.WriteCloser
	stderr   io.WriteCloser
	tty      bool
	resize   <-chan remotecommand.TerminalSize
}

func (c *RunConfig) getContextWithTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.context, c.timeout)
}

func (c *DockerContainerRuntime) PullImage(ctx context.Context,
	image string, authStr string,
	stdout io.WriteCloser) error {
	out, err := c.Client.ImagePull(ctx, image, types.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer out.Close()
	io.Copy(stdout, out)
	return nil
}

func (c *DockerContainerRuntime) RunDebugContainer(cfg RunConfig) error {

	createdBody, err := c.CreateContainer(cfg)
	if err != nil {
		return err
	}
	if err := c.StartContainer(cfg, createdBody.ID); err != nil {
		return err
	}

	defer c.CleanContainer(cfg, createdBody.ID)

	cfg.stdout.Write([]byte("container created, open tty...\n\r"))

	// from now on, should pipe stdin to the container and no long read stdin
	// close(m.stopListenEOF)

	return c.AttachToContainer(cfg, createdBody.ID)
}

func (c *DockerContainerRuntime) CreateContainer(cfg RunConfig) (*container.ContainerCreateCreatedBody, error) {

	config := &container.Config{
		Entrypoint: strslice.StrSlice(cfg.command),
		Image:      cfg.image,
		Tty:        true,
		OpenStdin:  true,
		StdinOnce:  true,
	}
	hostConfig := &container.HostConfig{
		NetworkMode: container.NetworkMode(c.containerMode(cfg.targetId)),
		UsernsMode:  container.UsernsMode(c.containerMode(cfg.targetId)),
		IpcMode:     container.IpcMode(c.containerMode(cfg.targetId)),
		PidMode:     container.PidMode(c.containerMode(cfg.targetId)),
		CapAdd:      strslice.StrSlice([]string{"SYS_PTRACE", "SYS_ADMIN"}),
	}
	ctx, cancel := cfg.getContextWithTimeout()
	defer cancel()
	body, err := c.Client.ContainerCreate(ctx, config, hostConfig, nil, "")
	if err != nil {
		return nil, err
	}
	return &body, nil
}

func (c *DockerContainerRuntime) containerMode(idOfCntnrToDbg string) string {
	return fmt.Sprintf("container:%s", idOfCntnrToDbg)
}

// Run a new container, this container will join the network,
// mount, and pid namespace of the given container
func (c *DockerContainerRuntime) StartContainer(cfg RunConfig, id string) error {
	ctx, cancel := cfg.getContextWithTimeout()
	defer cancel()
	err := c.Client.ContainerStart(ctx, id, types.ContainerStartOptions{})
	if err != nil {
		return err
	}
	return nil
}

func (c *DockerContainerRuntime) CleanContainer(cfg RunConfig, id string) {
	// cleanup procedure should use background context
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	// wait the container gracefully exit
	statusCh, errCh := c.Client.ContainerWait(ctx, id, container.WaitConditionNotRunning)
	var rmErr error
	select {
	case err := <-errCh:
		if err != nil {
			log.Println("error waiting container exit, kill with --force")
			// timeout or error occurs, try force remove anywawy
			rmErr = c.RmContainer(cfg, id, true)
		}
	case <-statusCh:
		rmErr = c.RmContainer(cfg, id, false)
	}
	if rmErr != nil {
		log.Printf("error remove container: %s \n", id)
	}
}

func (c *DockerContainerRuntime) RmContainer(cfg RunConfig, id string, force bool) error {
	// cleanup procedure should use background context
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	return c.Client.ContainerRemove(ctx, id,
		types.ContainerRemoveOptions{
			Force: true,
		})
}

// AttachToContainer do `docker attach`.  Blocks until container I/O complete
func (c *DockerContainerRuntime) AttachToContainer(cfg RunConfig, container string) error {
	handleResizing(cfg.resize, func(size remotecommand.TerminalSize) {
		c.resizeContainerTTY(cfg, container, uint(size.Height), uint(size.Width))
	})

	opts := types.ContainerAttachOptions{
		Stream: true,
		Stdin:  cfg.stdin != nil,
		Stdout: cfg.stdout != nil,
		Stderr: cfg.stderr != nil,
	}
	ctx, cancel := cfg.getContextWithTimeout()
	defer cancel()
	resp, err := c.Client.ContainerAttach(ctx, container, opts)
	if err != nil {
		return err
	}
	defer resp.Close()

	return c.holdHijackedConnection(cfg, resp)
}

func (c *DockerContainerRuntime) resizeContainerTTY(cfg RunConfig, id string, height, width uint) error {
	ctx, cancel := cfg.getContextWithTimeout()
	defer cancel()
	return c.Client.ContainerResize(ctx, id, types.ResizeOptions{
		Height: height,
		Width:  width,
	})
}

// holdHijackedConnection hold the HijackedResponse, redirect the inputStream to the connection, and redirect the response
// stream to stdout and stderr. NOTE: If needed, we could also add context in this function.
func (c *DockerContainerRuntime) holdHijackedConnection(cfg RunConfig, resp types.HijackedResponse) error {
	receiveStdout := make(chan error)
	if cfg.stdout != nil || cfg.stderr != nil {
		go func() {
			receiveStdout <- c.redirectResponseToOutputStream(cfg, resp.Reader)
		}()
	}

	stdinDone := make(chan struct{})
	go func() {
		if cfg.stdin != nil {
			io.Copy(resp.Conn, cfg.stdin)
		}
		resp.CloseWrite()
		close(stdinDone)
	}()

	select {
	case err := <-receiveStdout:
		return err
	case <-stdinDone:
		if cfg.stdout != nil || cfg.stderr != nil {
			return <-receiveStdout
		}
	}
	return nil
}

func (c *DockerContainerRuntime) redirectResponseToOutputStream(cfg RunConfig, resp io.Reader) error {
	var stdout io.Writer = cfg.stdout
	if stdout == nil {
		stdout = ioutil.Discard
	}
	var stderr io.Writer = cfg.stderr
	if stderr == nil {
		stderr = ioutil.Discard
	}
	var err error
	if cfg.tty {
		_, err = io.Copy(stdout, resp)
	} else {
		_, err = stdcopy.StdCopy(stdout, stderr, resp)
	}
	return err
}

func handleResizing(resize <-chan remotecommand.TerminalSize, resizeFunc func(size remotecommand.TerminalSize)) {
	go func() {
		defer runtime.HandleCrash()

		for size := range resize {
			if size.Height < 1 || size.Width < 1 {
				continue
			}
			resizeFunc(size)
		}
	}()
}
