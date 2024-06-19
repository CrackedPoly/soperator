package worker

import (
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	slurmv1 "nebius.ai/slurm-operator/api/v1"
	"nebius.ai/slurm-operator/internal/consts"
	"nebius.ai/slurm-operator/internal/render/common"
	"nebius.ai/slurm-operator/internal/values"
)

// renderContainerToolkitValidation renders init [corev1.Container] for toolkit validation
func renderContainerToolkitValidation(container *values.Container) corev1.Container {
	return corev1.Container{
		Name:            consts.ContainerNameToolkitValidation,
		Image:           container.Image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command: []string{
			"sh",
		},
		Args: []string{
			"-c",
			strings.Join(
				[]string{
					fmt.Sprintf("until [ -f %s/validations/toolkit-ready ]; do", consts.VolumeMountPathNvidia),
					"echo 'waiting for nvidia container stack to be setup';",
					"sleep 5;",
					"done",
				},
				" ",
			),
		},
		VolumeMounts: []corev1.VolumeMount{
			renderVolumeMountNvidia(),
		},
		TerminationMessagePolicy: corev1.TerminationMessageReadFile,
		TerminationMessagePath:   "/dev/termination-log",
	}
}

// renderContainerSlurmd renders [corev1.Container] for slurmd
func renderContainerSlurmd(
	container *values.Container,
	maxGPU int32,
	jailSubMounts []slurmv1.NodeVolumeJailSubMount,
) corev1.Container {
	volumeMounts := []corev1.VolumeMount{
		common.RenderVolumeMountSlurmConfigs(),
		common.RenderVolumeMountSpool(consts.ComponentTypeWorker, consts.SlurmdName),
		common.RenderVolumeMountJail(),
		common.RenderVolumeMountMungeSocket(),
		renderVolumeMountNvidia(),
		renderVolumeMountBoot(),
		renderVolumeMountNCCLTopology(),
		renderVolumeMountSharedMemory(),
		renderVolumeMountSysctl(),
	}
	volumeMounts = append(volumeMounts, common.RenderVolumeMountsForJailSubMounts(jailSubMounts)...)

	return corev1.Container{
		Name:            consts.ContainerNameSlurmd,
		Image:           container.Image,
		ImagePullPolicy: corev1.PullAlways, // TODO use digest and set to corev1.PullIfNotPresent
		Env: []corev1.EnvVar{
			{
				Name: "K8S_POD_NAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.name",
					},
				},
			}, {
				Name: "K8S_POD_NAMESPACE",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.namespace",
					},
				},
			},
		},
		Ports: []corev1.ContainerPort{{
			Name:          container.Name,
			ContainerPort: container.Port,
			Protocol:      corev1.ProtocolTCP,
		}},
		VolumeMounts: volumeMounts,
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"/bin/sh",
						"-c",
						"/usr/bin/sinfo > /dev/null && exit 0 || exit 1",
					},
				},
			},
			PeriodSeconds: 30,
		},
		SecurityContext: &corev1.SecurityContext{
			Privileged: ptr.To(true),
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{
					consts.ContainerSecurityContextCapabilitySysAdmin,
				},
			},
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeUnconfined,
			},
			ProcMount: ptr.To(corev1.UnmaskedProcMount),
		},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:              container.Resources.CPU,
				corev1.ResourceMemory:           container.Resources.Memory,
				corev1.ResourceEphemeralStorage: container.Resources.EphemeralStorage,
				consts.AnnotationMaxGPU:         resource.MustParse(strconv.Itoa(int(maxGPU))),
			},
		},
	}
}