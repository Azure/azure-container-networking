package main

// Entry point of the telemetry service if started by CNI

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/cni/log"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/telemetry"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
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
	maxLogFileSizeInMb                = 5
	maxLogFileCount                   = 8
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
			acn.OptLogLevelInfo:  zapcore.InfoLevel,
			acn.OptLogLevelError: zapcore.ErrorLevel,
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
	logLevel := acn.GetArg(acn.OptLogLevel).(zapcore.Level)
	configDirectory := acn.GetArg(acn.OptTelemetryConfigDir).(string)
	vers := acn.GetArg(acn.OptVersion).(bool)

	if vers {
		printVersion()
		os.Exit(0)
	}

	log.LoggerCfg.Level = logLevel
	logger := log.InitZapLogCNI(azureVnetTelemetry, azureVnetTelemetry+".log")

	logger.Info("Telemetry invocation info", zap.Any("arguments", os.Args))

	if runtime.GOOS == "linux" {
		configPath = fmt.Sprintf("%s/%s%s", configDirectory, azureVnetTelemetry, configExtension)
	} else {
		configPath = fmt.Sprintf("%s\\%s%s", configDirectory, azureVnetTelemetry, configExtension)
	}

	logger.Info("Config path", zap.String("path", configPath))

	config, err = telemetry.ReadConfigFile(configPath)
	if err != nil {
		logger.Error("Error reading telemetry config", zap.Error(err))
	}

	logger.Info("read config returned", zap.Any("config", config))

	setTelemetryDefaults(&config)

	logger.Info("Config after setting defaults", zap.Any("config", config))

	// Cleaning up orphan socket if present
	tbtemp := telemetry.NewTelemetryBuffer()
	tbtemp.Cleanup(telemetry.FdName)

	for {
		tb = telemetry.NewTelemetryBuffer()

		logger.Info("Starting telemetry server")
		err = tb.StartServer()
		if err == nil || tb.FdExists {
			break
		}

		logger.Error("Telemetry service starting failed", zap.Error(err))
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

	if telemetry.CreateAITelemetryHandle(aiConfig, config.DisableAll, config.DisableTrace, config.DisableMetric) != nil {
		logger.Error("[Telemetry] AI Handle creation error", zap.Error(err))
	}
	logger.Info("[Telemetry] Report to host interval", zap.Duration("seconds", config.ReportToHostIntervalInSeconds))
	tb.PushData(context.Background())
	telemetry.CloseAITelemetryHandle()

}
