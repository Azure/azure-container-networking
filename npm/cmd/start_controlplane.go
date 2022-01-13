package main

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm"
	npmconfig "github.com/Azure/azure-container-networking/npm/config"
	restserver "github.com/Azure/azure-container-networking/npm/http/server"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane"
	"github.com/Azure/azure-container-networking/npm/pkg/transport"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
)

func newStartNPMControlplaneCmd() *cobra.Command {
	startNPMControlplaneCmd := &cobra.Command{
		Use:   "controlplane",
		Short: "Starts the Azure NPM controlplane process",
		RunE: func(cmd *cobra.Command, args []string) error {
			config := &npmconfig.Config{}
			err := viper.Unmarshal(config)
			if err != nil {
				return fmt.Errorf("failed to load config with error: %w", err)
			}

			flags := npmconfig.Flags{
				KubeConfigPath: viper.GetString(flagKubeConfigPath),
			}

			return startControlplane(*config, flags)
		},
	}

	startNPMControlplaneCmd.Flags().String(flagKubeConfigPath, flagDefaults[flagKubeConfigPath], "path to kubeconfig")

	return startNPMControlplaneCmd
}

func startControlplane(config npmconfig.Config, flags npmconfig.Flags) error {
	klog.Infof("loaded config: %+v", config)
	klog.Infof("Start NPM version: %s", version)

	var err error

	err = initLogging()
	if err != nil {
		return err
	}

	// Create the kubernetes client
	var k8sConfig *rest.Config
	if flags.KubeConfigPath == "" {
		klog.Infof("loading in cluster kubeconfig")
		k8sConfig, err = rest.InClusterConfig()
		if err != nil {
			return fmt.Errorf("failed to load in cluster config: %w", err)
		}
	} else {
		klog.Infof("loading kubeconfig from flag: %s", flags.KubeConfigPath)
		k8sConfig, err = clientcmd.BuildConfigFromFlags("", flags.KubeConfigPath)
		if err != nil {
			return fmt.Errorf("failed to load kubeconfig [%s] with err config: %w", flags.KubeConfigPath, err)
		}
	}

	// Creates the clientset
	clientset, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		klog.Infof("clientset creation failed with error %v.", err)
		return fmt.Errorf("failed to generate clientset with cluster config: %w", err)
	}

	// Setting reSyncPeriod
	minResyncPeriod := time.Duration(config.ResyncPeriodInMinutes) * time.Minute

	// Adding some randomness so all NPM pods will not request for info at once.
	factor := rand.Float64() + 1 //nolint
	resyncPeriod := time.Duration(float64(minResyncPeriod.Nanoseconds()) * factor)
	klog.Infof("Resync period for NPM pod is set to %d.", int(resyncPeriod/time.Minute))
	factory := informers.NewSharedInformerFactory(clientset, resyncPeriod)

	k8sServerVersion := k8sServerVersion(clientset)

	var dp dataplane.GenericDataplane

	mgr := transport.NewManager(context.Background(), config.Transport.Port)

	// TODO (vakalapa): This needs to be passed to the Dataplane instance
	_ = mgr.InputChannel()

	// TODO (vakalapa): Replace this with the DP shim implementation of the GenericDataplane interface
	dp, err = dataplane.NewDataPlane(npm.GetNodeName(), common.NewIOShim(), npmV2DataplaneCfg)
	if err != nil {
		return fmt.Errorf("failed to create dataplane with error %w", err)
	}

	npMgr := npm.NewNetworkPolicyControlplane(config, factory, mgr, dp, version, k8sServerVersion)
	err = metrics.CreateTelemetryHandle(version, npm.GetAIMetadata())
	if err != nil {
		klog.Infof("CreateTelemetryHandle failed with error %v.", err)
		return fmt.Errorf("CreateTelemetryHandle failed with error %w", err)
	}

	go restserver.NPMRestServerListenAndServe(config, npMgr)

	return npMgr.Start(config, wait.NeverStop)
}
