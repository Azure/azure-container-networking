// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	localtls "github.com/Azure/azure-container-networking/server/tls"

	"github.com/Azure/azure-container-networking/cns/ipampoolmonitor"
	"github.com/Azure/azure-container-networking/cns/nmagentclient"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/cnm/ipam"
	"github.com/Azure/azure-container-networking/cnm/network"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/cnsclient"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/dncclient"
	"github.com/Azure/azure-container-networking/cns/hnsclient"
	"github.com/Azure/azure-container-networking/cns/imdsclient"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/requestcontroller"
	"github.com/Azure/azure-container-networking/cns/requestcontroller/kubecontroller"
	"github.com/Azure/azure-container-networking/cns/restserver"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Azure/azure-container-networking/store"
)

const (
	// Service name.
	name                              = "azure-cns"
	pluginName                        = "azure-vnet"
	defaultCNINetworkConfigFileName   = "10-azure.conflist"
	configFileName                    = "config.json"
	poolIPAMRefreshRateInMilliseconds = 1000

	// 720 * acn.FiveSeconds sec sleeps = 1Hr
	maxRetryNodeRegister = 720
)

// Version is populated by make during build.
var version string

// Reports channel
var reports = make(chan interface{})
var telemetryStopProcessing = make(chan bool)
var stopheartbeat = make(chan bool)
var stopSnapshots = make(chan bool)

// Command line arguments for CNS.
var args = acn.ArgumentList{
	{
		Name:         acn.OptEnvironment,
		Shorthand:    acn.OptEnvironmentAlias,
		Description:  "Set the operating environment",
		Type:         "string",
		DefaultValue: acn.OptEnvironmentAzure,
		ValueMap: map[string]interface{}{
			acn.OptEnvironmentAzure:    0,
			acn.OptEnvironmentMAS:      0,
			acn.OptEnvironmentFileIpam: 0,
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
		Description:  "Set to false to disable telemetry. This is deprecated in favor of cns_config.json",
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
	{
		Name:         acn.OptDebugCmd,
		Shorthand:    acn.OptDebugCmdAlias,
		Description:  "Debug flag to retrieve IPconfigs, available values: allocated, available, all",
		Type:         "string",
		DefaultValue: "",
	},
	{
		Name:         acn.OptDebugArg,
		Shorthand:    acn.OptDebugArgAlias,
		Description:  "Argument flag to be paired with the 'debugcmd' flag.",
		Type:         "string",
		DefaultValue: "",
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
	ipamQueryUrl := acn.GetArg(acn.OptIpamQueryUrl).(string)
	ipamQueryInterval := acn.GetArg(acn.OptIpamQueryInterval).(int)
	startCNM := acn.GetArg(acn.OptStartAzureCNM).(bool)
	vers := acn.GetArg(acn.OptVersion).(bool)
	createDefaultExtNetworkType := acn.GetArg(acn.OptCreateDefaultExtNetworkType).(string)
	telemetryEnabled := acn.GetArg(acn.OptTelemetry).(bool)
	httpConnectionTimeout := acn.GetArg(acn.OptHttpConnectionTimeout).(int)
	httpResponseHeaderTimeout := acn.GetArg(acn.OptHttpResponseHeaderTimeout).(int)
	storeFileLocation := acn.GetArg(acn.OptStoreFileLocation).(string)
	clientDebugCmd := acn.GetArg(acn.OptDebugCmd).(string)
	clientDebugArg := acn.GetArg(acn.OptDebugArg).(string)

	if vers {
		printVersion()
		os.Exit(0)
	}

	// Initialize CNS.
	var (
		err       error
		config    common.ServiceConfig
		dncClient *dncclient.DNCClient
	)

	config.Version = version
	config.Name = name
	// Create a channel to receive unhandled errors from CNS.
	config.ErrChan = make(chan error, 1)

	// Create logging provider.
	logger.InitLogger(name, logLevel, logTarget, logDirectory)

	if clientDebugCmd != "" {
		err := cnsclient.HandleCNSClientCommands(clientDebugCmd, clientDebugArg)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if !telemetryEnabled {
		logger.Errorf("[Azure CNS] Cannot disable telemetry via cmdline. Update cns_config.json to disable telemetry.")
	}

	cnsconfig, err := configuration.ReadConfig()
	if err != nil {
		logger.Errorf("[Azure CNS] Error reading cns config: %v", err)
	}

	configuration.SetCNSConfigDefaults(&cnsconfig)
	logger.Printf("[Azure CNS] Read config :%+v", cnsconfig)

	if cnsconfig.WireserverIP != "" {
		nmagentclient.WireserverIP = cnsconfig.WireserverIP
	}

	config.ChannelMode = cnsconfig.ChannelMode
	disableTelemetry := cnsconfig.TelemetrySettings.DisableAll
	if !disableTelemetry {
		ts := cnsconfig.TelemetrySettings
		aiConfig := aitelemetry.AIConfig{
			AppName:                      name,
			AppVersion:                   version,
			BatchSize:                    ts.TelemetryBatchSizeBytes,
			BatchInterval:                ts.TelemetryBatchIntervalInSecs,
			RefreshTimeout:               ts.RefreshIntervalInSecs,
			DisableMetadataRefreshThread: ts.DisableMetadataRefreshThread,
			DebugMode:                    ts.DebugMode,
		}

		logger.InitAI(aiConfig, ts.DisableTrace, ts.DisableMetric, ts.DisableEvent)
		logger.InitReportChannel(reports)
	}

	// Log platform information.
	logger.Printf("Running on %v", platform.GetOSInfo())

	err = acn.CreateDirectory(storeFileLocation)
	if err != nil {
		logger.Errorf("Failed to create File Store directory %s, due to Error:%v", storeFileLocation, err.Error())
		return
	}

	// Create the key value store.
	storeFileName := storeFileLocation + name + ".json"
	config.Store, err = store.NewJsonFileStore(storeFileName)
	if err != nil {
		logger.Errorf("Failed to create store file: %s, due to error %v\n", storeFileName, err)
		return
	}

	// Create DNCClient if CNS is running with managed DNC mode
	if config.ChannelMode == cns.Managed {
		// Return if the managed settings are invalid
		if !configuration.ValidateManagedSettings(&cnsconfig) {
			logger.Errorf("[Azure CNS] Missing ManagedSettings: [%+v] with ChannelMode set to managed", cnsconfig.ManagedSettings)
			return
		}

		if dncClient, err = dncclient.NewDNCClient(&cnsconfig.ManagedSettings, &cnsconfig.HttpClientSettings); err != nil {
			logger.Errorf(err.Error())
			return
		}
	}

	// Create CNS object.
	httpRestService, err := restserver.NewHTTPRestService(&config, new(imdsclient.ImdsClient), dncClient)
	if err != nil {
		logger.Errorf("Failed to create CNS object, err:%v.\n", err)
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
			logger.Printf("[Azure CNS] Successfully created default ext network")
		} else {
			logger.Printf("[Azure CNS] Failed to create default ext network due to error: %v", err)
			return
		}
	}

	// Start CNS.
	if httpRestService != nil {
		if cnsconfig.UseHTTPS {
			config.TlsSettings = localtls.TlsSettings{
				TLSSubjectName:     cnsconfig.TLSSubjectName,
				TLSCertificatePath: cnsconfig.TLSCertificatePath,
				TLSEndpoint:        cnsconfig.TLSEndpoint,
			}
		}

		err = httpRestService.Start(&config)
		if err != nil {
			logger.Errorf("Failed to start CNS, err:%v.\n", err)
			return
		}
	}

	if !disableTelemetry {
		go logger.SendToTelemetryService(reports, telemetryStopProcessing)
		go logger.SendHeartBeat(cnsconfig.TelemetrySettings.HeartBeatIntervalInMins, stopheartbeat)
		go httpRestService.SendNCSnapShotPeriodically(cnsconfig.TelemetrySettings.SnapshotIntervalInMins, stopSnapshots)
	}

	// If CNS is running with managed DNC mode
	if config.ChannelMode == cns.Managed {
		orchestratorDetails := dncClient.RegisterNode()
		httpRestService.SetNodeOrchestrator(orchestratorDetails)

		go func() {
			// Periodically poll DNC for node updates for network containers
			for {
				<-time.NewTicker(time.Duration(cnsconfig.ManagedSettings.NodeSyncIntervalInSeconds) * time.Second).C
				httpRestService.SyncNodeNcStatus(json.RawMessage{})
			}
		}()
	} else if config.ChannelMode == cns.CRD {
		var requestController requestcontroller.RequestController

		logger.Printf("[Azure CNS] Starting request controller")

		kubeConfig, err := kubecontroller.GetKubeConfig()
		if err != nil {
			logger.Errorf("[Azure CNS] Failed to get kubeconfig for request controller: %v", err)
			return
		}

		//convert interface type to implementation type
		httpRestServiceImplementation, ok := httpRestService.(*restserver.HTTPRestService)
		if !ok {
			logger.Errorf("[Azure CNS] Failed to convert interface httpRestService to implementation: %v", httpRestService)
			return
		}

		// Set orchestrator type
		orchestrator := cns.SetOrchestratorTypeRequest{
			OrchestratorType: cns.KubernetesCRD,
		}
		httpRestServiceImplementation.SetNodeOrchestrator(&orchestrator)

		// Get crd implementation of request controller
		requestController, err = kubecontroller.NewCrdRequestController(httpRestServiceImplementation, kubeConfig)
		if err != nil {
			logger.Errorf("[Azure CNS] Failed to make crd request controller :%v", err)
			return
		}

		// initialize the ipam pool monitor
		httpRestServiceImplementation.IPAMPoolMonitor = ipampoolmonitor.NewCNSIPAMPoolMonitor(httpRestServiceImplementation, requestController)

		//Start the RequestController which starts the reconcile loop
		requestControllerStopChannel := make(chan struct{})
		defer close(requestControllerStopChannel)
		go func() {
			if err := requestController.StartRequestController(requestControllerStopChannel); err != nil {
				logger.Errorf("[Azure CNS] Failed to start request controller: %v", err)
				return
			}
		}()

		ctx := context.Background()
		go func() {
			if err := httpRestServiceImplementation.IPAMPoolMonitor.Start(ctx, poolIPAMRefreshRateInMilliseconds); err != nil {
				logger.Errorf("[Azure CNS] Failed to start pool monitor with err: %v", err)
			}
		}()
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
			logger.Errorf("Failed to create network plugin, err:%v.\n", err)
			return
		}

		// Create IPAM plugin.
		ipamPlugin, err = ipam.NewPlugin(&pluginConfig)
		if err != nil {
			logger.Errorf("Failed to create IPAM plugin, err:%v.\n", err)
			return
		}

		// Create the key value store.
		pluginStoreFile := storeFileLocation + pluginName + ".json"
		pluginConfig.Store, err = store.NewJsonFileStore(pluginStoreFile)
		if err != nil {
			logger.Errorf("Failed to create plugin store file %s, due to error : %v\n", pluginStoreFile, err)
			return
		}

		// Set plugin options.
		netPlugin.SetOption(acn.OptAPIServerURL, url)
		logger.Printf("Start netplugin\n")
		if err := netPlugin.Start(&pluginConfig); err != nil {
			logger.Errorf("Failed to create network plugin, err:%v.\n", err)
			return
		}

		ipamPlugin.SetOption(acn.OptEnvironment, environment)
		ipamPlugin.SetOption(acn.OptAPIServerURL, url)
		ipamPlugin.SetOption(acn.OptIpamQueryUrl, ipamQueryUrl)
		ipamPlugin.SetOption(acn.OptIpamQueryInterval, ipamQueryInterval)
		if err := ipamPlugin.Start(&pluginConfig); err != nil {
			logger.Errorf("Failed to create IPAM plugin, err:%v.\n", err)
			return
		}
	}

	// Relay these incoming signals to OS signal channel.
	osSignalChannel := make(chan os.Signal, 1)
	signal.Notify(osSignalChannel, os.Interrupt, os.Kill, syscall.SIGTERM)

	// Wait until receiving a signal.
	select {
	case sig := <-osSignalChannel:
		logger.Printf("CNS Received OS signal <" + sig.String() + ">, shutting down.")
	case err := <-config.ErrChan:
		logger.Printf("CNS Received unhandled error %v, shutting down.", err)
	}

	if len(strings.TrimSpace(createDefaultExtNetworkType)) > 0 {
		if err := hnsclient.DeleteDefaultExtNetwork(); err == nil {
			logger.Printf("[Azure CNS] Successfully deleted default ext network")
		} else {
			logger.Printf("[Azure CNS] Failed to delete default ext network due to error: %v", err)
		}
	}

	if !disableTelemetry {
		telemetryStopProcessing <- true
		stopheartbeat <- true
		stopSnapshots <- true
	}

	// Cleanup.
	if httpRestService != nil {
		httpRestService.Stop()
	}

	if startCNM {
		if netPlugin != nil {
			netPlugin.Stop()
		}

		if ipamPlugin != nil {
			ipamPlugin.Stop()
		}
	}

	logger.Close()
}
