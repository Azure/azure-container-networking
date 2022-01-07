// Copyright Microsoft. All rights reserved.

package telemetry

const (

	// Metric Names
	CNIAddTimeMetricStr    = "CNIAddTimeMs"
	CNIDelTimeMetricStr    = "CNIDelTimeMs"
	CNIUpdateTimeMetricStr = "CNIUpdateTimeMs"
	CNILockTimeoutStr      = "CNILockTimeoutError"

	// Dimension Names
	ContextStr        = "Context"
	SubContextStr     = "SubContext"
	VMUptimeStr       = "VMUptime"
	OperationTypeStr  = "OperationType"
	VersionStr        = "Version"
	StatusStr         = "Status"
	CNIModeStr        = "CNIMode"
	CNINetworkModeStr = "CNINetworkMode"
	OSTypeStr         = "OSType"

	// Values
	SucceededStr     = "Succeeded"
	FailedStr        = "Failed"
	SingleTenancyStr = "SingleTenancy"
	MultiTenancyStr  = "MultiTenancy"
)
