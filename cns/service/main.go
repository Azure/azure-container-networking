// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/cnm/ipam"
	"github.com/Azure/azure-container-networking/cnm/network"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/configuration"
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
	name                            = "azure-cns"
	pluginName                      = "azure-vnet"
	defaultCNINetworkConfigFileName = "10-azure.conflist"
	configFileName                  = "config.json"
	dncApiVersion                   = "?api-version=2018-03-01"
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
		Name:         acn.OptPrivateEndpoint,
		Shorthand:    acn.OptPrivateEndpointAlias,
		Description:  "Set private endpoint",
		Type:         "string",
		DefaultValue: "",
	},
	{
		Name:         acn.OptInfrastructureNetworkID,
		Shorthand:    acn.OptInfrastructureNetworkIDAlias,
		Description:  "Set infrastructure network ID",
		Type:         "string",
		DefaultValue: "",
	},
	{
		Name:         acn.OptNodeID,
		Shorthand:    acn.OptNodeIDAlias,
		Description:  "Set node name/ID",
		Type:         "string",
		DefaultValue: "",
	},
	{
		Name:         acn.OptManaged,
		Shorthand:    acn.OptManagedAlias,
		Description:  "Set to true to enable managed mode. This is deprecated in favor of cns_config.json",
		Type:         "bool",
		DefaultValue: false,
	},
}

// Prints description and version information.
func printVersion() {
	fmt.Printf("Azure Container Network Service\n")
	fmt.Printf("Version %v\n", version)
}

// Try to register node with DNC when CNS is started in managed DNC mode
func registerNode(httpRestService cns.HTTPService, dncEP, infraVnet, nodeID string) {
	logger.Printf("[Azure CNS] Registering node %s with Infrastructure Network: %s PrivateEndpoint: %s", nodeID, infraVnet, dncEP)

	var (
		numCPU   = runtime.NumCPU()
		url      = fmt.Sprintf(acn.RegisterNodeURLFmt, dncEP, infraVnet, nodeID, numCPU, dncApiVersion)
		response *http.Response
		err      = fmt.Errorf("")
		body     bytes.Buffer
		httpc    = acn.GetHttpClient()
	)

	for sleep := true; err != nil; sleep = true {
		response, err = httpc.Post(url, "application/json", &body)
		if err == nil {
			if response.StatusCode == http.StatusCreated {
				var req cns.SetOrchestratorTypeRequest
				json.NewDecoder(response.Body).Decode(&req)
				httpRestService.SetNodeOrchestrator(&req)
				sleep = false
			} else {
				err = fmt.Errorf("[Azure CNS] Failed to register node with http status code %s", strconv.Itoa(response.StatusCode))
				logger.Errorf(err.Error())
			}

			response.Body.Close()
		} else {
			logger.Errorf("[Azure CNS] Failed to register node with err: %+v", err)
		}

		if sleep {
			time.Sleep(acn.FiveSeconds)
		}
	}

	logger.Printf("[Azure CNS] Node Registered")
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
	privateEndpoint := acn.GetArg(acn.OptPrivateEndpoint).(string)
	infravnet := acn.GetArg(acn.OptInfrastructureNetworkID).(string)
	nodeID := acn.GetArg(acn.OptNodeID).(string)

	if vers {
		printVersion()
		os.Exit(0)
	}

	// Initialize CNS.
	var (
		err    error
		config common.ServiceConfig
	)

	config.Version = version
	config.Name = name
	// Create a channel to receive unhandled errors from CNS.
	config.ErrChan = make(chan error, 1)

	// Create logging provider.
	logger.InitLogger(name, logLevel, logTarget, logDirectory)

	if !telemetryEnabled {
		logger.Errorf("[Azure CNS] Cannot disable telemetry via cmdline. Update cns_config.json to disable telemetry.")
	}

	cnsconfig, err := configuration.ReadConfig()
	if err != nil {
		logger.Errorf("[Azure CNS] Error reading cns config: %v", err)
	}

	configuration.SetCNSConfigDefaults(&cnsconfig)
	logger.Printf("[Azure CNS] Read config :%+v", cnsconfig)

	if cnsconfig.ChannelMode == cns.Managed {
		config.ChannelMode = cns.Managed
		privateEndpoint = cnsconfig.ManagedSettings.PrivateEndpoint
		infravnet = cnsconfig.ManagedSettings.InfrastructureNetworkID
		nodeID = cnsconfig.ManagedSettings.NodeID
	} else if cnsconfig.ChannelMode == cns.CRD {
		config.ChannelMode = cns.CRD
	} else if acn.GetArg(acn.OptManaged).(bool) {
		config.ChannelMode = cns.Managed
	}

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

	// Create CNS object.
	httpRestService, err := restserver.NewHTTPRestService(&config, new(imdsclient.ImdsClient))
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

	// If CNS is running on managed DNC mode
	if config.ChannelMode == cns.Managed {
		if privateEndpoint == "" || infravnet == "" || nodeID == "" {
			logger.Errorf("[Azure CNS] Missing required values to run in managed mode: PrivateEndpoint: %s InfrastructureNetworkID: %s NodeID: %s",
				privateEndpoint,
				infravnet,
				nodeID)
			return
		}

		httpRestService.SetOption(acn.OptPrivateEndpoint, privateEndpoint)
		httpRestService.SetOption(acn.OptInfrastructureNetworkID, infravnet)
		httpRestService.SetOption(acn.OptNodeID, nodeID)

		registerNode(httpRestService, privateEndpoint, infravnet, nodeID)
		go func(ep, vnet, node string) {
			// Periodically poll DNC for node updates
			for {
				<-time.NewTicker(time.Duration(cnsconfig.ManagedSettings.NodeSyncIntervalInSeconds) * time.Second).C
				httpRestService.SyncNodeStatus(ep, vnet, node, json.RawMessage{})
			}
		}(privateEndpoint, infravnet, nodeID)
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

		//Start the RequestController which starts the reconcile loop
		requestControllerStopChannel := make(chan struct{})
		defer close(requestControllerStopChannel)
		go func() {
			if err := requestController.StartRequestController(requestControllerStopChannel); err != nil {
				logger.Errorf("[Azure CNS] Failed to start request controller: %v", err)
				return
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
