package client

import (
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const defaultCtrlPodName = "kadd-controller"
const defaultCtrlPodNs = "default"
const defaultCtrlImage = "ejunjsh/kadd-controller:1.0"
const defaultCtrlPort = 8787

func getCtrlPod(nodeName string) *corev1.Pod {

	prop := corev1.MountPropagationBidirectional
	directoryCreate := corev1.HostPathDirectoryOrCreate
	priveleged := true
	ctrlPod := &corev1.Pod{
		TypeMeta: v1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      defaultCtrlPodName + "-" + nodeName,
			Namespace: defaultCtrlPodNs,
		},
		Spec: corev1.PodSpec{
			HostPID:  true,
			NodeName: nodeName,
			Containers: []corev1.Container{
				{
					Name:            defaultCtrlPodName + "-" + nodeName,
					Image:           defaultCtrlImage,
					ImagePullPolicy: corev1.PullIfNotPresent,
					LivenessProbe: &corev1.Probe{
						Handler: corev1.Handler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/health",
								Port: intstr.FromInt(defaultCtrlPort),
							},
						},
						InitialDelaySeconds: 10,
						PeriodSeconds:       10,
						SuccessThreshold:    1,
						TimeoutSeconds:      1,
						FailureThreshold:    3,
					},
					SecurityContext: &corev1.SecurityContext{
						Privileged: &priveleged,
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "docker",
							MountPath: "/var/run/docker.sock",
						},
						{
							Name:      "cgroup",
							MountPath: "/sys/fs/cgroup",
						},
						{
							Name:      "vardata",
							MountPath: "/var/data",
						},
						{
							Name:      "runcontainerd",
							MountPath: "/run/containerd",
						},
						{
							Name:      "runrunc",
							MountPath: "/run/runc",
						},
						{
							Name:             "lxcfs",
							MountPath:        "/var/lib/lxc",
							MountPropagation: &prop,
						},
					},
					Ports: []corev1.ContainerPort{
						{
							Name:          "http",
							HostPort:      defaultCtrlPort,
							ContainerPort: defaultCtrlPort,
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "docker",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: "/var/run/docker.sock",
						},
					},
				},
				{
					Name: "cgroup",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: "/sys/fs/cgroup",
						},
					},
				},
				{
					Name: "lxcfs",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: "/var/lib/lxc",
							Type: &directoryCreate,
						},
					},
				},
				{
					Name: "vardata",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: "/var/data",
						},
					},
				},
				{
					Name: "runcontainerd",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: "/run/containerd",
						},
					},
				},
				{
					Name: "runrunc",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: "/run/runc",
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}
	return ctrlPod
}
