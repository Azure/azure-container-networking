// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package main

import (
	"fmt"
	"os"
	"time"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cni/api"
	zaplog "github.com/Azure/azure-container-networking/cni/log"
	"github.com/Azure/azure-container-networking/cni/network"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/nns"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Azure/azure-container-networking/store"
	"github.com/Azure/azure-container-networking/telemetry"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	ipamQueryURL                    = "http://168.63.129.16/machine/plugins?comp=nmagent&type=getinterfaceinfov1"
	pluginName                      = "CNI"
	telemetryNumRetries             = 5
	telemetryWaitTimeInMilliseconds = 200
	name                            = "azure-vnet"
)

// Version is populated by make during build.
var version string

var logger = zaplog.CNILogger.With(zap.String("component", "cni-main"))

// Command line arguments for CNI plugin.
var args = common.ArgumentList{
	{
		Name:         common.OptVersion,
		Shorthand:    common.OptVersionAlias,
		Description:  "Print version information",
		Type:         "bool",
		DefaultValue: false,
	},
}

// Prints version information.
func printVersion() {
	fmt.Printf("Azure CNI Version %v\n", version)
}

func rootExecute() error {
	var config common.PluginConfig

	config.Version = version

	reportManager := &telemetry.ReportManager{
		Report: &telemetry.CNIReport{
			Context:       "AzureCNI",
			SystemDetails: telemetry.SystemInfo{},
			Version:       version,
			Logger:        logger,
		},
	}

	cniReport := reportManager.Report.(*telemetry.CNIReport)

	netPlugin, err := network.NewPlugin(
		name,
		&config,
		&nns.GrpcClient{},
		&network.Multitenancy{},
	)
	if err != nil {
		network.PrintCNIError(fmt.Sprintf("Failed to create network plugin, err:%v.\n", err))
		return errors.Wrap(err, "Create plugin error")
	}

	// Check CNI_COMMAND value
	cniCmd := os.Getenv(cni.Cmd)

	if cniCmd != cni.CmdVersion {
		logger.Info("Environment variable set", zap.String("CNI_COMMAND", cniCmd))

		cniReport.GetReport(pluginName, version, ipamQueryURL)

		var upTime time.Time
		p := platform.NewExecClient(logger)
		upTime, err = p.GetLastRebootTime()
		if err == nil {
			cniReport.VMUptime = upTime.Format("2006-01-02 15:04:05")
		}

		// Start telemetry process if not already started. This should be done inside lock, otherwise multiple process
		// end up creating/killing telemetry process results in undesired state.
		telemetry.Client.StartAndConnectTelemetry(logger)
		defer telemetry.Client.DisconnectTelemetry()
		telemetry.Client.SetSettings(cniReport)

		// CNI Acquires lock
		if err = netPlugin.Plugin.InitializeKeyValueStore(&config); err != nil {
			network.PrintCNIError(fmt.Sprintf("Failed to initialize key-value store of network plugin: %v", err))

			if telemetry.Client.IsConnected() {
				logger.Error("Not connected to telemetry service")
				return errors.Wrap(err, "lock acquire error")
			}
			telemetry.Client.SendError(err)

			if errors.Is(err, store.ErrTimeoutLockingStore) {
				var cniMetric telemetry.AIMetric
				cniMetric.Metric = aitelemetry.Metric{
					Name:             telemetry.CNILockTimeoutStr,
					Value:            1.0,
					CustomDimensions: make(map[string]string),
				}
				telemetry.Client.SendMetric(&cniMetric)
			}
			return errors.Wrap(err, "lock acquire error")
		}

		defer func() {
			if errUninit := netPlugin.Plugin.UninitializeKeyValueStore(); errUninit != nil {
				logger.Error("Failed to uninitialize key-value store of network plugin", zap.Error(errUninit))
			}

			if recover() != nil {
				os.Exit(1)
			}
		}()

		t := time.Now()
		cniReport.Timestamp = t.Format("2006-01-02 15:04:05")

		if err = netPlugin.Start(&config); err != nil {
			network.PrintCNIError(fmt.Sprintf("Failed to start network plugin, err:%v.\n", err))
			telemetry.Client.SendError(err)
			panic("network plugin start fatal error")
		}

		// used to dump state
		if cniCmd == cni.CmdGetEndpointsState {
			logger.Debug("Retrieving state")
			var simpleState *api.AzureCNIState
			simpleState, err = netPlugin.GetAllEndpointState("azure")
			if err != nil {
				logger.Error("Failed to get Azure CNI state", zap.Error(err))
				return errors.Wrap(err, "Get all endpoints error")
			}

			err = simpleState.PrintResult()
			if err != nil {
				logger.Error("Failed to print state result to stdout", zap.Error(err))
			}

			return errors.Wrap(err, "Get cni state printresult error")
		}
	}

	handled, _ := network.HandleIfCniUpdate(netPlugin.Update)
	if handled {
		logger.Info("CNI UPDATE finished.")
	} else if err = netPlugin.Execute(cni.PluginApi(netPlugin)); err != nil {
		logger.Error("Failed to execute network plugin", zap.Error(err))
	}

	if cniCmd == cni.CmdVersion {
		return errors.Wrap(err, "Execute netplugin failure")
	}
	netPlugin.Stop()

	return errors.Wrap(err, "Execute netplugin failure")
}

// Main is the entry point for CNI network plugin.
func main() {
	// Initialize and parse command line arguments.
	common.ParseArgs(&args, printVersion)
	vers := common.GetArg(common.OptVersion).(bool)

	if vers {
		printVersion()
		os.Exit(0)
	}

	if rootExecute() != nil {
		os.Exit(1)
	}
}
