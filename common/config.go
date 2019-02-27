// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package common

// Command line options.
const (
	// Operating environment.
	OptEnvironment      = "environment"
	OptEnvironmentAlias = "e"
	OptEnvironmentAzure = "azure"
	OptEnvironmentMAS   = "mas"

	// API server URL.
	OptAPIServerURL      = "api-url"
	OptAPIServerURLAlias = "u"
	OptCnsURL            = "cns-url"
	OptCnsURLAlias       = "c"

	// Logging level.
	OptLogLevel      = "log-level"
	OptLogLevelAlias = "l"
	OptLogLevelInfo  = "info"
	OptLogLevelDebug = "debug"

	// Logging target.
	OptLogTarget       = "log-target"
	OptLogTargetAlias  = "t"
	OptLogTargetSyslog = "syslog"
	OptLogTargetStderr = "stderr"
	OptLogTargetFile   = "logfile"
	OptLogStdout       = "stdout"
	OptLogMultiWrite   = "stdoutfile"

	// Logging location
	OptLogLocation      = "log-location"
	OptLogLocationAlias = "o"

	// IPAM query URL.
	OptIpamQueryUrl      = "ipam-query-url"
	OptIpamQueryUrlAlias = "q"

	// IPAM query interval.
	OptIpamQueryInterval      = "ipam-query-interval"
	OptIpamQueryIntervalAlias = "i"

	// Don't Start CNM
	OptStopAzureVnet      = "stop-azure-cnm"
	OptStopAzureVnetAlias = "stopcnm"

	// Interval to send reports to host
	OptReportToHostInterval      = "report-interval"
	OptReportToHostIntervalAlias = "hostinterval"

	// Version.
	OptVersion      = "version"
	OptVersionAlias = "v"

	// Help.
	OptHelp      = "help"
	OptHelpAlias = "h"

	// CNI binary location
	OptCNIPath      = "cni-path"
	OptCNIPathAlias = "cni"

	// CNI binary location
	OptCNIConfigFile      = "cni-config-file"
	OptCNIConfigFileAlias = "cniconfig"
)
