// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package main

import (
	"bytes"
	"encoding/json"
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
		IPSetMode:   ipsets.ApplyAllIPSets,
		NetworkName: "azure", // FIXME  should be specified in DP config instead
	},
	PolicyManagerCfg: &policies.PolicyManagerCfg{
		PolicyMode: policies.IPSetPolicyMode,
	},
}

func newStartNPMCmd() *cobra.Command {
	// getTuplesCmd represents the getTuples command
	startNPMCmd := &cobra.Command{
		Use:   "start",
		Short: "Starts the Azure NPM process",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			viper.AutomaticEnv() // read in environment variables that match
			viper.SetDefault(npmconfig.ConfigEnvPath, npmconfig.GetConfigPath())
			cfgFile := viper.GetString(npmconfig.ConfigEnvPath)
			viper.SetConfigFile(cfgFile)

			// If a config file is found, read it in.
			// NOTE: there is no config merging with default, if config is loaded, options must be set
			if err := viper.ReadInConfig(); err == nil {
				klog.Infof("Using config file: %+v", viper.ConfigFileUsed())
			} else {
				klog.Infof("Failed to load config from env %s: %v", npmconfig.ConfigEnvPath, err)
				b, _ := json.Marshal(npmconfig.DefaultConfig)
				err := viper.ReadConfig(bytes.NewBuffer(b))
				if err != nil {
					return fmt.Errorf("failed to read in default with err %w", err)
				}
			}

			return nil
		},

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
	klog.Infof("Start NPM version: %s", version)

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
	if config.Toggles.EnableV2NPM {
		dp, err = dataplane.NewDataPlane(npm.GetNodeName(), common.NewIOShim(), npmV2DataplaneCfg)
		if err != nil {
			return fmt.Errorf("failed to create dataplane with error %w", err)
		}
	}
	npMgr := npm.NewNetworkPolicyManager(config, factory, dp, exec.New(), version, k8sServerVersion)
	err = metrics.CreateTelemetryHandle(version, npm.GetAIMetadata())
	if err != nil {
		klog.Infof("CreateTelemetryHandle failed with error %v.", err)
		return fmt.Errorf("CreateTelemetryHandle failed with error %w", err)
	}

	go restserver.NPMRestServerListenAndServe(config, npMgr)

	if err = npMgr.Start(config, wait.NeverStop); err != nil {
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
		metrics.SendErrorLogAndMetric(util.NpmID, "Error: failed to retrieving kubernetes version")
		panic(err.Error)
	}

	if err = util.SetIsNewNwPolicyVerFlag(serverVersion); err != nil {
		metrics.SendErrorLogAndMetric(util.NpmID, "Error: failed to set IsNewNwPolicyVerFlag")
		panic(err.Error)
	}
	return serverVersion
}
