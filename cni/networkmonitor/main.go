// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package main

import (
	"fmt"
	"os"
	"time"

	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Azure/azure-container-networking/store"
)

const (
	// Service name.
	name                    = "azure-cnimonitor"
	pluginName              = "azure-vnet"
	DEFAULT_TIMEOUT_IN_SECS = "60"
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
	fmt.Printf("Azure Container Network Service\n")
	fmt.Printf("Version %v\n", version)
}

// Main is the entry point for CNS.
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

	// Initialize CNS.
	var config acn.PluginConfig
	config.Version = version

	// Create a channel to receive unhandled errors from CNS.
	config.ErrChan = make(chan error, 1)

	var err error
	// Create logging provider.
	log.SetName(name)
	log.SetLevel(logLevel)
	if logDirectory != "" {
		log.SetLogDirectory(logDirectory)
	}

	err = log.SetTarget(logTarget)
	if err != nil {
		fmt.Printf("Failed to configure logging: %v\n", err)
		return
	}

	// Log platform information.
	log.Printf("Running on %v", platform.GetOSInfo())

	config.Store, err = store.NewJsonFileStore(platform.CNIRuntimePath + pluginName + ".json")
	if err != nil {
		fmt.Printf("Failed to create store: %v\n", err)
		return
	}

	for true {
		nm, err := network.NewNetworkManager()
		if err != nil {
			log.Printf("Failed while creating network manager")
			return
		}

		if err := nm.Initialize(&config); err != nil {
			log.Printf("Failed while initializing network manager %+v", err)
		}

		log.Printf("network manager:%+v", nm)

		if err := nm.SetupNetworkUsingState(); err != nil {
			log.Printf("Failed while SetupNetworkUsingState")
			return
		}

		log.Printf("Going to sleep for %v seconds", timeout)
		time.Sleep(time.Duration(timeout) * time.Second)
	}

	log.Close()
}
