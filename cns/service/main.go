// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Azure/azure-container-networking/cnm/ipam"
	"github.com/Azure/azure-container-networking/cnm/network"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/restserver"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Azure/azure-container-networking/store"
)

const (
	// Service name.
	name                            = "azure-cns"
	pluginName                      = "azure-vnet"
	defaultCNINetworkConfigFileName = "10-azure.conflist"
)

// Version is populated by make during build.
var version string

// Reports channel
var reports = make(chan interface{})
var telemetryStopProcessing = make(chan bool)

// Command line arguments for CNS.
var args = acn.ArgumentList{
	{
		Name:         acn.OptEnvironment,
		Shorthand:    acn.OptEnvironmentAlias,
		Description:  "Set the operating environment",
		Type:         "string",
		DefaultValue: acn.OptEnvironmentAzure,
		ValueMap: map[string]interface{}{
			acn.OptEnvironmentAzure: 0,
			acn.OptEnvironmentMAS:   0,
		},
	},

	{
		Name:         acn.OptAPIServerURL,
		Shorthand:    acn.OptAPIServerURLAlias,
		Description:  "Set the API server URL",
		Type:         "string",
		DefaultValue: "",
	},
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
		Name:         acn.OptIpamQueryUrl,
		Shorthand:    acn.OptIpamQueryUrlAlias,
		Description:  "Set the IPAM query URL",
		Type:         "string",
		DefaultValue: "",
	},
	{
		Name:         acn.OptIpamQueryInterval,
		Shorthand:    acn.OptIpamQueryIntervalAlias,
		Description:  "Set the IPAM plugin query interval",
		Type:         "int",
		DefaultValue: "",
	},
	{
		Name:         acn.OptCnsURL,
		Shorthand:    acn.OptCnsURLAlias,
		Description:  "Set the URL for CNS to listen on",
		Type:         "string",
		DefaultValue: "",
	},
	{
		Name:         acn.OptStopAzureVnet,
		Shorthand:    acn.OptStopAzureVnetAlias,
		Description:  "Stop Azure-CNM if flag is true",
		Type:         "bool",
		DefaultValue: false,
	},
	{
		Name:         acn.OptVersion,
		Shorthand:    acn.OptVersionAlias,
		Description:  "Print version information",
		Type:         "bool",
		DefaultValue: false,
	},
	{
		Name:         acn.OptReportToHostInterval,
		Shorthand:    acn.OptReportToHostIntervalAlias,
		Description:  "Set interval in ms to report to host",
		Type:         "int",
		DefaultValue: "60000",
	},
	{
		Name:         acn.OptCNIPath,
		Shorthand:    acn.OptCNIPathAlias,
		Description:  "Set CNI binary absolute path to parent (of azure-vnet and azure-vnet-ipam)",
		Type:         "string",
		DefaultValue: platform.K8SCNIRuntimePath,
	},
	{
		Name:         acn.OptCNIConfigFile,
		Shorthand:    acn.OptCNIConfigFileAlias,
		Description:  "Set CNI configuration file absolute path",
		Type:         "string",
		DefaultValue: platform.K8SNetConfigPath + string(os.PathSeparator) + defaultCNINetworkConfigFileName,
	},
}

// Prints description and version information.
func printVersion() {
	fmt.Printf("Azure Container Network Service\n")
	fmt.Printf("Version %v\n", version)
}

// Main is the entry point for CNS.
func main() {
	var stopcnm = false
	// Initialize and parse command line arguments.
	acn.ParseArgs(&args, printVersion)

	environment := acn.GetArg(acn.OptEnvironment).(string)
	url := acn.GetArg(acn.OptAPIServerURL).(string)
	cniPath := acn.GetArg(acn.OptCNIPath).(string)
	cniConfigFile := acn.GetArg(acn.OptCNIConfigFile).(string)
	cnsURL := acn.GetArg(acn.OptCnsURL).(string)
	logLevel := acn.GetArg(acn.OptLogLevel).(int)
	logTarget := acn.GetArg(acn.OptLogTarget).(int)
	logDirectory := acn.GetArg(acn.OptLogLocation).(string)
	ipamQueryUrl, _ := acn.GetArg(acn.OptIpamQueryUrl).(string)
	ipamQueryInterval, _ := acn.GetArg(acn.OptIpamQueryInterval).(int)
	stopcnm = acn.GetArg(acn.OptStopAzureVnet).(bool)
	vers := acn.GetArg(acn.OptVersion).(bool)
	// reportToHostInterval := acn.GetArg(acn.OptReportToHostInterval).(int)

	if vers {
		printVersion()
		os.Exit(0)
	}

	// Initialize CNS.
	var config common.ServiceConfig
	config.Version = version
	config.Name = name

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

	if logger := log.GetStd(); logger != nil {
		logger.SetChannel(reports)
	}

	// Log platform information.
	log.Printf("Running on %v", platform.GetOSInfo())

	err = acn.CreateDirectory(platform.CNMRuntimePath)
	if err != nil {
		log.Errorf("Failed to create File Store directory Error:%v", err.Error())
		return
	}

	// Create the key value store.
	config.Store, err = store.NewJsonFileStore(platform.CNMRuntimePath + name + ".json")
	if err != nil {
		log.Errorf("Failed to create store: %v\n", err)
		return
	}

	// Create CNS object.
	httpRestService, err := restserver.NewHTTPRestService(&config)
	if err != nil {
		log.Errorf("Failed to create CNS object, err:%v.\n", err)
		return
	}

	// Set CNS options.
	httpRestService.SetOption(acn.OptCnsURL, cnsURL)
	httpRestService.SetOption(acn.OptCNIPath, cniPath)
	httpRestService.SetOption(acn.OptCNIConfigFile, cniConfigFile)

	// Start CNS.
	if httpRestService != nil {
		// go telemetry.SendCnsTelemetry(reportToHostInterval,
		// 	reports,
		// 	httpRestService.(*restserver.HTTPRestService),
		// 	telemetryStopProcessing)
		err = httpRestService.Start(&config)
		if err != nil {
			log.Errorf("Failed to start CNS, err:%v.\n", err)
			return
		}
	}

	var netPlugin network.NetPlugin
	var ipamPlugin ipam.IpamPlugin

	if !stopcnm {
		var pluginConfig acn.PluginConfig
		pluginConfig.Version = version

		// Create a channel to receive unhandled errors from the plugins.
		pluginConfig.ErrChan = make(chan error, 1)

		// Create network plugin.
		netPlugin, err = network.NewPlugin(&pluginConfig)
		if err != nil {
			log.Errorf("Failed to create network plugin, err:%v.\n", err)
			return
		}

		// Create IPAM plugin.
		ipamPlugin, err = ipam.NewPlugin(&pluginConfig)
		if err != nil {
			log.Errorf("Failed to create IPAM plugin, err:%v.\n", err)
			return
		}

		// Create the key value store.
		pluginConfig.Store, err = store.NewJsonFileStore(platform.CNMRuntimePath + pluginName + ".json")
		if err != nil {
			log.Errorf("Failed to create store: %v\n", err)
			return
		}

		// Set plugin options.
		netPlugin.SetOption(acn.OptAPIServerURL, url)
		log.Printf("Start netplugin\n")
		if err := netPlugin.Start(&pluginConfig); err != nil {
			log.Errorf("Failed to create network plugin, err:%v.\n", err)
			return
		}

		ipamPlugin.SetOption(acn.OptEnvironment, environment)
		ipamPlugin.SetOption(acn.OptAPIServerURL, url)
		ipamPlugin.SetOption(acn.OptIpamQueryUrl, ipamQueryUrl)
		ipamPlugin.SetOption(acn.OptIpamQueryInterval, ipamQueryInterval)
		if err := ipamPlugin.Start(&pluginConfig); err != nil {
			log.Errorf("Failed to create IPAM plugin, err:%v.\n", err)
			return
		}
	}

	// Relay these incoming signals to OS signal channel.
	osSignalChannel := make(chan os.Signal, 1)
	signal.Notify(osSignalChannel, os.Interrupt, os.Kill, syscall.SIGTERM)

	// Wait until receiving a signal.
	select {
	case sig := <-osSignalChannel:
		log.Printf("CNS Received OS signal <" + sig.String() + ">, shutting down.")
	case err := <-config.ErrChan:
		log.Printf("CNS Received unhandled error %v, shutting down.", err)
	}

	// Cleanup.
	if httpRestService != nil {
		httpRestService.Stop()
	}

	telemetryStopProcessing <- true

	if !stopcnm {
		if netPlugin != nil {
			netPlugin.Stop()
		}

		if ipamPlugin != nil {
			ipamPlugin.Stop()
		}
	}

	log.Close()
}
