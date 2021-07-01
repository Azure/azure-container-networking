// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package main

import (
	"math/rand"
	"time"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm"
	restserver "github.com/Azure/azure-container-networking/npm/http/server"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/utils/exec"
)

const (
	waitForTelemetryInSeconds = 60
	resyncPeriodInMinutes     = 15
)

// Version is populated by make during build.
var version string

func initLogging() error {
	log.SetName("azure-npm")
	log.SetLevel(log.LevelInfo)
	if err := log.SetTargetLogDirectory(log.TargetStdout, ""); err != nil {
		log.Logf("Failed to configure logging, err:%v.", err)
		return err
	}

	return nil
}

func main() {
	var err error

	defer func() {
		if r := recover(); r != nil {
			log.Logf("recovered from error: %v", err)
		}
	}()

	if err = initLogging(); err != nil {
		panic(err.Error())
	}

	metrics.InitializeAll()

	// Creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}

	// Creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Logf("clientset creation failed with error %v.", err)
		panic(err.Error())
	}

	// Setting reSyncPeriod to 15 mins
	minResyncPeriod := resyncPeriodInMinutes * time.Minute

	// Adding some randomness so all NPM pods will not request for info at once.
	factor := rand.Float64() + 1
	resyncPeriod := time.Duration(float64(minResyncPeriod.Nanoseconds()) * factor)

	log.Logf("[INFO] Resync period for NPM pod is set to %d.", int(resyncPeriod/time.Minute))
	factory := informers.NewSharedInformerFactory(clientset, resyncPeriod)

	npMgr := npm.NewNetworkPolicyManager(clientset, factory, exec.New(), version)
	metrics.CreateTelemetryHandle(npMgr.GetAppVersion(), npm.GetAIMetadata())

	restserver := restserver.NewNpmRestServer(restserver.DefaultHTTPListeningAddress)
	go restserver.NPMRestServerListenAndServe(npMgr)

	if err = npMgr.Start(wait.NeverStop); err != nil {
		log.Logf("npm failed with error %v.", err)
		panic(err.Error)
	}

	select {}
}
