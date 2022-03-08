package main

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm"
	npmconfig "github.com/Azure/azure-container-networking/npm/config"
	"github.com/Azure/azure-container-networking/npm/daemon"
	restserver "github.com/Azure/azure-container-networking/npm/http/server"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/pkg/controlplane/goalstateprocessor"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane"
	"github.com/Azure/azure-container-networking/npm/pkg/models"
	"github.com/Azure/azure-container-networking/npm/pkg/transport"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
)

const (
	podNameEnv  = "DAEMON_POD_NAME"
	nodeNameEnv = "DAEMON_NODE_NAME"
)

func newStartNPMDaemonCmd() *cobra.Command {
	startNPMDaemonCmd := &cobra.Command{
		Use:   "daemon",
		Short: "Starts the Azure NPM daemon process",
		RunE: func(cmd *cobra.Command, args []string) error {
			config := &npmconfig.Config{}
			err := viper.Unmarshal(config)
			if err != nil {
				return fmt.Errorf("failed to load config with error: %w", err)
			}

			return startDaemon(*config)
		},
	}

	return startNPMDaemonCmd
}

func startDaemon(config npmconfig.Config) error {
	klog.Infof("loaded config: %+v", config)
	klog.Infof("starting NPM fan-out daemon with image: %s", version)
	// Read these ENV variables from the Pod spec `env` section.
	pod := os.Getenv(podNameEnv)
	node := os.Getenv(nodeNameEnv)

	klog.Infof("initializing metrics")
	metrics.InitializeAll()

	addr := config.Transport.Address + ":" + strconv.Itoa(config.Transport.ServicePort)
	ctx := context.Background()
	err := initLogging()
	if err != nil {
		klog.Errorf("failed to init logging : %v", err)
		return err
	}

	var dp dataplane.GenericDataplane

	dp, err = dataplane.NewDataPlane(models.GetNodeName(), common.NewIOShim(), npmV2DataplaneCfg, wait.NeverStop)
	if err != nil {
		klog.Errorf("failed to create dataplane: %v", err)
		return fmt.Errorf("failed to create dataplane with error %w", err)
	}

	dp.RunPeriodicTasks()
	// TODO Daemon should implement cache encoder
	go restserver.NPMRestServerListenAndServe(config, nil)

	client, err := transport.NewEventsClient(ctx, pod, node, addr)
	if err != nil {
		klog.Errorf("failed to create dataplane events client with error %v", err)
		return fmt.Errorf("failed to create dataplane events client: %w", err)
	}

	gsp, err := goalstateprocessor.NewGoalStateProcessor(ctx, node, pod, client.EventsChannel(), dp)
	if err != nil {
		klog.Errorf("failed to create goalstate processor with error %v", err)
		return fmt.Errorf("failed to create goalstate processor: %w", err)
	}

	n, err := daemon.NewNetworkPolicyDaemon(ctx, config, dp, gsp, client, version)
	if err != nil {
		klog.Errorf("failed to create dataplane : %v", err)
		return fmt.Errorf("failed to create dataplane: %w", err)
	}

	err = metrics.CreateTelemetryHandle(config.NPMVersion(), version, npm.GetAIMetadata())
	if err != nil {
		klog.Infof("CreateTelemetryHandle failed with error %v.", err)
		return fmt.Errorf("CreateTelemetryHandle failed with error %w", err)
	}

	err = n.Start(config, wait.NeverStop)
	if err != nil {
		klog.Errorf("failed to start dataplane : %v", err)
		return fmt.Errorf("failed to start dataplane: %w", err)
	}

	metrics.SendLog(util.FanOutServerID, "started fan-out daemon")

	return nil
}
