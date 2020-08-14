package client

import (
	"context"
	"fmt"
	"io"
	corev1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	coreclient "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/tools/watch"
	"k8s.io/kubernetes/pkg/client/conditions"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
	"net/url"
	"time"
)

const name = "haha"

type KubeClient struct {
	Clientset  *kubernetes.Clientset
	CoreClient coreclient.CoreV1Interface
	RestConfig *rest.Config
	RestClient *rest.RESTClient
}

func NewKubeClient() (*KubeClient, error) {
	flags := genericclioptions.NewConfigFlags()
	configLoader := flags.ToRawKubeConfigLoader()
	config, _ := configLoader.ClientConfig()
	clientset, _ := kubernetes.NewForConfig(config)

	matchVersionKubeConfigFlags := cmdutil.NewMatchVersionFlags(flags)
	f := cmdutil.NewFactory(matchVersionKubeConfigFlags)
	restClient, err := f.RESTClient()

	if err != nil {
		return nil, err
	}

	return &KubeClient{
		Clientset:  clientset,
		CoreClient: clientset.CoreV1(),
		RestConfig: config,
		RestClient: restClient,
	}, nil
}

func (cli *KubeClient) getPodByName(ns string, podName string) (*corev1.Pod, error) {
	return cli.CoreClient.Pods(ns).Get(podName, metaV1.GetOptions{})
}

func (cli *KubeClient) getContainerIDByName(pod *corev1.Pod, containerName string) (string, error) {
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.Name != containerName {
			continue
		}
		// #52 if a pod is running but not ready(because of readiness probe), we can connect
		if containerStatus.State.Running == nil {
			return "", fmt.Errorf("container [%s] not running", containerName)
		}
		return containerStatus.ContainerID, nil
	}

	// #14 otherwise we should search for running init containers
	for _, initContainerStatus := range pod.Status.InitContainerStatuses {
		if initContainerStatus.Name != containerName {
			continue
		}
		if initContainerStatus.State.Running == nil {
			return "", fmt.Errorf("init container [%s] is not running", containerName)
		}
		return initContainerStatus.ContainerID, nil
	}

	return "", fmt.Errorf("cannot find specified container %s", containerName)
}

func (cli *KubeClient) remoteExecute(
	method string,
	url *url.URL,
	stdin io.Reader,
	stdout, stderr io.Writer,
	tty bool,
	terminalSizeQueue remotecommand.TerminalSizeQueue) error {

	exec, err := remotecommand.NewSPDYExecutor(cli.RestConfig, method, url)
	if err != nil {
		return err
	}

	return exec.Stream(remotecommand.StreamOptions{
		Stdin:             stdin,
		Stdout:            stdout,
		Stderr:            stderr,
		Tty:               tty,
		TerminalSizeQueue: terminalSizeQueue,
	})

}

func (cli *KubeClient) launchController(nodeName string) (*corev1.Pod, error) {
	ctrlPod, err := cli.getPodByName(defaultCtrlPodNs, defaultCtrlPodName)
	if err != nil {
		return nil, err
	}
	if ctrlPod != nil {
		return ctrlPod, nil
	} else {
		ctrlPod = getCtrlPod(nodeName)

		ctrlPod, err := cli.CoreClient.Pods(defaultCtrlPodNs).Create(ctrlPod)
		if err != nil {
			return ctrlPod, err
		}

		watcher, err := cli.CoreClient.Pods(defaultCtrlPodNs).Watch(metaV1.SingleObject(ctrlPod.ObjectMeta))
		if err != nil {
			return nil, err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		fmt.Fprintf(o.Out, "Waiting for pod %s to run...\n", ctrlPod.Name)
		event, err := watch.UntilWithoutRetry(ctx, watcher, conditions.PodRunning)
		if err != nil {
			fmt.Fprintf(o.ErrOut, "Error occurred while waiting for pod to run:  %v\n", err)
			return nil, err
		}
		ctrlPod = event.Object.(*corev1.Pod)
		return ctrlPod, nil

	}
}
