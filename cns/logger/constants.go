// Copyright Microsoft. All rights reserved.
package logger

const (
	// Metrics
	HeartBeatMetricStr       = "HeartBeat"
	ConfigSnapshotMetricsStr = "ConfigSnapshot"

	// Dimensions
	orchestratorTypeKey             = "OrchestratorType"
	nodeIDKey                       = "NodeID"
	HomeAZStr                       = "HomeAZ"
	IsAZRSupportedStr               = "IsAZRSupported"
	IsAZRDualStackFixPresentStr     = "IsAZRDualStackFixPresent"
	HomeAZErrorCodeStr              = "HomeAZErrorCode"
	HomeAZErrorMsgStr               = "HomeAZErrorMsg"
	CNSConfigPropertyStr            = "CNSConfiguration"
	CNSConfigMD5CheckSumPropertyStr = "CNSConfigurationMD5Checksum"
	apiServerKey                    = "APIServer"

	// CNS NC Snspshot properties
	CnsNCSnapshotEventStr         = "CNSNCSnapshot"
	IpConfigurationStr            = "IPConfiguration"
	LocalIPConfigurationStr       = "LocalIPConfiguration"
	PrimaryInterfaceIdentifierStr = "PrimaryInterfaceIdentifier"
	MultiTenancyInfoStr           = "MultiTenancyInfo"
	CnetAddressSpaceStr           = "CnetAddressSpace"
	AllowNCToHostCommunicationStr = "AllowNCToHostCommunication"
	AllowHostToNCCommunicationStr = "AllowHostToNCCommunication"
	NetworkContainerTypeStr       = "NetworkContainerType"
	OrchestratorContextStr        = "OrchestratorContext"
)
