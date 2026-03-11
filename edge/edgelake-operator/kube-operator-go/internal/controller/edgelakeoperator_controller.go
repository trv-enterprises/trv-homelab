package controller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	edgelakev1alpha1 "github.com/trv-edgelake-infra/kube-operator-go/api/v1alpha1"
	"github.com/trv-edgelake-infra/kube-operator-go/internal/resources"
)

const (
	// FinalizerName is the name of the finalizer
	FinalizerName = "edgelake.trv.io/finalizer"

	// Condition types
	ConditionTypeReady      = "Ready"
	ConditionTypeConfigured = "Configured"
	ConditionTypeDeployed   = "Deployed"

	// Phase values
	PhasePending  = "Pending"
	PhaseCreating = "Creating"
	PhaseRunning  = "Running"
	PhaseFailed   = "Failed"
)

// EdgeLakeOperatorReconciler reconciles a EdgeLakeOperator object
type EdgeLakeOperatorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=edgelake.trv.io,resources=edgelakeoperators,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=edgelake.trv.io,resources=edgelakeoperators/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=edgelake.trv.io,resources=edgelakeoperators/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

// Reconcile is the main reconciliation loop
func (r *EdgeLakeOperatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling EdgeLakeOperator", "name", req.Name, "namespace", req.Namespace)

	// Fetch the EdgeLakeOperator instance
	edgelake := &edgelakev1alpha1.EdgeLakeOperator{}
	if err := r.Get(ctx, req.NamespacedName, edgelake); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("EdgeLakeOperator resource not found, ignoring")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get EdgeLakeOperator")
		return ctrl.Result{}, err
	}

	// Set defaults
	r.setDefaults(edgelake)

	// Handle deletion
	if !edgelake.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, edgelake)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(edgelake, FinalizerName) {
		controllerutil.AddFinalizer(edgelake, FinalizerName)
		if err := r.Update(ctx, edgelake); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Update status phase
	if edgelake.Status.Phase == "" {
		edgelake.Status.Phase = PhasePending
		if err := r.Status().Update(ctx, edgelake); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile resources
	result, err := r.reconcileResources(ctx, edgelake)
	if err != nil {
		// Update status to failed
		edgelake.Status.Phase = PhaseFailed
		meta.SetStatusCondition(&edgelake.Status.Conditions, metav1.Condition{
			Type:               ConditionTypeReady,
			Status:             metav1.ConditionFalse,
			Reason:             "ReconciliationFailed",
			Message:            err.Error(),
			LastTransitionTime: metav1.Now(),
		})
		_ = r.Status().Update(ctx, edgelake)
		return result, err
	}

	// Update status
	if err := r.updateStatus(ctx, edgelake); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	logger.Info("Successfully reconciled EdgeLakeOperator")
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// setDefaults sets default values for the EdgeLakeOperator spec
func (r *EdgeLakeOperatorReconciler) setDefaults(edgelake *edgelakev1alpha1.EdgeLakeOperator) {
	if edgelake.Spec.Metadata == nil {
		edgelake.Spec.Metadata = &edgelakev1alpha1.MetadataSpec{}
	}
	if edgelake.Spec.Metadata.Hostname == "" {
		edgelake.Spec.Metadata.Hostname = "edgelake-operator"
	}
	if edgelake.Spec.Metadata.AppName == "" {
		edgelake.Spec.Metadata.AppName = edgelake.Name
	}
	if edgelake.Spec.Metadata.ServiceName == "" {
		edgelake.Spec.Metadata.ServiceName = fmt.Sprintf("%s-service", edgelake.Name)
	}
	if edgelake.Spec.Metadata.ConfigMapName == "" {
		edgelake.Spec.Metadata.ConfigMapName = fmt.Sprintf("%s-configmap", edgelake.Name)
	}
	if edgelake.Spec.Metadata.ServiceType == "" {
		edgelake.Spec.Metadata.ServiceType = corev1.ServiceTypeNodePort
	}

	if edgelake.Spec.Image == nil {
		edgelake.Spec.Image = &edgelakev1alpha1.ImageSpec{}
	}
	if edgelake.Spec.Image.Repository == "" {
		edgelake.Spec.Image.Repository = "anylogco/edgelake-network"
	}
	if edgelake.Spec.Image.Tag == "" {
		edgelake.Spec.Image.Tag = "1.3.2500"
	}
	if edgelake.Spec.Image.PullPolicy == "" {
		edgelake.Spec.Image.PullPolicy = corev1.PullIfNotPresent
	}

	if edgelake.Spec.Persistence == nil {
		edgelake.Spec.Persistence = &edgelakev1alpha1.PersistenceSpec{
			Enabled:    true,
			AccessMode: corev1.ReadWriteOnce,
		}
	}
	if edgelake.Spec.Persistence.AnyLog == nil {
		edgelake.Spec.Persistence.AnyLog = &edgelakev1alpha1.VolumeSpec{Size: "5Gi"}
	}
	if edgelake.Spec.Persistence.Blockchain == nil {
		edgelake.Spec.Persistence.Blockchain = &edgelakev1alpha1.VolumeSpec{Size: "1Gi"}
	}
	if edgelake.Spec.Persistence.Data == nil {
		edgelake.Spec.Persistence.Data = &edgelakev1alpha1.VolumeSpec{Size: "10Gi"}
	}
	if edgelake.Spec.Persistence.Scripts == nil {
		edgelake.Spec.Persistence.Scripts = &edgelakev1alpha1.VolumeSpec{Size: "1Gi"}
	}

	if edgelake.Spec.NodeConfigs == nil {
		edgelake.Spec.NodeConfigs = &edgelakev1alpha1.NodeConfigsSpec{}
	}
	r.setNodeConfigDefaults(edgelake.Spec.NodeConfigs)
}

// setNodeConfigDefaults sets defaults for node configuration
func (r *EdgeLakeOperatorReconciler) setNodeConfigDefaults(nc *edgelakev1alpha1.NodeConfigsSpec) {
	if nc.Directories == nil {
		nc.Directories = &edgelakev1alpha1.DirectoriesConfig{}
	}
	if nc.Directories.AnyLogPath == "" {
		nc.Directories.AnyLogPath = "/app"
	}
	if nc.Directories.LocalScripts == "" {
		nc.Directories.LocalScripts = "/app/deployment-scripts/node-deployment"
	}
	if nc.Directories.TestDir == "" {
		nc.Directories.TestDir = "/app/deployment-scripts/tests"
	}

	if nc.General == nil {
		nc.General = &edgelakev1alpha1.GeneralConfig{}
	}
	if nc.General.NodeType == "" {
		nc.General.NodeType = "operator"
	}
	if nc.General.CompanyName == "" {
		nc.General.CompanyName = "New Company"
	}

	if nc.Networking == nil {
		nc.Networking = &edgelakev1alpha1.NetworkingConfig{}
	}
	if nc.Networking.AnyLogServerPort == 0 {
		nc.Networking.AnyLogServerPort = 32148
	}
	if nc.Networking.AnyLogRESTPort == 0 {
		nc.Networking.AnyLogRESTPort = 32149
	}
	if nc.Networking.TCPThreads == 0 {
		nc.Networking.TCPThreads = 6
	}
	if nc.Networking.RESTTimeout == 0 {
		nc.Networking.RESTTimeout = 30
	}
	if nc.Networking.RESTThreads == 0 {
		nc.Networking.RESTThreads = 6
	}
	if nc.Networking.BrokerThreads == 0 {
		nc.Networking.BrokerThreads = 6
	}

	if nc.Database == nil {
		nc.Database = &edgelakev1alpha1.DatabaseConfig{}
	}
	if nc.Database.DBType == "" {
		nc.Database.DBType = "sqlite"
	}
	if nc.Database.DBIP == "" {
		nc.Database.DBIP = "127.0.0.1"
	}
	if nc.Database.DBPort == 0 {
		nc.Database.DBPort = 5432
	}
	if nc.Database.NoSQLType == "" {
		nc.Database.NoSQLType = "mongo"
	}
	if nc.Database.NoSQLIP == "" {
		nc.Database.NoSQLIP = "127.0.0.1"
	}
	if nc.Database.NoSQLPort == 0 {
		nc.Database.NoSQLPort = 27017
	}

	if nc.Blockchain == nil {
		nc.Blockchain = &edgelakev1alpha1.BlockchainConfig{}
	}
	if nc.Blockchain.LedgerConn == "" {
		nc.Blockchain.LedgerConn = "127.0.0.1:32048"
	}
	if nc.Blockchain.SyncTime == "" {
		nc.Blockchain.SyncTime = "30 second"
	}
	if nc.Blockchain.BlockchainSync == "" {
		nc.Blockchain.BlockchainSync = "30 second"
	}
	if nc.Blockchain.BlockchainSource == "" {
		nc.Blockchain.BlockchainSource = "master"
	}
	if nc.Blockchain.BlockchainDestination == "" {
		nc.Blockchain.BlockchainDestination = "file"
	}

	if nc.Operator == nil {
		nc.Operator = &edgelakev1alpha1.OperatorConfig{}
	}
	if nc.Operator.ClusterName == "" {
		nc.Operator.ClusterName = "new-company-cluster1"
	}
	if nc.Operator.DefaultDBMS == "" {
		nc.Operator.DefaultDBMS = "new_company"
	}
	if nc.Operator.StartDate == 0 {
		nc.Operator.StartDate = 30
	}
	if nc.Operator.OperatorThreads == 0 {
		nc.Operator.OperatorThreads = 3
	}
	if nc.Operator.TableName == "" {
		nc.Operator.TableName = "*"
	}
	if nc.Operator.PartitionColumn == "" {
		nc.Operator.PartitionColumn = "insert_timestamp"
	}
	if nc.Operator.PartitionInterval == "" {
		nc.Operator.PartitionInterval = "14 days"
	}
	if nc.Operator.PartitionKeep == 0 {
		nc.Operator.PartitionKeep = 3
	}
	if nc.Operator.PartitionSync == "" {
		nc.Operator.PartitionSync = "1 day"
	}

	if nc.MQTT == nil {
		nc.MQTT = &edgelakev1alpha1.MQTTConfig{}
	}
	if nc.MQTT.MQTTPort == 0 {
		nc.MQTT.MQTTPort = 1883
	}
	if nc.MQTT.MsgDBMS == "" {
		nc.MQTT.MsgDBMS = "new_company"
	}
	if nc.MQTT.MsgTable == "" {
		nc.MQTT.MsgTable = "bring [table]"
	}
	if nc.MQTT.MsgTimestampColumn == "" {
		nc.MQTT.MsgTimestampColumn = "bring [timestamp]"
	}
	if nc.MQTT.MsgValueColumn == "" {
		nc.MQTT.MsgValueColumn = "bring [value]"
	}
	if nc.MQTT.MsgValueColumnType == "" {
		nc.MQTT.MsgValueColumnType = "float"
	}

	if nc.Aggregations == nil {
		nc.Aggregations = &edgelakev1alpha1.AggregationsConfig{}
	}
	if nc.Aggregations.AggregationTimeColumn == "" {
		nc.Aggregations.AggregationTimeColumn = "insert_timestamp"
	}
	if nc.Aggregations.AggregationValueColumn == "" {
		nc.Aggregations.AggregationValueColumn = "value"
	}

	if nc.Advanced == nil {
		nc.Advanced = &edgelakev1alpha1.AdvancedConfig{}
	}
	if nc.Advanced.QueryPool == 0 {
		nc.Advanced.QueryPool = 6
	}
	if nc.Advanced.ThresholdTime == "" {
		nc.Advanced.ThresholdTime = "60 seconds"
	}
	if nc.Advanced.ThresholdVolume == "" {
		nc.Advanced.ThresholdVolume = "100KB"
	}
}

// handleDeletion handles the deletion of the EdgeLakeOperator
func (r *EdgeLakeOperatorReconciler) handleDeletion(ctx context.Context, edgelake *edgelakev1alpha1.EdgeLakeOperator) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Handling deletion of EdgeLakeOperator")

	if controllerutil.ContainsFinalizer(edgelake, FinalizerName) {
		// Perform cleanup
		// Resources owned by this CR will be garbage collected automatically
		// due to owner references

		// Remove finalizer
		controllerutil.RemoveFinalizer(edgelake, FinalizerName)
		if err := r.Update(ctx, edgelake); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// reconcileResources reconciles all resources for the EdgeLakeOperator
func (r *EdgeLakeOperatorReconciler) reconcileResources(ctx context.Context, edgelake *edgelakev1alpha1.EdgeLakeOperator) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Update phase to creating
	edgelake.Status.Phase = PhaseCreating
	_ = r.Status().Update(ctx, edgelake)

	// Reconcile ConfigMap
	if err := r.reconcileConfigMap(ctx, edgelake); err != nil {
		logger.Error(err, "Failed to reconcile ConfigMap")
		return ctrl.Result{}, err
	}

	// Reconcile PVCs if persistence is enabled
	if edgelake.Spec.Persistence != nil && edgelake.Spec.Persistence.Enabled {
		if err := r.reconcilePVCs(ctx, edgelake); err != nil {
			logger.Error(err, "Failed to reconcile PVCs")
			return ctrl.Result{}, err
		}
	}

	// Reconcile Service
	if err := r.reconcileService(ctx, edgelake); err != nil {
		logger.Error(err, "Failed to reconcile Service")
		return ctrl.Result{}, err
	}

	// Reconcile Deployment
	if err := r.reconcileDeployment(ctx, edgelake); err != nil {
		logger.Error(err, "Failed to reconcile Deployment")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcileConfigMap ensures the ConfigMap exists with correct configuration
func (r *EdgeLakeOperatorReconciler) reconcileConfigMap(ctx context.Context, edgelake *edgelakev1alpha1.EdgeLakeOperator) error {
	logger := log.FromContext(ctx)

	configMap := resources.BuildConfigMap(edgelake)

	// Set owner reference
	if err := controllerutil.SetControllerReference(edgelake, configMap, r.Scheme); err != nil {
		return err
	}

	// Check if ConfigMap exists
	found := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating ConfigMap", "name", configMap.Name)
		return r.Create(ctx, configMap)
	} else if err != nil {
		return err
	}

	// Update if needed
	found.Data = configMap.Data
	return r.Update(ctx, found)
}

// reconcilePVCs ensures PVCs exist
func (r *EdgeLakeOperatorReconciler) reconcilePVCs(ctx context.Context, edgelake *edgelakev1alpha1.EdgeLakeOperator) error {
	logger := log.FromContext(ctx)

	pvcs := resources.BuildPVCs(edgelake)
	for _, pvc := range pvcs {
		// Set owner reference
		if err := controllerutil.SetControllerReference(edgelake, pvc, r.Scheme); err != nil {
			return err
		}

		// Check if PVC exists
		found := &corev1.PersistentVolumeClaim{}
		err := r.Get(ctx, types.NamespacedName{Name: pvc.Name, Namespace: pvc.Namespace}, found)
		if err != nil && errors.IsNotFound(err) {
			logger.Info("Creating PVC", "name", pvc.Name)
			if err := r.Create(ctx, pvc); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		// PVCs are immutable after creation, so we don't update
	}

	return nil
}

// reconcileService ensures the Service exists
func (r *EdgeLakeOperatorReconciler) reconcileService(ctx context.Context, edgelake *edgelakev1alpha1.EdgeLakeOperator) error {
	logger := log.FromContext(ctx)

	service := resources.BuildService(edgelake)

	// Set owner reference
	if err := controllerutil.SetControllerReference(edgelake, service, r.Scheme); err != nil {
		return err
	}

	// Check if Service exists
	found := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating Service", "name", service.Name)
		return r.Create(ctx, service)
	} else if err != nil {
		return err
	}

	// Update Service spec (preserve ClusterIP)
	found.Spec.Ports = service.Spec.Ports
	found.Spec.Type = service.Spec.Type
	found.Spec.Selector = service.Spec.Selector
	return r.Update(ctx, found)
}

// reconcileDeployment ensures the Deployment exists
func (r *EdgeLakeOperatorReconciler) reconcileDeployment(ctx context.Context, edgelake *edgelakev1alpha1.EdgeLakeOperator) error {
	logger := log.FromContext(ctx)

	deployment := resources.BuildDeployment(edgelake)

	// Set owner reference
	if err := controllerutil.SetControllerReference(edgelake, deployment, r.Scheme); err != nil {
		return err
	}

	// Check if Deployment exists
	found := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating Deployment", "name", deployment.Name)
		return r.Create(ctx, deployment)
	} else if err != nil {
		return err
	}

	// Update Deployment
	found.Spec = deployment.Spec
	return r.Update(ctx, found)
}

// updateStatus updates the status of the EdgeLakeOperator
func (r *EdgeLakeOperatorReconciler) updateStatus(ctx context.Context, edgelake *edgelakev1alpha1.EdgeLakeOperator) error {
	// Check deployment status
	deployment := &appsv1.Deployment{}
	deploymentName := fmt.Sprintf("%s-deployment", edgelake.Spec.Metadata.AppName)
	err := r.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: edgelake.Namespace}, deployment)
	if err != nil {
		if errors.IsNotFound(err) {
			edgelake.Status.Phase = PhaseCreating
			edgelake.Status.Ready = false
		} else {
			return err
		}
	} else {
		// Check if deployment is ready
		if deployment.Status.ReadyReplicas > 0 && deployment.Status.ReadyReplicas == deployment.Status.Replicas {
			edgelake.Status.Phase = PhaseRunning
			edgelake.Status.Ready = true
			meta.SetStatusCondition(&edgelake.Status.Conditions, metav1.Condition{
				Type:               ConditionTypeReady,
				Status:             metav1.ConditionTrue,
				Reason:             "DeploymentReady",
				Message:            "EdgeLake deployment is ready",
				LastTransitionTime: metav1.Now(),
			})
		} else {
			edgelake.Status.Phase = PhaseCreating
			edgelake.Status.Ready = false
			meta.SetStatusCondition(&edgelake.Status.Conditions, metav1.Condition{
				Type:               ConditionTypeReady,
				Status:             metav1.ConditionFalse,
				Reason:             "DeploymentNotReady",
				Message:            "EdgeLake deployment is not ready",
				LastTransitionTime: metav1.Now(),
			})
		}
	}

	// Update resource names in status
	edgelake.Status.DeploymentName = deploymentName
	edgelake.Status.ServiceName = edgelake.Spec.Metadata.ServiceName
	edgelake.Status.ConfigMapName = edgelake.Spec.Metadata.ConfigMapName
	edgelake.Status.ObservedGeneration = edgelake.Generation

	return r.Status().Update(ctx, edgelake)
}

// SetupWithManager sets up the controller with the Manager.
func (r *EdgeLakeOperatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&edgelakev1alpha1.EdgeLakeOperator{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
