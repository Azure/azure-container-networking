package main

import (
	"time"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Version is populated by make during build.
var version string

func initLogging() error {
	log.SetName("azure-npm")
	log.SetLevel(log.LevelInfo)
	err := log.SetTarget(log.TargetLogfile)
	if err != nil {
		log.Printf("[Azure-NPM] Failed to configure logging, err:%v.\n", err)
		return err
	}

	return nil
}

func main() {
	if err := initLogging(); err != nil {
		panic(err.Error())
	}

	// Creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}

	// Creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("[Azure-NPM] clientset creation failed with error %v.\n", err)
		panic(err.Error())
	}

	factory := informers.NewSharedInformerFactory(clientset, time.Hour*24)

	npMgr := npm.NewNetworkPolicyManager(clientset, factory, version)
	err = npMgr.Run(wait.NeverStop)
	if err != nil {
		log.Printf("[Azure-NPM] npm failed with error %v.", err)
		panic(err.Error)
	}

	go npMgr.RunReportManager()

	select {}
}
