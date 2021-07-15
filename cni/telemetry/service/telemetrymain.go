package main

// Entry point of the telemetry service if started by CNI

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/Azure/azure-container-networking/aitelemetry"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/telemetry"
)

const (
	defaultReportToHostIntervalInSecs = 30
	defaultRefreshTimeoutInSecs       = 15
	defaultBatchSizeInBytes           = 16384
	defaultBatchIntervalInSecs        = 15
	defaultGetEnvRetryCount           = 2
	defaultGetEnvRetryWaitTimeInSecs  = 3
	pluginName                        = "AzureCNI"
	azureVnetTelemetry                = "azure-vnet-telemetry"
	configExtension                   = ".config"
)

var version string

var args = acn.ArgumentList{
	{
		Name:         acn.OptLogLevel,
		Shorthand:    acn.OptLogLevelAlias,
		Description:  "Set the logging level",
		Type:         "int",
		DefaultValue: acn.OptLogLevelInfo,
		ValueMap: map[string]interface{}{
			acn.OptLogLevelInfo:  log.LevelInfo,
			acn.OptLogLevelDebug: log.LevelDebug,
		},
	},
	{
		Name:         acn.OptLogTarget,
		Shorthand:    acn.OptLogTargetAlias,
		Description:  "Set the logging target",
		Type:         "int",
		DefaultValue: acn.OptLogTargetFile,
		ValueMap: map[string]interface{}{
			acn.OptLogTargetSyslog: log.TargetSyslog,
			acn.OptLogTargetStderr: log.TargetStderr,
			acn.OptLogTargetFile:   log.TargetLogfile,
			acn.OptLogStdout:       log.TargetStdout,
			acn.OptLogMultiWrite:   log.TargetStdOutAndLogFile,
		},
	},
	{
		Name:         acn.OptLogLocation,
		Shorthand:    acn.OptLogLocationAlias,
		Description:  "Set the directory location where logs will be saved",
		Type:         "string",
		DefaultValue: "",
	},
	{
		Name:         acn.OptVersion,
		Shorthand:    acn.OptVersionAlias,
		Description:  "Print version information",
		Type:         "bool",
		DefaultValue: false,
	},
	{
		Name:         acn.OptTelemetryConfigDir,
		Shorthand:    acn.OptTelemetryConfigDirAlias,
		Description:  "Set the telmetry config directory",
		Type:         "string",
		DefaultValue: telemetry.CniInstallDir,
	},
}

// Prints description and version information.
func printVersion() {
	fmt.Printf("Azure Container Telemetry Service\n")
	fmt.Printf("Version %v\n", version)
}

func setTelemetryDefaults(config *telemetry.TelemetryConfig) {
	if config.ReportToHostIntervalInSeconds == 0 {
		config.ReportToHostIntervalInSeconds = defaultReportToHostIntervalInSecs
	}

	if config.RefreshTimeoutInSecs == 0 {
		config.RefreshTimeoutInSecs = defaultRefreshTimeoutInSecs
	}

	if config.BatchIntervalInSecs == 0 {
		config.BatchIntervalInSecs = defaultBatchIntervalInSecs
	}

	if config.BatchSizeInBytes == 0 {
		config.BatchSizeInBytes = defaultBatchSizeInBytes
	}

	if config.GetEnvRetryCount == 0 {
		config.GetEnvRetryCount = defaultGetEnvRetryCount
	}

	if config.GetEnvRetryWaitTimeInSecs == 0 {
		config.GetEnvRetryWaitTimeInSecs = defaultGetEnvRetryWaitTimeInSecs
	}
}

func main() {
	var tb *telemetry.TelemetryBuffer
	var config telemetry.TelemetryConfig
	var configPath string
	var err error

	acn.ParseArgs(&args, printVersion)
	logTarget := acn.GetArg(acn.OptLogTarget).(int)
	logDirectory := acn.GetArg(acn.OptLogLocation).(string)
	logLevel := acn.GetArg(acn.OptLogLevel).(int)
	configDirectory := acn.GetArg(acn.OptTelemetryConfigDir).(string)
	vers := acn.GetArg(acn.OptVersion).(bool)

	if vers {
		printVersion()
		os.Exit(0)
	}

	log.SetName(azureVnetTelemetry)
	log.SetLevel(logLevel)
	err = log.SetTargetLogDirectory(logTarget, logDirectory)
	if err != nil {
		fmt.Printf("Failed to configure logging: %v\n", err)
		return
	}

	log.Logf("args %+v", os.Args)

	if runtime.GOOS == "linux" {
		configPath = fmt.Sprintf("%s/%s%s", configDirectory, azureVnetTelemetry, configExtension)
	} else {
		configPath = fmt.Sprintf("%s\\%s%s", configDirectory, azureVnetTelemetry, configExtension)
	}

	log.Logf("[Telemetry] Config path: %s", configPath)

	config, err = telemetry.ReadConfigFile(configPath)
	if err != nil {
		log.Logf("[Telemetry] Error reading telemetry config: %v", err)
	}

	log.Logf("read config returned %+v", config)

	setTelemetryDefaults(&config)

	log.Logf("Config after setting defaults %+v", config)

	// Cleaning up orphan socket if present
	tbtemp := telemetry.NewTelemetryBuffer()
	tbtemp.Cleanup(telemetry.FdName)

	for {
		tb = telemetry.NewTelemetryBuffer()

		log.Logf("[Telemetry] Starting telemetry server")
		err = tb.StartServer()
		if err == nil || tb.FdExists {
			break
		}

		log.Logf("[Telemetry] Telemetry service starting failed: %v", err)
		tb.Cleanup(telemetry.FdName)
		time.Sleep(time.Millisecond * 200)
	}

	aiConfig := aitelemetry.AIConfig{
		AppName:                      pluginName,
		AppVersion:                   version,
		BatchSize:                    config.BatchSizeInBytes,
		BatchInterval:                config.BatchIntervalInSecs,
		RefreshTimeout:               config.RefreshTimeoutInSecs,
		DisableMetadataRefreshThread: config.DisableMetadataThread,
		DebugMode:                    config.DebugMode,
		GetEnvRetryCount:             config.GetEnvRetryCount,
		GetEnvRetryWaitTimeInSecs:    config.GetEnvRetryWaitTimeInSecs,
	}

	err = telemetry.CreateAITelemetryHandle(aiConfig, config.DisableAll, config.DisableTrace, config.DisableMetric)
	log.Printf("[Telemetry] AI Handle creation status:%v", err)
	log.Logf("[Telemetry] Report to host for an interval of %d seconds", config.ReportToHostIntervalInSeconds)
	tb.PushData()
	telemetry.CloseAITelemetryHandle()

	log.Close()
}
