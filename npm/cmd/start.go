// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm"
	npmconfig "github.com/Azure/azure-container-networking/npm/config"
	restserver "github.com/Azure/azure-container-networking/npm/http/server"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/Azure/azure-container-networking/npm/pkg/models"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/util/wait"
	k8sversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
	"k8s.io/utils/exec"
)

var npmV2DataplaneCfg = &dataplane.Config{
	IPSetManagerCfg: &ipsets.IPSetManagerCfg{
		// NOTE: NetworkName and IPSetMode must be set later by the npm ConfigMap or default config
	},
	PolicyManagerCfg: &policies.PolicyManagerCfg{
		PolicyMode: policies.IPSetPolicyMode,
		// NOTE: PlaceAzureChainFirst must be set later by the npm ConfigMap or default config
	},
}

func newStartNPMCmd() *cobra.Command {
	// getTuplesCmd represents the getTuples command
	startNPMCmd := &cobra.Command{
		Use:   "start",
		Short: "Starts the Azure NPM process",
		RunE: func(cmd *cobra.Command, args []string) error {
			config := &npmconfig.Config{}
			err := viper.Unmarshal(config)
			if err != nil {
				return fmt.Errorf("failed to load config with error: %w", err)
			}

			flags := npmconfig.Flags{
				KubeConfigPath: viper.GetString(flagKubeConfigPath),
			}

			return start(*config, flags)
		},
	}

	startNPMCmd.Flags().String(flagKubeConfigPath, flagDefaults[flagKubeConfigPath], "path to kubeconfig")

	return startNPMCmd
}

func start(config npmconfig.Config, flags npmconfig.Flags) error {
	klog.Infof("loaded config: %+v", config)
	if util.IsWindowsDP() {
		config.Toggles.EnableV2NPM = true
		klog.Infof("NPM is running on Windows Dataplane. Enabling V2 NPM")
	} else {
		klog.Infof("NPM is running on Linux Dataplane")
	}
	klog.Infof("starting NPM version %d with image %s", config.NPMVersion(), version)

	var err error

	err = initLogging()
	if err != nil {
		return err
	}

	klog.Infof("initializing metrics")
	metrics.InitializeAll()

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
	stopChannel := wait.NeverStop
	if config.Toggles.EnableV2NPM {
		// update the dataplane config
		npmV2DataplaneCfg.MaxBatchedACLsPerPod = config.MaxBatchedACLsPerPod

		npmV2DataplaneCfg.ApplyInBackground = config.Toggles.ApplyInBackground
		if config.ApplyMaxBatches > 0 {
			npmV2DataplaneCfg.ApplyMaxBatches = config.ApplyMaxBatches
		} else {
			npmV2DataplaneCfg.ApplyMaxBatches = npmconfig.DefaultConfig.ApplyMaxBatches
		}
		if config.ApplyIntervalInMilliseconds > 0 {
			npmV2DataplaneCfg.ApplyInterval = time.Duration(config.ApplyIntervalInMilliseconds * int(time.Millisecond))
		} else {
			npmV2DataplaneCfg.ApplyInterval = time.Duration(npmconfig.DefaultConfig.ApplyIntervalInMilliseconds * int(time.Millisecond))
		}

		if config.WindowsNetworkName == "" {
			npmV2DataplaneCfg.NetworkName = util.AzureNetworkName
		} else {
			npmV2DataplaneCfg.NetworkName = config.WindowsNetworkName
		}

		npmV2DataplaneCfg.PlaceAzureChainFirst = config.Toggles.PlaceAzureChainFirst
		if config.Toggles.ApplyIPSetsOnNeed {
			npmV2DataplaneCfg.IPSetMode = ipsets.ApplyOnNeed
		} else {
			npmV2DataplaneCfg.IPSetMode = ipsets.ApplyAllIPSets
		}

		var nodeIP string
		if util.IsWindowsDP() {
			nodeIP, err = util.NodeIP()
			if err != nil {
				metrics.SendErrorLogAndMetric(util.NpmID, "error: failed to get node IP while booting up: %v", err)
				return fmt.Errorf("failed to get node IP while booting up: %w", err)
			}
			klog.Infof("node IP is %s", nodeIP)
		}
		npmV2DataplaneCfg.NodeIP = nodeIP

		dp, err = dataplane.NewDataPlane(models.GetNodeName(), common.NewIOShim(), npmV2DataplaneCfg, stopChannel)
		if err != nil {
			metrics.SendErrorLogAndMetric(util.NpmID, "error: failed to create dataplane with error %v", err)
			return fmt.Errorf("failed to create dataplane with error %w", err)
		}
		dp.RunPeriodicTasks()
	}
	npMgr := npm.NewNetworkPolicyManager(config, factory, dp, exec.New(), version, k8sServerVersion)
	err = metrics.CreateTelemetryHandle(config.NPMVersion(), version, npm.GetAIMetadata())
	if err != nil {
		klog.Infof("CreateTelemetryHandle failed with error %v. AITelemetry is not initialized.", err)
	}

	go restserver.NPMRestServerListenAndServe(config, npMgr)

	metrics.SendLog(util.NpmID, "starting NPM", metrics.PrintLog)
	if err = npMgr.Start(config, stopChannel); err != nil {
		metrics.SendErrorLogAndMetric(util.NpmID, "Failed to start NPM due to %+v", err)
		return fmt.Errorf("failed to start with err: %w", err)
	}

	select {}
}

func initLogging() error {
	log.SetName("azure-npm")
	log.SetLevel(log.LevelInfo)
	if err := log.SetTargetLogDirectory(log.TargetStdout, ""); err != nil {
		log.Logf("Failed to configure logging, err:%v.", err)
		return fmt.Errorf("%w", err)
	}

	return nil
}

func k8sServerVersion(kubeclientset kubernetes.Interface) *k8sversion.Info {
	var err error
	var serverVersion *k8sversion.Info
	for ticker, start := time.NewTicker(1*time.Second).C, time.Now(); time.Since(start) < time.Minute*1; {
		<-ticker
		serverVersion, err = kubeclientset.Discovery().ServerVersion()
		if err == nil {
			break
		}
	}

	if err != nil {
		metrics.SendErrorLogAndMetric(util.NpmID, "Error: failed to retrieving kubernetes version with err: %s", err.Error())
	}
	return serverVersion
}
