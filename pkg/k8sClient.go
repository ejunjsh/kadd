package pkg

import (
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
)

type KubeClient struct {
	clientset *kubernetes.Clientset
}

func NewKubeClient() *KubeClient {
	flags := genericclioptions.NewConfigFlags()
	configLoader := flags.ToRawKubeConfigLoader()
	namespace, _, _ := configLoader.Namespace()
	config, _ := configLoader.ClientConfig()
	clientset, _ := kubernetes.NewForConfig(config)
	return &KubeClient{
		clientset: clientset,
	}
}
