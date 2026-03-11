package resources

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	edgelakev1alpha1 "github.com/trv-edgelake-infra/kube-operator-go/api/v1alpha1"
)

// BuildService creates a Service for EdgeLake
func BuildService(cr *edgelakev1alpha1.EdgeLakeOperator) *corev1.Service {
	labels := Labels(cr)
	selectorLabels := SelectorLabels(cr)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Spec.Metadata.ServiceName,
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     cr.Spec.Metadata.ServiceType,
			Selector: selectorLabels,
			Ports:    buildServicePorts(cr),
		},
	}

	return service
}

// buildServicePorts creates service port definitions
func buildServicePorts(cr *edgelakev1alpha1.EdgeLakeOperator) []corev1.ServicePort {
	ports := []corev1.ServicePort{
		{
			Name:       "tcp-server",
			Protocol:   corev1.ProtocolTCP,
			Port:       cr.Spec.NodeConfigs.Networking.AnyLogServerPort,
			TargetPort: intstr.FromInt(int(cr.Spec.NodeConfigs.Networking.AnyLogServerPort)),
		},
		{
			Name:       "rest-api",
			Protocol:   corev1.ProtocolTCP,
			Port:       cr.Spec.NodeConfigs.Networking.AnyLogRESTPort,
			TargetPort: intstr.FromInt(int(cr.Spec.NodeConfigs.Networking.AnyLogRESTPort)),
		},
	}

	// Add NodePort for NodePort/LoadBalancer services
	if cr.Spec.Metadata.ServiceType == corev1.ServiceTypeNodePort || cr.Spec.Metadata.ServiceType == corev1.ServiceTypeLoadBalancer {
		ports[0].NodePort = cr.Spec.NodeConfigs.Networking.AnyLogServerPort
		ports[1].NodePort = cr.Spec.NodeConfigs.Networking.AnyLogRESTPort
	}

	// Add broker port if specified
	if cr.Spec.NodeConfigs.Networking.AnyLogBrokerPort > 0 {
		brokerPort := corev1.ServicePort{
			Name:       "mqtt-broker",
			Protocol:   corev1.ProtocolTCP,
			Port:       cr.Spec.NodeConfigs.Networking.AnyLogBrokerPort,
			TargetPort: intstr.FromInt(int(cr.Spec.NodeConfigs.Networking.AnyLogBrokerPort)),
		}
		if cr.Spec.Metadata.ServiceType == corev1.ServiceTypeNodePort || cr.Spec.Metadata.ServiceType == corev1.ServiceTypeLoadBalancer {
			brokerPort.NodePort = cr.Spec.NodeConfigs.Networking.AnyLogBrokerPort
		}
		ports = append(ports, brokerPort)
	}

	return ports
}
