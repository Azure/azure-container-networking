package main

import (
	"fmt"
	"time"

	"github.com/Azure/azure-container-networking/cni/npm"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

/*
func init() {
	log.SetName("NetworkPolicyManager")
	log.SetLevel(log.LevelInfo)
	err := log.SetTarget(log.TargetLogfile)
	if err != nil {
		log.Printf("[cni-npm] Failed to configure logging, err:%v.\n", err)
		return err
	}
}
*/
func main() {
	// Creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}

	// Creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Printf("[cni-npm] clientset creation failed with error %v.\n", err)
		panic(err.Error())
	}

	factory := informers.NewSharedInformerFactory(clientset, time.Hour*24)
	npMgr := npm.NewNetworkPolicyManager(clientset, factory)
	err = npMgr.Run(wait.NeverStop)
	if err != nil {
		fmt.Printf("[cni-npm] npm failed with error %v.\n", err)
		panic(err.Error)
	}
	select {}
}
