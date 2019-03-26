package main

// Entry point of the telemetry service if started by CNI

import (
	"fmt"
	"os"
	"runtime"
	"time"

	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/telemetry"
)

const (
	reportToHostIntervalInSeconds = 30
	azureVnetTelemetry            = "azure-vnet-telemetry"
	configExtension               = ".config"
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
	if logDirectory != "" {
		log.SetLogDirectory(logDirectory)
	}

	err = log.SetTarget(logTarget)
	if err != nil {
		fmt.Printf("Failed to configure logging: %v\n", err)
		return
	}

	log.Printf("args %+v", os.Args)

	if runtime.GOOS == "linux" {
		configPath = fmt.Sprintf("%s/%s%s", configDirectory, azureVnetTelemetry, configExtension)
	} else {
		configPath = fmt.Sprintf("%s\\%s%s", configDirectory, azureVnetTelemetry, configExtension)
	}

	log.Printf("[Telemetry] Config path: %s", configPath)

	config, err = telemetry.ReadConfigFile(configPath)
	if err != nil {
		log.Printf("[Telemetry] Error reading telemetry config: %v", err)
	}

	log.Printf("read config returned %+v", config)

	for {
		tb = telemetry.NewTelemetryBuffer("")

		log.Printf("[Telemetry] Starting telemetry server")
		err = tb.StartServer()
		if err == nil || tb.FdExists {
			break
		}

		log.Printf("[Telemetry] Telemetry service starting failed: %v", err)
		tb.Cleanup(telemetry.FdName)
		time.Sleep(time.Millisecond * 200)
	}

	if config.ReportToHostIntervalInSeconds == 0 {
		config.ReportToHostIntervalInSeconds = reportToHostIntervalInSeconds
	}

	log.Printf("[Telemetry] Report to host for an interval of %d seconds", config.ReportToHostIntervalInSeconds)
	tb.BufferAndPushData(config.ReportToHostIntervalInSeconds * time.Second)
	log.Close()
}
