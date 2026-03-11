package resources

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	edgelakev1alpha1 "github.com/trv-edgelake-infra/kube-operator-go/api/v1alpha1"
)

// PVCConfig holds configuration for a single PVC
type PVCConfig struct {
	Name string
	Size string
}

// BuildPVCs creates PersistentVolumeClaims for EdgeLake
func BuildPVCs(cr *edgelakev1alpha1.EdgeLakeOperator) []*corev1.PersistentVolumeClaim {
	if cr.Spec.Persistence == nil || !cr.Spec.Persistence.Enabled {
		return nil
	}

	labels := Labels(cr)
	pvcs := []*corev1.PersistentVolumeClaim{}

	// Define PVC configurations
	pvcConfigs := []PVCConfig{
		{
			Name: fmt.Sprintf("%s-anylog-pvc", cr.Spec.Metadata.AppName),
			Size: cr.Spec.Persistence.AnyLog.Size,
		},
		{
			Name: fmt.Sprintf("%s-blockchain-pvc", cr.Spec.Metadata.AppName),
			Size: cr.Spec.Persistence.Blockchain.Size,
		},
		{
			Name: fmt.Sprintf("%s-data-pvc", cr.Spec.Metadata.AppName),
			Size: cr.Spec.Persistence.Data.Size,
		},
		{
			Name: fmt.Sprintf("%s-scripts-pvc", cr.Spec.Metadata.AppName),
			Size: cr.Spec.Persistence.Scripts.Size,
		},
	}

	for _, config := range pvcConfigs {
		pvc := buildPVC(cr, config, labels)
		pvcs = append(pvcs, pvc)
	}

	return pvcs
}

// buildPVC creates a single PVC
func buildPVC(cr *edgelakev1alpha1.EdgeLakeOperator, config PVCConfig, labels map[string]string) *corev1.PersistentVolumeClaim {
	accessMode := cr.Spec.Persistence.AccessMode
	if accessMode == "" {
		accessMode = corev1.ReadWriteOnce
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.Name,
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{accessMode},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(config.Size),
				},
			},
		},
	}

	// Set storage class if specified
	if cr.Spec.Persistence.StorageClassName != "" {
		pvc.Spec.StorageClassName = &cr.Spec.Persistence.StorageClassName
	}

	return pvc
}
