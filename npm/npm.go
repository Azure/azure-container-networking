// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"fmt"
	"os"
	"reflect"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/Azure/azure-container-networking/telemetry"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	networkinginformers "k8s.io/client-go/informers/networking/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const (
	hostNetAgentURLForNpm           = "http://168.63.129.16/machine/plugins?comp=netagent&type=npmreport"
	contentType                     = "application/json"
	telemetryRetryWaitTimeInSeconds = 60
)

// NetworkPolicyManager contains informers for pod, namespace and networkpolicy.
type NetworkPolicyManager struct {
	sync.Mutex
	clientset *kubernetes.Clientset

	informerFactory informers.SharedInformerFactory
	podInformer     coreinformers.PodInformer
	nsInformer      coreinformers.NamespaceInformer
	npInformer      networkinginformers.NetworkPolicyInformer

	nodeName               string
	nsMap                  map[string]*namespace
	isAzureNpmChainCreated bool

	clusterState  telemetry.ClusterState
	reportManager *telemetry.ReportManager

	serverVersion    *version.Info
	TelemetryEnabled bool
}

// GetClusterState returns current cluster state.
func (npMgr *NetworkPolicyManager) GetClusterState() telemetry.ClusterState {
	pods, err := npMgr.clientset.CoreV1().Pods("").List(metav1.ListOptions{})
	if err != nil {
		log.Printf("Error Listing pods in GetClusterState")
	}

	namespaces, err := npMgr.clientset.CoreV1().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		log.Printf("Error Listing namespaces in GetClusterState")
	}

	networkpolicies, err := npMgr.clientset.NetworkingV1().NetworkPolicies("").List(metav1.ListOptions{})
	if err != nil {
		log.Printf("Error Listing networkpolicies in GetClusterState")
	}

	npMgr.clusterState.PodCount = len(pods.Items)
	npMgr.clusterState.NsCount = len(namespaces.Items)
	npMgr.clusterState.NwPolicyCount = len(networkpolicies.Items)

	return npMgr.clusterState
}

// UpdateAndSendReport updates the npm report then send it.
// This function should only be called when npMgr is locked.
func (npMgr *NetworkPolicyManager) UpdateAndSendReport(err error, eventMsg string) error {
	if !npMgr.TelemetryEnabled {
		return nil
	}

	clusterState := npMgr.GetClusterState()
	v := reflect.ValueOf(npMgr.reportManager.Report).Elem().FieldByName("ClusterState")
	if v.CanSet() {
		v.FieldByName("PodCount").SetInt(int64(clusterState.PodCount))
		v.FieldByName("NsCount").SetInt(int64(clusterState.NsCount))
		v.FieldByName("NwPolicyCount").SetInt(int64(clusterState.NwPolicyCount))
	}

	reflect.ValueOf(npMgr.reportManager.Report).Elem().FieldByName("EventMessage").SetString(eventMsg)

	if err != nil {
		reflect.ValueOf(npMgr.reportManager.Report).Elem().FieldByName("EventMessage").SetString(err.Error())
	}

	var telemetryBuffer *telemetry.TelemetryBuffer
	connectToTelemetryServer(telemetryBuffer)

	return npMgr.reportManager.SendReport(telemetryBuffer)
}

// Run starts shared informers and waits for the shared informer cache to sync.
func (npMgr *NetworkPolicyManager) Run(stopCh <-chan struct{}) error {
	// Starts all informers manufactured by npMgr's informerFactory.
	npMgr.informerFactory.Start(stopCh)

	// Wait for the initial sync of local cache.
	if !cache.WaitForCacheSync(stopCh, npMgr.podInformer.Informer().HasSynced) {
		return fmt.Errorf("Pod informer failed to sync")
	}

	if !cache.WaitForCacheSync(stopCh, npMgr.nsInformer.Informer().HasSynced) {
		return fmt.Errorf("Namespace informer failed to sync")
	}

	if !cache.WaitForCacheSync(stopCh, npMgr.npInformer.Informer().HasSynced) {
		return fmt.Errorf("Namespace informer failed to sync")
	}

	return nil
}

func connectToTelemetryServer(telemetryBuffer *telemetry.TelemetryBuffer) {
	for {
		telemetryBuffer = telemetry.NewTelemetryBuffer("")
		err := telemetryBuffer.StartServer()
		if err == nil || telemetryBuffer.FdExists {
			connErr := telemetryBuffer.Connect()
			if connErr == nil {
				break
			}

			log.Printf("[NPM-Telemetry] Failed to establish telemetry manager connection.")
			time.Sleep(time.Second * telemetryRetryWaitTimeInSeconds)
		}
	}
}

// RunReportManager starts NPMReportManager and send telemetry periodically.
func (npMgr *NetworkPolicyManager) RunReportManager() {
	if !npMgr.TelemetryEnabled {
		return
	}

	var telemetryBuffer *telemetry.TelemetryBuffer
	connectToTelemetryServer(telemetryBuffer)

	go telemetryBuffer.BufferAndPushData(time.Duration(0))

	for {
		clusterState := npMgr.GetClusterState()
		v := reflect.ValueOf(npMgr.reportManager.Report).Elem().FieldByName("ClusterState")
		if v.CanSet() {
			v.FieldByName("PodCount").SetInt(int64(clusterState.PodCount))
			v.FieldByName("NsCount").SetInt(int64(clusterState.NsCount))
			v.FieldByName("NwPolicyCount").SetInt(int64(clusterState.NwPolicyCount))
		}

		if err := npMgr.reportManager.SendReport(telemetryBuffer); err != nil {
			log.Printf("[NPM-Telemetry] Error sending NPM telemetry report")
			connectToTelemetryServer(telemetryBuffer)
		}

		time.Sleep(5 * time.Minute)
	}
}

// NewNetworkPolicyManager creates a NetworkPolicyManager
func NewNetworkPolicyManager(clientset *kubernetes.Clientset, informerFactory informers.SharedInformerFactory, npmVersion string) *NetworkPolicyManager {

	podInformer := informerFactory.Core().V1().Pods()
	nsInformer := informerFactory.Core().V1().Namespaces()
	npInformer := informerFactory.Networking().V1().NetworkPolicies()

	serverVersion, err := clientset.ServerVersion()
	if err != nil {
		log.Printf("Error retrieving server version")
		panic(err.Error)
	}
	log.Printf("API server version: %+v", serverVersion)

	if err = util.SetIsNewNwPolicyVerFlag(serverVersion); err != nil {
		log.Printf("Error setting IsNewNwPolicyVerFlag")
		panic(err.Error)
	}

	npMgr := &NetworkPolicyManager{
		clientset:              clientset,
		informerFactory:        informerFactory,
		podInformer:            podInformer,
		nsInformer:             nsInformer,
		npInformer:             npInformer,
		nodeName:               os.Getenv("HOSTNAME"),
		nsMap:                  make(map[string]*namespace),
		isAzureNpmChainCreated: false,
		clusterState: telemetry.ClusterState{
			PodCount:      0,
			NsCount:       0,
			NwPolicyCount: 0,
		},
		reportManager: &telemetry.ReportManager{
			HostNetAgentURL: hostNetAgentURLForNpm,
			ContentType:     contentType,
			Report:          &telemetry.NPMReport{},
		},
		serverVersion:    serverVersion,
		TelemetryEnabled: true,
	}

	clusterID := util.GetClusterID(npMgr.nodeName)
	clusterState := npMgr.GetClusterState()
	npMgr.reportManager.Report.(*telemetry.NPMReport).GetReport(clusterID, npMgr.nodeName, npmVersion, serverVersion.GitVersion, clusterState)

	allNs, err := newNs(util.KubeAllNamespacesFlag)
	if err != nil {
		log.Printf("Error creating all-namespace")
		panic(err.Error)
	}
	npMgr.nsMap[util.KubeAllNamespacesFlag] = allNs

	podInformer.Informer().AddEventHandler(
		// Pod event handlers
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				npMgr.AddPod(obj.(*corev1.Pod))
			},
			UpdateFunc: func(old, new interface{}) {
				npMgr.UpdatePod(old.(*corev1.Pod), new.(*corev1.Pod))
			},
			DeleteFunc: func(obj interface{}) {
				npMgr.DeletePod(obj.(*corev1.Pod))
			},
		},
	)

	nsInformer.Informer().AddEventHandler(
		// Namespace event handlers
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				npMgr.AddNamespace(obj.(*corev1.Namespace))
			},
			UpdateFunc: func(old, new interface{}) {
				npMgr.UpdateNamespace(old.(*corev1.Namespace), new.(*corev1.Namespace))
			},
			DeleteFunc: func(obj interface{}) {
				npMgr.DeleteNamespace(obj.(*corev1.Namespace))
			},
		},
	)

	npInformer.Informer().AddEventHandler(
		// Network policy event handlers
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				npMgr.AddNetworkPolicy(obj.(*networkingv1.NetworkPolicy))
			},
			UpdateFunc: func(old, new interface{}) {
				npMgr.UpdateNetworkPolicy(old.(*networkingv1.NetworkPolicy), new.(*networkingv1.NetworkPolicy))
			},
			DeleteFunc: func(obj interface{}) {
				npMgr.DeleteNetworkPolicy(obj.(*networkingv1.NetworkPolicy))
			},
		},
	)

	return npMgr
}
