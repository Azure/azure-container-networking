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

func initLogging() error {
	log.SetName("azure-npm")
	log.SetLevel(log.LevelInfo)
	//err := log.SetTarget(log.TargetLogfile)
	err := log.SetTarget(log.TargetStderr)
	if err != nil {
		log.Printf("[cni-npm] Failed to configure logging, err:%v.\n", err)
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
		log.Printf("[cni-npm] clientset creation failed with error %v.\n", err)
		panic(err.Error())
	}

	factory := informers.NewSharedInformerFactory(clientset, time.Hour*24)
	npMgr := npm.NewNetworkPolicyManager(clientset, factory)
	err = npMgr.Run(wait.NeverStop)
	if err != nil {
		log.Printf("[cni-npm] npm failed with error %v.\n", err)
		panic(err.Error)
	}

	select {}
}
