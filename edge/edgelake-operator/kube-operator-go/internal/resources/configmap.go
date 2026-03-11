package resources

import (
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	edgelakev1alpha1 "github.com/trv-edgelake-infra/kube-operator-go/api/v1alpha1"
)

// BuildConfigMap creates a ConfigMap for EdgeLake configuration
func BuildConfigMap(cr *edgelakev1alpha1.EdgeLakeOperator) *corev1.ConfigMap {
	labels := Labels(cr)
	data := buildConfigMapData(cr)

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Spec.Metadata.ConfigMapName,
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Data: data,
	}
}

// buildConfigMapData builds the ConfigMap data from the CR spec
func buildConfigMapData(cr *edgelakev1alpha1.EdgeLakeOperator) map[string]string {
	data := make(map[string]string)
	nc := cr.Spec.NodeConfigs

	// Kubernetes indicator
	data["IS_KUBERNETES"] = "true"

	// Proxy IP for service discovery
	data["PROXY_IP"] = fmt.Sprintf("%s.%s.svc.cluster.local", cr.Spec.Metadata.ServiceName, cr.Namespace)

	// Directories
	if nc.Directories != nil {
		data["ANYLOG_PATH"] = nc.Directories.AnyLogPath
		data["LOCAL_SCRIPTS"] = nc.Directories.LocalScripts
		data["TEST_DIR"] = nc.Directories.TestDir
	}

	// General
	if nc.General != nil {
		if nc.General.LicenseKey != "" {
			data["LICENSE_KEY"] = nc.General.LicenseKey
		}
		data["INIT_TYPE"] = "prod"
		data["NODE_TYPE"] = nc.General.NodeType
		nodeName := nc.General.NodeName
		if nodeName == "" {
			nodeName = cr.Spec.Metadata.AppName
		}
		data["NODE_NAME"] = nodeName
		data["COMPANY_NAME"] = nc.General.CompanyName
		data["DISABLE_CLI"] = strconv.FormatBool(nc.General.DisableCLI)
		data["REMOTE_CLI"] = strconv.FormatBool(nc.General.RemoteCLI)
	}

	// Geolocation
	if nc.Geolocation != nil {
		if nc.Geolocation.Location != "" {
			data["LOCATION"] = nc.Geolocation.Location
		}
		if nc.Geolocation.Country != "" {
			data["COUNTRY"] = nc.Geolocation.Country
		}
		if nc.Geolocation.State != "" {
			data["STATE"] = nc.Geolocation.State
		}
		if nc.Geolocation.City != "" {
			data["CITY"] = nc.Geolocation.City
		}
	}

	// Networking
	if nc.Networking != nil {
		if nc.Networking.OverlayIP != "" {
			data["OVERLAY_IP"] = nc.Networking.OverlayIP
		}
		data["ANYLOG_SERVER_PORT"] = strconv.Itoa(int(nc.Networking.AnyLogServerPort))
		data["ANYLOG_REST_PORT"] = strconv.Itoa(int(nc.Networking.AnyLogRESTPort))
		if nc.Networking.AnyLogBrokerPort > 0 {
			data["ANYLOG_BROKER_PORT"] = strconv.Itoa(int(nc.Networking.AnyLogBrokerPort))
		}
		data["TCP_BIND"] = strconv.FormatBool(nc.Networking.TCPBind)
		data["REST_BIND"] = strconv.FormatBool(nc.Networking.RESTBind)
		data["BROKER_BIND"] = strconv.FormatBool(nc.Networking.BrokerBind)

		if nc.Networking.ConfigName != "" {
			data["CONFIG_NAME"] = nc.Networking.ConfigName
		}
		if nc.Networking.NICType != "" {
			data["NIC_TYPE"] = nc.Networking.NICType
		}
		data["TCP_THREADS"] = strconv.Itoa(int(nc.Networking.TCPThreads))
		data["REST_TIMEOUT"] = strconv.Itoa(int(nc.Networking.RESTTimeout))
		data["REST_THREADS"] = strconv.Itoa(int(nc.Networking.RESTThreads))
		data["BROKER_THREADS"] = strconv.Itoa(int(nc.Networking.BrokerThreads))
	}

	// Database
	if nc.Database != nil {
		data["DB_TYPE"] = nc.Database.DBType
		if nc.Database.DBUser != "" {
			data["DB_USER"] = nc.Database.DBUser
		}
		if nc.Database.DBPasswd != "" {
			data["DB_PASSWD"] = nc.Database.DBPasswd
		}
		data["DB_IP"] = nc.Database.DBIP
		data["DB_PORT"] = strconv.Itoa(int(nc.Database.DBPort))
		data["AUTOCOMMIT"] = strconv.FormatBool(nc.Database.Autocommit)
		data["ENABLE_NOSQL"] = strconv.FormatBool(nc.Database.EnableNoSQL)
		data["SYSTEM_QUERY"] = strconv.FormatBool(nc.Database.SystemQuery)
		data["MEMORY"] = strconv.FormatBool(nc.Database.Memory)

		data["NOSQL_TYPE"] = nc.Database.NoSQLType
		if nc.Database.NoSQLUser != "" {
			data["NOSQL_USER"] = nc.Database.NoSQLUser
		}
		if nc.Database.NoSQLPasswd != "" {
			data["NOSQL_PASSWD"] = nc.Database.NoSQLPasswd
		}
		data["NOSQL_IP"] = nc.Database.NoSQLIP
		data["NOSQL_PORT"] = strconv.Itoa(int(nc.Database.NoSQLPort))
		data["BLOBS_DBMS"] = strconv.FormatBool(nc.Database.BlobsDBMS)
		data["BLOBS_REUSE"] = strconv.FormatBool(nc.Database.BlobsReuse)
	}

	// Blockchain
	if nc.Blockchain != nil {
		data["LEDGER_CONN"] = nc.Blockchain.LedgerConn
		data["SYNC_TIME"] = nc.Blockchain.SyncTime
		data["BLOCKCHAIN_SYNC"] = nc.Blockchain.BlockchainSync
		data["BLOCKCHAIN_SOURCE"] = nc.Blockchain.BlockchainSource
		data["BLOCKCHAIN_DESTINATION"] = nc.Blockchain.BlockchainDestination
	}

	// Operator
	if nc.Operator != nil {
		data["CLUSTER_NAME"] = nc.Operator.ClusterName
		data["DEFAULT_DBMS"] = nc.Operator.DefaultDBMS
		if nc.Operator.Member != "" {
			data["MEMBER"] = nc.Operator.Member
		}
		data["ENABLE_HA"] = strconv.FormatBool(nc.Operator.EnableHA)
		data["START_DATE"] = strconv.Itoa(int(nc.Operator.StartDate))
		data["OPERATOR_THREADS"] = strconv.Itoa(int(nc.Operator.OperatorThreads))

		data["ENABLE_PARTITIONS"] = strconv.FormatBool(nc.Operator.EnablePartitions)
		data["TABLE_NAME"] = nc.Operator.TableName
		data["PARTITION_COLUMN"] = nc.Operator.PartitionColumn
		data["PARTITION_INTERVAL"] = nc.Operator.PartitionInterval
		data["PARTITION_KEEP"] = strconv.Itoa(int(nc.Operator.PartitionKeep))
		data["PARTITION_SYNC"] = nc.Operator.PartitionSync
	}

	// MQTT
	if nc.MQTT != nil {
		data["ENABLE_MQTT"] = strconv.FormatBool(nc.MQTT.EnableMQTT)
		if nc.MQTT.MQTTBroker != "" {
			data["MQTT_BROKER"] = nc.MQTT.MQTTBroker
		}
		data["MQTT_PORT"] = strconv.Itoa(int(nc.MQTT.MQTTPort))
		if nc.MQTT.MQTTUser != "" {
			data["MQTT_USER"] = nc.MQTT.MQTTUser
		}
		if nc.MQTT.MQTTPasswd != "" {
			data["MQTT_PASSWD"] = nc.MQTT.MQTTPasswd
		}
		data["MQTT_LOG"] = strconv.FormatBool(nc.MQTT.MQTTLog)

		if nc.MQTT.MsgTopic != "" {
			data["MSG_TOPIC"] = nc.MQTT.MsgTopic
		}
		data["MSG_DBMS"] = nc.MQTT.MsgDBMS
		data["MSG_TABLE"] = nc.MQTT.MsgTable
		data["MSG_TIMESTAMP_COLUMN"] = nc.MQTT.MsgTimestampColumn
		data["MSG_VALUE_COLUMN"] = nc.MQTT.MsgValueColumn
		data["MSG_VALUE_COLUMN_TYPE"] = nc.MQTT.MsgValueColumnType
	}

	// OPC-UA
	if nc.OPCUA != nil {
		data["ENABLE_OPCUA"] = strconv.FormatBool(nc.OPCUA.EnableOPCUA)
		if nc.OPCUA.OPCUAURL != "" {
			data["OPCUA_URL"] = nc.OPCUA.OPCUAURL
		}
		if nc.OPCUA.OPCUANode != "" {
			data["OPCUA_NODE"] = nc.OPCUA.OPCUANode
		}
		if nc.OPCUA.OPCUAFrequency != "" {
			data["OPCUA_FREQUENCY"] = nc.OPCUA.OPCUAFrequency
		}
	}

	// EtherIP
	if nc.EtherIP != nil {
		data["ENABLE_ETHERIP"] = strconv.FormatBool(nc.EtherIP.EnableEtherIP)
		data["SIMULATOR_MODE"] = strconv.FormatBool(nc.EtherIP.SimulatorMode)
		if nc.EtherIP.EtherIPURL != "" {
			data["ETHERIP_URL"] = nc.EtherIP.EtherIPURL
		}
		if nc.EtherIP.EtherIPFrequency != "" {
			data["ETHERIP_FREQUENCY"] = nc.EtherIP.EtherIPFrequency
		}
	}

	// Aggregations
	if nc.Aggregations != nil {
		data["ENABLE_AGGREGATIONS"] = strconv.FormatBool(nc.Aggregations.EnableAggregations)
		data["AGGREGATION_TIME_COLUMN"] = nc.Aggregations.AggregationTimeColumn
		data["AGGREGATION_VALUE_COLUMN"] = nc.Aggregations.AggregationValueColumn
	}

	// Monitoring
	if nc.Monitoring != nil {
		data["MONITOR_NODES"] = strconv.FormatBool(nc.Monitoring.MonitorNodes)
		data["STORE_MONITORING"] = strconv.FormatBool(nc.Monitoring.StoreMonitoring)
		data["SYSLOG_MONITORING"] = strconv.FormatBool(nc.Monitoring.SyslogMonitoring)
	}

	// MCP
	if nc.MCP != nil {
		data["MCP_AUTOSTART"] = strconv.FormatBool(nc.MCP.MCPAutostart)
	}

	// Advanced
	if nc.Advanced != nil {
		data["DEPLOY_LOCAL_SCRIPT"] = strconv.FormatBool(nc.Advanced.DeployLocalScript)
		data["DEBUG_MODE"] = strconv.FormatBool(nc.Advanced.DebugMode)
		data["COMPRESS_FILE"] = strconv.FormatBool(nc.Advanced.CompressFile)
		data["QUERY_POOL"] = strconv.Itoa(int(nc.Advanced.QueryPool))
		data["WRITE_IMMEDIATE"] = strconv.FormatBool(nc.Advanced.WriteImmediate)
		data["THRESHOLD_TIME"] = nc.Advanced.ThresholdTime
		data["THRESHOLD_VOLUME"] = nc.Advanced.ThresholdVolume
	}

	// Nebula
	if nc.Nebula != nil {
		data["ENABLE_NEBULA"] = strconv.FormatBool(nc.Nebula.EnableNebula)
		data["NEBULA_NEW_KEYS"] = strconv.FormatBool(nc.Nebula.NebulaNewKeys)
		data["IS_LIGHTHOUSE"] = strconv.FormatBool(nc.Nebula.IsLighthouse)
		if nc.Nebula.CIDROverlayAddress != "" {
			data["CIDR_OVERLAY_ADDRESS"] = nc.Nebula.CIDROverlayAddress
		}
		if nc.Nebula.LighthouseIP != "" {
			data["LIGHTHOUSE_IP"] = nc.Nebula.LighthouseIP
		}
		if nc.Nebula.LighthouseNodeIP != "" {
			data["LIGHTHOUSE_NODE_IP"] = nc.Nebula.LighthouseNodeIP
		}
	}

	return data
}
