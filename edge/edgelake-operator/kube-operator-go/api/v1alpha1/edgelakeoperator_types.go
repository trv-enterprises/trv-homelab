package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EdgeLakeOperatorSpec defines the desired state of EdgeLakeOperator
type EdgeLakeOperatorSpec struct {
	// Metadata configuration for Kubernetes resources
	// +optional
	Metadata *MetadataSpec `json:"metadata,omitempty"`

	// Image configuration for the EdgeLake container
	// +optional
	Image *ImageSpec `json:"image,omitempty"`

	// Persistence configuration for volumes
	// +optional
	Persistence *PersistenceSpec `json:"persistence,omitempty"`

	// Resources defines compute resources for the container
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// NodeConfigs contains EdgeLake node configuration
	// +optional
	NodeConfigs *NodeConfigsSpec `json:"nodeConfigs,omitempty"`
}

// MetadataSpec defines Kubernetes metadata configuration
type MetadataSpec struct {
	// Hostname for the deployment
	// +kubebuilder:default="edgelake-operator"
	// +optional
	Hostname string `json:"hostname,omitempty"`

	// AppName is the application name
	// +kubebuilder:default="edgelake-operator"
	// +optional
	AppName string `json:"appName,omitempty"`

	// ServiceName for the Kubernetes service
	// +kubebuilder:default="edgelake-operator-service"
	// +optional
	ServiceName string `json:"serviceName,omitempty"`

	// ConfigMapName for the configuration
	// +kubebuilder:default="edgelake-operator-configmap"
	// +optional
	ConfigMapName string `json:"configMapName,omitempty"`

	// NodeSelector for pod scheduling
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// ServiceType specifies the Kubernetes service type
	// +kubebuilder:validation:Enum=ClusterIP;NodePort;LoadBalancer
	// +kubebuilder:default="NodePort"
	// +optional
	ServiceType corev1.ServiceType `json:"serviceType,omitempty"`
}

// ImageSpec defines container image configuration
type ImageSpec struct {
	// SecretName for image pull secrets
	// +optional
	SecretName string `json:"secretName,omitempty"`

	// Repository is the Docker image repository
	// +kubebuilder:default="anylogco/edgelake-network"
	// +optional
	Repository string `json:"repository,omitempty"`

	// Tag is the image tag
	// +kubebuilder:default="1.3.2500"
	// +optional
	Tag string `json:"tag,omitempty"`

	// PullPolicy for the image
	// +kubebuilder:validation:Enum=Always;IfNotPresent;Never
	// +kubebuilder:default="IfNotPresent"
	// +optional
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`
}

// PersistenceSpec defines persistent volume configuration
type PersistenceSpec struct {
	// Enabled enables persistent volumes
	// +kubebuilder:default=true
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// StorageClassName for the PVCs
	// +optional
	StorageClassName string `json:"storageClassName,omitempty"`

	// AccessMode for the PVCs
	// +kubebuilder:default="ReadWriteOnce"
	// +optional
	AccessMode corev1.PersistentVolumeAccessMode `json:"accessMode,omitempty"`

	// AnyLog volume configuration
	// +optional
	AnyLog *VolumeSpec `json:"anylog,omitempty"`

	// Blockchain volume configuration
	// +optional
	Blockchain *VolumeSpec `json:"blockchain,omitempty"`

	// Data volume configuration
	// +optional
	Data *VolumeSpec `json:"data,omitempty"`

	// Scripts volume configuration
	// +optional
	Scripts *VolumeSpec `json:"scripts,omitempty"`
}

// VolumeSpec defines a volume size
type VolumeSpec struct {
	// Size of the volume
	// +kubebuilder:default="5Gi"
	Size string `json:"size,omitempty"`
}

// NodeConfigsSpec contains all EdgeLake node configuration
type NodeConfigsSpec struct {
	// Directories configuration
	// +optional
	Directories *DirectoriesConfig `json:"directories,omitempty"`

	// General node configuration
	// +optional
	General *GeneralConfig `json:"general,omitempty"`

	// Geolocation configuration
	// +optional
	Geolocation *GeolocationConfig `json:"geolocation,omitempty"`

	// Networking configuration
	// +optional
	Networking *NetworkingConfig `json:"networking,omitempty"`

	// Database configuration
	// +optional
	Database *DatabaseConfig `json:"database,omitempty"`

	// Blockchain configuration
	// +optional
	Blockchain *BlockchainConfig `json:"blockchain,omitempty"`

	// Operator configuration
	// +optional
	Operator *OperatorConfig `json:"operator,omitempty"`

	// MQTT configuration
	// +optional
	MQTT *MQTTConfig `json:"mqtt,omitempty"`

	// OPCUA configuration
	// +optional
	OPCUA *OPCUAConfig `json:"opcua,omitempty"`

	// EtherIP configuration
	// +optional
	EtherIP *EtherIPConfig `json:"etherip,omitempty"`

	// Aggregations configuration
	// +optional
	Aggregations *AggregationsConfig `json:"aggregations,omitempty"`

	// Monitoring configuration
	// +optional
	Monitoring *MonitoringConfig `json:"monitoring,omitempty"`

	// MCP configuration
	// +optional
	MCP *MCPConfig `json:"mcp,omitempty"`

	// Advanced configuration
	// +optional
	Advanced *AdvancedConfig `json:"advanced,omitempty"`

	// Nebula VPN configuration
	// +optional
	Nebula *NebulaConfig `json:"nebula,omitempty"`
}

// DirectoriesConfig defines path configuration
type DirectoriesConfig struct {
	// AnyLogPath is the EdgeLake root path
	// +kubebuilder:default="/app"
	// +optional
	AnyLogPath string `json:"anylogPath,omitempty"`

	// LocalScripts directory
	// +kubebuilder:default="/app/deployment-scripts/node-deployment"
	// +optional
	LocalScripts string `json:"localScripts,omitempty"`

	// TestDir directory
	// +kubebuilder:default="/app/deployment-scripts/tests"
	// +optional
	TestDir string `json:"testDir,omitempty"`
}

// GeneralConfig defines general node settings
type GeneralConfig struct {
	// LicenseKey for EdgeLake
	// +optional
	LicenseKey string `json:"licenseKey,omitempty"`

	// NodeType specifies the node type
	// +kubebuilder:validation:Enum=master;operator;query
	// +kubebuilder:default="operator"
	// +optional
	NodeType string `json:"nodeType,omitempty"`

	// NodeName is the name of the EdgeLake instance
	// +optional
	NodeName string `json:"nodeName,omitempty"`

	// CompanyName is the organization name
	// +kubebuilder:default="New Company"
	// +optional
	CompanyName string `json:"companyName,omitempty"`

	// DisableCLI disables the CLI interface
	// +kubebuilder:default=false
	// +optional
	DisableCLI bool `json:"disableCLI,omitempty"`

	// RemoteCLI enables remote CLI access
	// +kubebuilder:default=false
	// +optional
	RemoteCLI bool `json:"remoteCLI,omitempty"`
}

// GeolocationConfig defines location settings
type GeolocationConfig struct {
	// Location GPS coordinates
	// +optional
	Location string `json:"location,omitempty"`

	// Country name
	// +optional
	Country string `json:"country,omitempty"`

	// State or province
	// +optional
	State string `json:"state,omitempty"`

	// City name
	// +optional
	City string `json:"city,omitempty"`
}

// NetworkingConfig defines network settings
type NetworkingConfig struct {
	// OverlayIP for VPN/Tailscale
	// +optional
	OverlayIP string `json:"overlayIP,omitempty"`

	// AnyLogServerPort is the TCP server port
	// +kubebuilder:default=32148
	// +optional
	AnyLogServerPort int32 `json:"anylogServerPort,omitempty"`

	// AnyLogRESTPort is the REST API port
	// +kubebuilder:default=32149
	// +optional
	AnyLogRESTPort int32 `json:"anylogRESTPort,omitempty"`

	// AnyLogBrokerPort is the MQTT broker port
	// +optional
	AnyLogBrokerPort int32 `json:"anylogBrokerPort,omitempty"`

	// TCPBind binds to specific IP
	// +kubebuilder:default=false
	// +optional
	TCPBind bool `json:"tcpBind,omitempty"`

	// RESTBind binds to specific IP
	// +kubebuilder:default=false
	// +optional
	RESTBind bool `json:"restBind,omitempty"`

	// BrokerBind binds to specific IP
	// +kubebuilder:default=false
	// +optional
	BrokerBind bool `json:"brokerBind,omitempty"`

	// ConfigName is the network configuration policy name
	// +optional
	ConfigName string `json:"configName,omitempty"`

	// NICType is the NIC type
	// +optional
	NICType string `json:"nicType,omitempty"`

	// TCPThreads is the number of TCP threads
	// +kubebuilder:default=6
	// +optional
	TCPThreads int32 `json:"tcpThreads,omitempty"`

	// RESTTimeout in seconds
	// +kubebuilder:default=30
	// +optional
	RESTTimeout int32 `json:"restTimeout,omitempty"`

	// RESTThreads is the number of REST threads
	// +kubebuilder:default=6
	// +optional
	RESTThreads int32 `json:"restThreads,omitempty"`

	// BrokerThreads is the number of broker threads
	// +kubebuilder:default=6
	// +optional
	BrokerThreads int32 `json:"brokerThreads,omitempty"`
}

// DatabaseConfig defines database settings
type DatabaseConfig struct {
	// DBType is sqlite or psql
	// +kubebuilder:validation:Enum=sqlite;psql
	// +kubebuilder:default="sqlite"
	// +optional
	DBType string `json:"dbType,omitempty"`

	// DBUser for PostgreSQL
	// +optional
	DBUser string `json:"dbUser,omitempty"`

	// DBPasswd for PostgreSQL
	// +optional
	DBPasswd string `json:"dbPasswd,omitempty"`

	// DBIP is the database host
	// +kubebuilder:default="127.0.0.1"
	// +optional
	DBIP string `json:"dbIP,omitempty"`

	// DBPort is the database port
	// +kubebuilder:default=5432
	// +optional
	DBPort int32 `json:"dbPort,omitempty"`

	// Autocommit enables autocommit
	// +kubebuilder:default=false
	// +optional
	Autocommit bool `json:"autocommit,omitempty"`

	// EnableNoSQL enables NoSQL database
	// +kubebuilder:default=false
	// +optional
	EnableNoSQL bool `json:"enableNoSQL,omitempty"`

	// SystemQuery enables system_query database
	// +kubebuilder:default=false
	// +optional
	SystemQuery bool `json:"systemQuery,omitempty"`

	// Memory uses in-memory SQLite
	// +kubebuilder:default=false
	// +optional
	Memory bool `json:"memory,omitempty"`

	// NoSQLType is the NoSQL database type
	// +kubebuilder:default="mongo"
	// +optional
	NoSQLType string `json:"nosqlType,omitempty"`

	// NoSQLUser for MongoDB
	// +optional
	NoSQLUser string `json:"nosqlUser,omitempty"`

	// NoSQLPasswd for MongoDB
	// +optional
	NoSQLPasswd string `json:"nosqlPasswd,omitempty"`

	// NoSQLIP is the MongoDB host
	// +kubebuilder:default="127.0.0.1"
	// +optional
	NoSQLIP string `json:"nosqlIP,omitempty"`

	// NoSQLPort is the MongoDB port
	// +kubebuilder:default=27017
	// +optional
	NoSQLPort int32 `json:"nosqlPort,omitempty"`

	// BlobsDBMS stores blobs in database
	// +kubebuilder:default=false
	// +optional
	BlobsDBMS bool `json:"blobsDBMS,omitempty"`

	// BlobsReuse reuses existing blobs
	// +kubebuilder:default=true
	// +optional
	BlobsReuse bool `json:"blobsReuse,omitempty"`
}

// BlockchainConfig defines blockchain settings
type BlockchainConfig struct {
	// LedgerConn is the master node connection
	// +kubebuilder:default="127.0.0.1:32048"
	// +optional
	LedgerConn string `json:"ledgerConn,omitempty"`

	// SyncTime is the blockchain sync frequency
	// +kubebuilder:default="30 second"
	// +optional
	SyncTime string `json:"syncTime,omitempty"`

	// BlockchainSync is the sync interval
	// +kubebuilder:default="30 second"
	// +optional
	BlockchainSync string `json:"blockchainSync,omitempty"`

	// BlockchainSource is master, ethereum, or hyperledger
	// +kubebuilder:validation:Enum=master;ethereum;hyperledger
	// +kubebuilder:default="master"
	// +optional
	BlockchainSource string `json:"blockchainSource,omitempty"`

	// BlockchainDestination is file or dbms
	// +kubebuilder:validation:Enum=file;dbms
	// +kubebuilder:default="file"
	// +optional
	BlockchainDestination string `json:"blockchainDestination,omitempty"`
}

// OperatorConfig defines operator-specific settings
type OperatorConfig struct {
	// ClusterName is the cluster name
	// +kubebuilder:default="new-company-cluster1"
	// +optional
	ClusterName string `json:"clusterName,omitempty"`

	// DefaultDBMS is the default database name
	// +kubebuilder:default="new_company"
	// +optional
	DefaultDBMS string `json:"defaultDBMS,omitempty"`

	// Member is the operator member ID
	// +optional
	Member string `json:"member,omitempty"`

	// EnableHA enables high availability
	// +kubebuilder:default=false
	// +optional
	EnableHA bool `json:"enableHA,omitempty"`

	// StartDate is days back to sync
	// +kubebuilder:default=30
	// +optional
	StartDate int32 `json:"startDate,omitempty"`

	// OperatorThreads is the number of operator threads
	// +kubebuilder:default=3
	// +optional
	OperatorThreads int32 `json:"operatorThreads,omitempty"`

	// EnablePartitions enables partitioning
	// +kubebuilder:default=true
	// +optional
	EnablePartitions bool `json:"enablePartitions,omitempty"`

	// TableName for partitioning (* for all)
	// +kubebuilder:default="*"
	// +optional
	TableName string `json:"tableName,omitempty"`

	// PartitionColumn is the partition column
	// +kubebuilder:default="insert_timestamp"
	// +optional
	PartitionColumn string `json:"partitionColumn,omitempty"`

	// PartitionInterval is the partition interval
	// +kubebuilder:default="14 days"
	// +optional
	PartitionInterval string `json:"partitionInterval,omitempty"`

	// PartitionKeep is the number of partitions to keep
	// +kubebuilder:default=3
	// +optional
	PartitionKeep int32 `json:"partitionKeep,omitempty"`

	// PartitionSync is the partition sync frequency
	// +kubebuilder:default="1 day"
	// +optional
	PartitionSync string `json:"partitionSync,omitempty"`
}

// MQTTConfig defines MQTT client settings
type MQTTConfig struct {
	// EnableMQTT enables the MQTT client
	// +kubebuilder:default=false
	// +optional
	EnableMQTT bool `json:"enableMQTT,omitempty"`

	// MQTTBroker address
	// +optional
	MQTTBroker string `json:"mqttBroker,omitempty"`

	// MQTTPort is the broker port
	// +kubebuilder:default=1883
	// +optional
	MQTTPort int32 `json:"mqttPort,omitempty"`

	// MQTTUser for authentication
	// +optional
	MQTTUser string `json:"mqttUser,omitempty"`

	// MQTTPasswd for authentication
	// +optional
	MQTTPasswd string `json:"mqttPasswd,omitempty"`

	// MQTTLog enables MQTT logging
	// +kubebuilder:default=false
	// +optional
	MQTTLog bool `json:"mqttLog,omitempty"`

	// MsgTopic is the MQTT topic to subscribe
	// +optional
	MsgTopic string `json:"msgTopic,omitempty"`

	// MsgDBMS is the target database
	// +kubebuilder:default="new_company"
	// +optional
	MsgDBMS string `json:"msgDBMS,omitempty"`

	// MsgTable is the target table
	// +kubebuilder:default="bring [table]"
	// +optional
	MsgTable string `json:"msgTable,omitempty"`

	// MsgTimestampColumn is the timestamp column
	// +kubebuilder:default="bring [timestamp]"
	// +optional
	MsgTimestampColumn string `json:"msgTimestampColumn,omitempty"`

	// MsgValueColumn is the value column
	// +kubebuilder:default="bring [value]"
	// +optional
	MsgValueColumn string `json:"msgValueColumn,omitempty"`

	// MsgValueColumnType is the value column type
	// +kubebuilder:default="float"
	// +optional
	MsgValueColumnType string `json:"msgValueColumnType,omitempty"`
}

// OPCUAConfig defines OPC-UA client settings
type OPCUAConfig struct {
	// EnableOPCUA enables the OPC-UA client
	// +kubebuilder:default=false
	// +optional
	EnableOPCUA bool `json:"enableOPCUA,omitempty"`

	// OPCUAURL is the server URL
	// +optional
	OPCUAURL string `json:"opcuaURL,omitempty"`

	// OPCUANode is the node path
	// +optional
	OPCUANode string `json:"opcuaNode,omitempty"`

	// OPCUAFrequency is the polling frequency
	// +optional
	OPCUAFrequency string `json:"opcuaFrequency,omitempty"`
}

// EtherIPConfig defines EtherNet/IP settings
type EtherIPConfig struct {
	// EnableEtherIP enables EtherNet/IP
	// +kubebuilder:default=false
	// +optional
	EnableEtherIP bool `json:"enableEtherIP,omitempty"`

	// SimulatorMode enables simulator
	// +kubebuilder:default=false
	// +optional
	SimulatorMode bool `json:"simulatorMode,omitempty"`

	// EtherIPURL is the PLC IP address
	// +optional
	EtherIPURL string `json:"etheripURL,omitempty"`

	// EtherIPFrequency is the polling frequency
	// +optional
	EtherIPFrequency string `json:"etheripFrequency,omitempty"`
}

// AggregationsConfig defines aggregation settings
type AggregationsConfig struct {
	// EnableAggregations enables data aggregations
	// +kubebuilder:default=false
	// +optional
	EnableAggregations bool `json:"enableAggregations,omitempty"`

	// AggregationTimeColumn is the timestamp column
	// +kubebuilder:default="insert_timestamp"
	// +optional
	AggregationTimeColumn string `json:"aggregationTimeColumn,omitempty"`

	// AggregationValueColumn is the value column
	// +kubebuilder:default="value"
	// +optional
	AggregationValueColumn string `json:"aggregationValueColumn,omitempty"`
}

// MonitoringConfig defines monitoring settings
type MonitoringConfig struct {
	// MonitorNodes enables node monitoring
	// +kubebuilder:default=false
	// +optional
	MonitorNodes bool `json:"monitorNodes,omitempty"`

	// StoreMonitoring stores monitoring data
	// +kubebuilder:default=false
	// +optional
	StoreMonitoring bool `json:"storeMonitoring,omitempty"`

	// SyslogMonitoring accepts syslog data
	// +kubebuilder:default=false
	// +optional
	SyslogMonitoring bool `json:"syslogMonitoring,omitempty"`
}

// MCPConfig defines MCP settings
type MCPConfig struct {
	// MCPAutostart auto-starts MCP server
	// +kubebuilder:default=false
	// +optional
	MCPAutostart bool `json:"mcpAutostart,omitempty"`
}

// AdvancedConfig defines advanced settings
type AdvancedConfig struct {
	// DeployLocalScript deploys custom script
	// +kubebuilder:default=false
	// +optional
	DeployLocalScript bool `json:"deployLocalScript,omitempty"`

	// DebugMode enables debug mode
	// +kubebuilder:default=false
	// +optional
	DebugMode bool `json:"debugMode,omitempty"`

	// CompressFile compresses backup files
	// +kubebuilder:default=true
	// +optional
	CompressFile bool `json:"compressFile,omitempty"`

	// QueryPool is the number of parallel queries
	// +kubebuilder:default=6
	// +optional
	QueryPool int32 `json:"queryPool,omitempty"`

	// WriteImmediate writes data immediately
	// +kubebuilder:default=false
	// +optional
	WriteImmediate bool `json:"writeImmediate,omitempty"`

	// ThresholdTime is the buffer threshold time
	// +kubebuilder:default="60 seconds"
	// +optional
	ThresholdTime string `json:"thresholdTime,omitempty"`

	// ThresholdVolume is the buffer threshold volume
	// +kubebuilder:default="100KB"
	// +optional
	ThresholdVolume string `json:"thresholdVolume,omitempty"`
}

// NebulaConfig defines Nebula VPN settings
type NebulaConfig struct {
	// EnableNebula enables Nebula VPN
	// +kubebuilder:default=false
	// +optional
	EnableNebula bool `json:"enableNebula,omitempty"`

	// NebulaNewKeys generates new keys
	// +kubebuilder:default=false
	// +optional
	NebulaNewKeys bool `json:"nebulaNewKeys,omitempty"`

	// IsLighthouse marks as lighthouse
	// +kubebuilder:default=false
	// +optional
	IsLighthouse bool `json:"isLighthouse,omitempty"`

	// CIDROverlayAddress is the overlay address
	// +optional
	CIDROverlayAddress string `json:"cidrOverlayAddress,omitempty"`

	// LighthouseIP is the lighthouse IP
	// +optional
	LighthouseIP string `json:"lighthouseIP,omitempty"`

	// LighthouseNodeIP is the lighthouse node physical IP
	// +optional
	LighthouseNodeIP string `json:"lighthouseNodeIP,omitempty"`
}

// EdgeLakeOperatorStatus defines the observed state of EdgeLakeOperator
type EdgeLakeOperatorStatus struct {
	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Phase represents the current phase of the operator
	// +optional
	Phase string `json:"phase,omitempty"`

	// ObservedGeneration is the most recent generation observed
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Ready indicates if the EdgeLake node is ready
	// +optional
	Ready bool `json:"ready,omitempty"`

	// DeploymentName is the name of the created deployment
	// +optional
	DeploymentName string `json:"deploymentName,omitempty"`

	// ServiceName is the name of the created service
	// +optional
	ServiceName string `json:"serviceName,omitempty"`

	// ConfigMapName is the name of the created configmap
	// +optional
	ConfigMapName string `json:"configMapName,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// EdgeLakeOperator is the Schema for the edgelakeoperators API
type EdgeLakeOperator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EdgeLakeOperatorSpec   `json:"spec,omitempty"`
	Status EdgeLakeOperatorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EdgeLakeOperatorList contains a list of EdgeLakeOperator
type EdgeLakeOperatorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EdgeLakeOperator `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EdgeLakeOperator{}, &EdgeLakeOperatorList{})
}
