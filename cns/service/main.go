// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Azure/azure-container-networking/cnm/ipam"
	"github.com/Azure/azure-container-networking/cnm/network"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/hnsclient"
	"github.com/Azure/azure-container-networking/cns/restserver"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Azure/azure-container-networking/store"
	"github.com/Azure/azure-container-networking/telemetry"
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
		Name:         acn.OptStartAzureCNM,
		Shorthand:    acn.OptStartAzureCNMAlias,
		Description:  "Start Azure-CNM if flag is set",
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
		Name:         acn.OptNetPluginPath,
		Shorthand:    acn.OptNetPluginPathAlias,
		Description:  "Set network plugin binary absolute path to parent (of azure-vnet and azure-vnet-ipam)",
		Type:         "string",
		DefaultValue: platform.K8SCNIRuntimePath,
	},
	{
		Name:         acn.OptNetPluginConfigFile,
		Shorthand:    acn.OptNetPluginConfigFileAlias,
		Description:  "Set network plugin configuration file absolute path",
		Type:         "string",
		DefaultValue: platform.K8SNetConfigPath + string(os.PathSeparator) + defaultCNINetworkConfigFileName,
	},
	{
		Name:         acn.OptCreateDefaultExtNetworkType,
		Shorthand:    acn.OptCreateDefaultExtNetworkTypeAlias,
		Description:  "Create default external network for windows platform with the specified type (l2bridge or l2tunnel)",
		Type:         "string",
		DefaultValue: "",
	},
	{
		Name:         acn.OptTelemetry,
		Shorthand:    acn.OptTelemetryAlias,
		Description:  "Set to false to disable telemetry",
		Type:         "bool",
		DefaultValue: true,
	},
	{
		Name:         acn.OptHttpConnectionTimeout,
		Shorthand:    acn.OptHttpConnectionTimeoutAlias,
		Description:  "Set HTTP connection timeout in seconds to be used by http client in CNS",
		Type:         "int",
		DefaultValue: "5",
	},
	{
		Name:         acn.OptHttpResponseHeaderTimeout,
		Shorthand:    acn.OptHttpResponseHeaderTimeoutAlias,
		Description:  "Set HTTP response header timeout in seconds to be used by http client in CNS",
		Type:         "int",
		DefaultValue: "120",
	},
	{
		Name:         acn.OptStoreFileLocation,
		Shorthand:    acn.OptStoreFileLocationAlias,
		Description:  "Set store file absolute path",
		Type:         "string",
		DefaultValue: platform.CNMRuntimePath,
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

	environment := acn.GetArg(acn.OptEnvironment).(string)
	url := acn.GetArg(acn.OptAPIServerURL).(string)
	cniPath := acn.GetArg(acn.OptNetPluginPath).(string)
	cniConfigFile := acn.GetArg(acn.OptNetPluginConfigFile).(string)
	cnsURL := acn.GetArg(acn.OptCnsURL).(string)
	logLevel := acn.GetArg(acn.OptLogLevel).(int)
	logTarget := acn.GetArg(acn.OptLogTarget).(int)
	logDirectory := acn.GetArg(acn.OptLogLocation).(string)
	ipamQueryUrl, _ := acn.GetArg(acn.OptIpamQueryUrl).(string)
	ipamQueryInterval, _ := acn.GetArg(acn.OptIpamQueryInterval).(int)
	startCNM := acn.GetArg(acn.OptStartAzureCNM).(bool)
	vers := acn.GetArg(acn.OptVersion).(bool)
	createDefaultExtNetworkType := acn.GetArg(acn.OptCreateDefaultExtNetworkType).(string)
	telemetryEnabled := acn.GetArg(acn.OptTelemetry).(bool)
	httpConnectionTimeout := acn.GetArg(acn.OptHttpConnectionTimeout).(int)
	httpResponseHeaderTimeout := acn.GetArg(acn.OptHttpResponseHeaderTimeout).(int)
	storeFileLocation := acn.GetArg(acn.OptStoreFileLocation).(string)

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

	// Set-up channel for CNS telemetry if it's enabled (enabled by default)
	if logger := log.GetStd(); logger != nil && telemetryEnabled {
		logger.SetChannel(reports)
	}

	// Log platform information.
	log.Printf("Running on %v", platform.GetOSInfo())

	err = acn.CreateDirectory(storeFileLocation)
	if err != nil {
		log.Errorf("Failed to create File Store directory %s, due to Error:%v", storeFileLocation, err.Error())
		return
	}

	// Create the key value store.
	storeFileName := storeFileLocation + name + ".json"
	config.Store, err = store.NewJsonFileStore(storeFileName)
	if err != nil {
		log.Errorf("Failed to create store file: %s, due to error %v\n", storeFileName, err)
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
	httpRestService.SetOption(acn.OptNetPluginPath, cniPath)
	httpRestService.SetOption(acn.OptNetPluginConfigFile, cniConfigFile)
	httpRestService.SetOption(acn.OptCreateDefaultExtNetworkType, createDefaultExtNetworkType)
	httpRestService.SetOption(acn.OptHttpConnectionTimeout, httpConnectionTimeout)
	httpRestService.SetOption(acn.OptHttpResponseHeaderTimeout, httpResponseHeaderTimeout)

	// Create default ext network if commandline option is set
	if len(strings.TrimSpace(createDefaultExtNetworkType)) > 0 {
		if err := hnsclient.CreateDefaultExtNetwork(createDefaultExtNetworkType); err == nil {
			log.Printf("[Azure CNS] Successfully created default ext network")
		} else {
			log.Printf("[Azure CNS] Failed to create default ext network due to error: %v", err)
			return
		}
	}

	// Start CNS.
	if httpRestService != nil {
		if telemetryEnabled {
			go telemetry.SendCnsTelemetry(
				reports,
				httpRestService.(*restserver.HTTPRestService),
				telemetryStopProcessing)
		}

		err = httpRestService.Start(&config)
		if err != nil {
			log.Errorf("Failed to start CNS, err:%v.\n", err)
			return
		}
	}

	var netPlugin network.NetPlugin
	var ipamPlugin ipam.IpamPlugin

	if startCNM {
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
		pluginStoreFile := storeFileLocation + pluginName + ".json"
		pluginConfig.Store, err = store.NewJsonFileStore(pluginStoreFile)
		if err != nil {
			log.Errorf("Failed to create plugin store file %s, due to error : %v\n", pluginStoreFile, err)
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

	if len(strings.TrimSpace(createDefaultExtNetworkType)) > 0 {
		if err := hnsclient.DeleteDefaultExtNetwork(); err == nil {
			log.Printf("[Azure CNS] Successfully deleted default ext network")
		} else {
			log.Printf("[Azure CNS] Failed to delete default ext network due to error: %v", err)
		}
	}

	// Cleanup.
	if httpRestService != nil {
		httpRestService.Stop()
	}

	telemetryStopProcessing <- true

	if startCNM {
		if netPlugin != nil {
			netPlugin.Stop()
		}

		if ipamPlugin != nil {
			ipamPlugin.Stop()
		}
	}

	log.Close()
}
