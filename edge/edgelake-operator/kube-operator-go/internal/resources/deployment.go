package resources

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	edgelakev1alpha1 "github.com/trv-edgelake-infra/kube-operator-go/api/v1alpha1"
)

// BuildDeployment creates a Deployment for EdgeLake
func BuildDeployment(cr *edgelakev1alpha1.EdgeLakeOperator) *appsv1.Deployment {
	labels := Labels(cr)
	selectorLabels := SelectorLabels(cr)

	replicas := int32(1)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-deployment", cr.Spec.Metadata.AppName),
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: selectorLabels,
				},
				Spec: buildPodSpec(cr),
			},
		},
	}

	return deployment
}

// buildPodSpec creates the Pod spec for the EdgeLake container
func buildPodSpec(cr *edgelakev1alpha1.EdgeLakeOperator) corev1.PodSpec {
	spec := corev1.PodSpec{
		Containers: []corev1.Container{
			buildContainer(cr),
		},
		Volumes: buildVolumes(cr),
	}

	// Add node selector if specified
	if cr.Spec.Metadata != nil && len(cr.Spec.Metadata.NodeSelector) > 0 {
		spec.NodeSelector = cr.Spec.Metadata.NodeSelector
	}

	// Add image pull secret if specified
	if cr.Spec.Image != nil && cr.Spec.Image.SecretName != "" {
		spec.ImagePullSecrets = []corev1.LocalObjectReference{
			{Name: cr.Spec.Image.SecretName},
		}
	}

	return spec
}

// buildContainer creates the EdgeLake container
func buildContainer(cr *edgelakev1alpha1.EdgeLakeOperator) corev1.Container {
	image := fmt.Sprintf("%s:%s", cr.Spec.Image.Repository, cr.Spec.Image.Tag)

	container := corev1.Container{
		Name:            fmt.Sprintf("%s-container", cr.Spec.Metadata.Hostname),
		Image:           image,
		ImagePullPolicy: cr.Spec.Image.PullPolicy,
		Ports:           buildContainerPorts(cr),
		EnvFrom: []corev1.EnvFromSource{
			{
				ConfigMapRef: &corev1.ConfigMapEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cr.Spec.Metadata.ConfigMapName,
					},
				},
			},
		},
		TTY:          true,
		Stdin:        true,
		VolumeMounts: buildVolumeMounts(cr),
		// Add liveness probe
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(int(cr.Spec.NodeConfigs.Networking.AnyLogRESTPort)),
				},
			},
			InitialDelaySeconds: 60,
			PeriodSeconds:       30,
			TimeoutSeconds:      5,
			FailureThreshold:    3,
		},
		// Add readiness probe
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(int(cr.Spec.NodeConfigs.Networking.AnyLogRESTPort)),
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
			TimeoutSeconds:      5,
			FailureThreshold:    3,
		},
	}

	// Add resources if specified
	if cr.Spec.Resources != nil {
		container.Resources = *cr.Spec.Resources
	}

	return container
}

// buildContainerPorts creates container port definitions
func buildContainerPorts(cr *edgelakev1alpha1.EdgeLakeOperator) []corev1.ContainerPort {
	ports := []corev1.ContainerPort{
		{
			Name:          "tcp-server",
			ContainerPort: cr.Spec.NodeConfigs.Networking.AnyLogServerPort,
			Protocol:      corev1.ProtocolTCP,
		},
		{
			Name:          "rest-api",
			ContainerPort: cr.Spec.NodeConfigs.Networking.AnyLogRESTPort,
			Protocol:      corev1.ProtocolTCP,
		},
	}

	// Add broker port if specified
	if cr.Spec.NodeConfigs.Networking.AnyLogBrokerPort > 0 {
		ports = append(ports, corev1.ContainerPort{
			Name:          "mqtt-broker",
			ContainerPort: cr.Spec.NodeConfigs.Networking.AnyLogBrokerPort,
			Protocol:      corev1.ProtocolTCP,
		})
	}

	return ports
}

// buildVolumeMounts creates volume mounts for the container
func buildVolumeMounts(cr *edgelakev1alpha1.EdgeLakeOperator) []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "anylog-volume",
			MountPath: "/app/EdgeLake/anylog",
		},
		{
			Name:      "blockchain-volume",
			MountPath: "/app/EdgeLake/blockchain",
		},
		{
			Name:      "data-volume",
			MountPath: "/app/EdgeLake/data",
		},
		{
			Name:      "scripts-volume",
			MountPath: "/app/deployment-scripts",
		},
	}
}

// buildVolumes creates volume definitions
func buildVolumes(cr *edgelakev1alpha1.EdgeLakeOperator) []corev1.Volume {
	if cr.Spec.Persistence != nil && cr.Spec.Persistence.Enabled {
		return []corev1.Volume{
			{
				Name: "anylog-volume",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: fmt.Sprintf("%s-anylog-pvc", cr.Spec.Metadata.AppName),
					},
				},
			},
			{
				Name: "blockchain-volume",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: fmt.Sprintf("%s-blockchain-pvc", cr.Spec.Metadata.AppName),
					},
				},
			},
			{
				Name: "data-volume",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: fmt.Sprintf("%s-data-pvc", cr.Spec.Metadata.AppName),
					},
				},
			},
			{
				Name: "scripts-volume",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: fmt.Sprintf("%s-scripts-pvc", cr.Spec.Metadata.AppName),
					},
				},
			},
		}
	}

	// Use emptyDir if persistence is disabled
	return []corev1.Volume{
		{
			Name: "anylog-volume",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "blockchain-volume",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "data-volume",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "scripts-volume",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}
}
