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
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/util/wait"
	k8sversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"k8s.io/utils/exec"
)

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
			if err := viper.ReadInConfig(); err == nil {
				klog.Info("Using config file: ", viper.ConfigFileUsed())
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
				return fmt.Errorf("failed to load config with error %w", err)
			}

			return start(*config)
		},
	}
	return startNPMCmd
}

func start(config npmconfig.Config) error {
	klog.Infof("loaded config: %+v", config)
	klog.Infof("Start NPM version: %s", version)

	var err error
	defer func() {
		if r := recover(); r != nil {
			klog.Infof("recovered from error: %v", err)
		}
	}()

	if err = initLogging(); err != nil {
		return err
	}

	metrics.InitializeAll()

	// Creates the in-cluster config
	k8sConfig, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("failed to load in cluster config: %w", err)
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
	factor := rand.Float64() + 1
	resyncPeriod := time.Duration(float64(minResyncPeriod.Nanoseconds()) * factor)
	klog.Infof("Resync period for NPM pod is set to %d.", int(resyncPeriod/time.Minute))
	factory := informers.NewSharedInformerFactory(clientset, resyncPeriod)

	k8sServerVersion := k8sServerVersion(clientset)

	var dp dataplane.GenericDataplane
	if config.Toggles.EnableV2Controllers {
		dp, err = dataplane.NewDataPlane(npm.GetNodeName(), common.NewIOShim())
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
		metrics.SendErrorLogAndMetric(util.NpmID, "Failed to start NPM due to %s", err)
		panic(err.Error)
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
