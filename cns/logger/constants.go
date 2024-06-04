// Copyright Microsoft. All rights reserved.
package logger

const (
	// Metrics
	HeartBeatMetricStr = "HeartBeat"

	// Dimensions
	OrchestratorTypeStr = "OrchestratorType"
	NodeIDStr           = "NodeID"
	HomeAZStr           = "HomeAZ"
	IsAZRSupportedStr   = "IsAZRSupported"
	HomeAZErrorCodeStr  = "HomeAZErrorCode"
	HomeAZErrorMsgStr   = "HomeAZErrorMsg"

	// CNS Snspshot properties
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
