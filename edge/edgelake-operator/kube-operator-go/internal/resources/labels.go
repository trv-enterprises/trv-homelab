package resources

import (
	edgelakev1alpha1 "github.com/trv-edgelake-infra/kube-operator-go/api/v1alpha1"
)

// Labels returns the common labels for EdgeLake resources
func Labels(cr *edgelakev1alpha1.EdgeLakeOperator) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "edgelake-operator",
		"app.kubernetes.io/instance":   cr.Name,
		"app.kubernetes.io/version":    cr.Spec.Image.Tag,
		"app.kubernetes.io/component":  "edgelake",
		"app.kubernetes.io/managed-by": "edgelake-operator",
		"app":                          cr.Spec.Metadata.AppName,
	}
}

// SelectorLabels returns the selector labels for EdgeLake resources
func SelectorLabels(cr *edgelakev1alpha1.EdgeLakeOperator) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     "edgelake-operator",
		"app.kubernetes.io/instance": cr.Name,
		"app":                        cr.Spec.Metadata.AppName,
	}
}
