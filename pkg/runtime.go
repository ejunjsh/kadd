package pkg

import (
	"context"
	"encoding/base64"
	"fmt"
	term "github.com/aylei/kubectl-debug/pkg/util"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/strslice"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"io"
	"io/ioutil"
	"log"
)

type ContainerRuntime interface {
	PullImage(ctx context.Context, image string,
		skipTLS bool, authStr string,
		stdout io.WriteCloser) error
	ContainerInfo(ctx context.Context, cfg RunConfig) (ContainerInfo, error)
	RunDebugContainer(cfg RunConfig) error
}

type DockerContainerRuntime struct {
	client *dockerclient.Client
}

func (c *DockerContainerRuntime) PullImage(ctx context.Context,
	image string, skipTLS bool, authStr string,
	stdout io.WriteCloser) error {
	authBytes := base64.URLEncoding.EncodeToString([]byte(authStr))
	out, err := c.client.ImagePull(ctx, image, types.ImagePullOptions{RegistryAuth: string(authBytes)})
	if err != nil {
		return err
	}
	defer out.Close()
	// write pull progress to user
	term.DisplayJSONMessagesStream(out, stdout, 1, true, nil)
	return nil
}

func (c *DockerContainerRuntime) ContainerInfo(ctx context.Context, cfg RunConfig) (ContainerInfo, error) {
	var ret ContainerInfo
	cntnr, err := c.client.ContainerInspect(ctx, cfg.idOfContainerToDebug)
	if err != nil {
		return ContainerInfo{}, err
	}
	ret.Pid = int64(cntnr.State.Pid)
	for _, mount := range cntnr.Mounts {
		ret.MountDestinations = append(ret.MountDestinations, mount.Destination)
	}
	return ret, nil
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
		NetworkMode: container.NetworkMode(c.containerMode(cfg.idOfContainerToDebug)),
		UsernsMode:  container.UsernsMode(c.containerMode(cfg.idOfContainerToDebug)),
		IpcMode:     container.IpcMode(c.containerMode(cfg.idOfContainerToDebug)),
		PidMode:     container.PidMode(c.containerMode(cfg.idOfContainerToDebug)),
		CapAdd:      strslice.StrSlice([]string{"SYS_PTRACE", "SYS_ADMIN"}),
	}
	ctx, cancel := cfg.getContextWithTimeout()
	defer cancel()
	body, err := c.client.ContainerCreate(ctx, config, hostConfig, nil, "")
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
	err := c.client.ContainerStart(ctx, id, types.ContainerStartOptions{})
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
	statusCh, errCh := c.client.ContainerWait(ctx, id, container.WaitConditionNotRunning)
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
	} else if cfg.verbosity > 0 {
		log.Printf("Debug session end, debug container %s removed", id)
	}
}

func (c *DockerContainerRuntime) RmContainer(cfg RunConfig, id string, force bool) error {
	// cleanup procedure should use background context
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	return c.client.ContainerRemove(ctx, id,
		types.ContainerRemoveOptions{
			Force: true,
		})
}

// AttachToContainer do `docker attach`.  Blocks until container I/O complete
func (c *DockerContainerRuntime) AttachToContainer(cfg RunConfig, container string) error {
	HandleResizing(cfg.resize, func(size remotecommand.TerminalSize) {
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
	resp, err := c.client.ContainerAttach(ctx, container, opts)
	if err != nil {
		return err
	}
	defer resp.Close()

	return c.holdHijackedConnection(cfg, resp)
}

func (c *DockerContainerRuntime) resizeContainerTTY(cfg RunConfig, id string, height, width uint) error {
	ctx, cancel := cfg.getContextWithTimeout()
	defer cancel()
	return c.client.ContainerResize(ctx, id, types.ResizeOptions{
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
