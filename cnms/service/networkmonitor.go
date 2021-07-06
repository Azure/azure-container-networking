// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package main

import (
	"fmt"
	"os"
	"time"

	cnms "github.com/Azure/azure-container-networking/cnms/cnmspackage"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Azure/azure-container-networking/store"
	"github.com/Azure/azure-container-networking/telemetry"
)

const (
	// Service name.
	name                            = "azure-cnimonitor"
	pluginName                      = "azure-vnet"
	DEFAULT_TIMEOUT_IN_SECS         = "10"
	telemetryNumRetries             = 5
	telemetryWaitTimeInMilliseconds = 200
)

// Version is populated by make during build.
var version string

// Command line arguments for CNM plugin.
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
		Name:         acn.OptIntervalTime,
		Shorthand:    acn.OptIntervalTimeAlias,
		Description:  "Periodic Interval Time",
		Type:         "int",
		DefaultValue: DEFAULT_TIMEOUT_IN_SECS,
	},
	{
		Name:         acn.OptVersion,
		Shorthand:    acn.OptVersionAlias,
		Description:  "Print version information",
		Type:         "bool",
		DefaultValue: false,
	},
}

// Prints description and version information.
func printVersion() {
	fmt.Printf("Azure Container Network Monitoring Service\n")
	fmt.Printf("Version %v\n", version)
}

// Main is the entry point for CNMS.
func main() {
	// Initialize and parse command line arguments.
	acn.ParseArgs(&args, printVersion)
	logLevel := acn.GetArg(acn.OptLogLevel).(int)
	logTarget := acn.GetArg(acn.OptLogTarget).(int)
	logDirectory := acn.GetArg(acn.OptLogLocation).(string)
	timeout := acn.GetArg(acn.OptIntervalTime).(int)
	vers := acn.GetArg(acn.OptVersion).(bool)
	if vers {
		printVersion()
		os.Exit(0)
	}

	// Initialize CNMS.
	var config acn.PluginConfig
	config.Version = version

	// Create a channel to receive unhandled errors from CNMS.
	config.ErrChan = make(chan error, 1)

	var err error
	// Create logging provider.
	log.SetName(name)
	log.SetLevel(logLevel)
	if err := log.SetTargetLogDirectory(logTarget, logDirectory); err != nil {
		fmt.Printf("[monitor] Failed to configure logging: %v\n", err)
		return
	}

	// Log platform information.
	log.Printf("[monitor] Running on %v", platform.GetOSInfo())

	reportManager := &telemetry.ReportManager{
		ContentType: telemetry.ContentType,
		Report: &telemetry.CNIReport{
			Context:          "AzureCNINetworkMonitor",
			Version:          version,
			SystemDetails:    telemetry.SystemInfo{},
			InterfaceDetails: telemetry.InterfaceInfo{},
			BridgeDetails:    telemetry.BridgeInfo{},
		},
	}

	reportManager.Report.(*telemetry.CNIReport).GetOSDetails()

	netMonitor := &cnms.NetworkMonitor{
		AddRulesToBeValidated:    make(map[string]int),
		DeleteRulesToBeValidated: make(map[string]int),
		CNIReport:                reportManager.Report.(*telemetry.CNIReport),
	}

	tb := telemetry.NewTelemetryBuffer()
	tb.ConnectToTelemetryService(telemetryNumRetries, telemetryWaitTimeInMilliseconds)
	defer tb.Close()

	for {
		config.Store, err = store.NewJsonFileStore(platform.CNIRuntimePath + pluginName + ".json")
		if err != nil {
			fmt.Printf("[monitor] Failed to create store: %v\n", err)
			return
		}

		nm, err := network.NewNetworkManager()
		if err != nil {
			log.Printf("[monitor] Failed while creating network manager")
			return
		}

		if err := nm.Initialize(&config, false); err != nil {
			log.Printf("[monitor] Failed while initializing network manager %+v", err)
		}

		log.Printf("[monitor] network manager:%+v", nm)

		if err := nm.SetupNetworkUsingState(netMonitor); err != nil {
			log.Printf("[monitor] Failed while calling SetupNetworkUsingState with error %v", err)
		}

		if netMonitor.CNIReport.ErrorMessage != "" {
			log.Printf("[monitor] Reporting discrepancy in rules")
			netMonitor.CNIReport.Timestamp = time.Now().Format("2006-01-02 15:04:05")
			if err := reportManager.SendReport(tb); err != nil {
				log.Errorf("[monitor] SendReport failed due to %v", err)
			} else {
				log.Printf("[monitor] Reported successfully")
			}

			netMonitor.CNIReport.ErrorMessage = ""
		}

		log.Printf("[monitor] Going to sleep for %v seconds", timeout)
		time.Sleep(time.Duration(timeout) * time.Second)
		nm = nil
	}
}
