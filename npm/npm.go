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
	telemetryRetryTimeInSeconds   = 60
	heartbeatIntervalInMinutes    = 30
)

// GetClusterState returns current cluster state.
func (npMgr *NetworkPolicyManager) GetClusterState() telemetry.ClusterState {
	pods, err := npMgr.clientset.CoreV1().Pods("").List(metav1.ListOptions{})
	if err != nil {
		log.Logf("Error: Failed to list pods in GetClusterState")
	}

	namespaces, err := npMgr.clientset.CoreV1().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		log.Logf("Error: Failed to list namespaces in GetClusterState")
	}

	networkpolicies, err := npMgr.clientset.NetworkingV1().NetworkPolicies("").List(metav1.ListOptions{})
	if err != nil {
		log.Logf("Error: Failed to list networkpolicies in GetClusterState")
	}

	npMgr.clusterState.PodCount = len(pods.Items)
	npMgr.clusterState.NsCount = len(namespaces.Items)
	npMgr.clusterState.NwPolicyCount = len(networkpolicies.Items)

	return npMgr.clusterState
}

// SendNpmTelemetry updates the npm report then send it.
func (npMgr *NetworkPolicyManager) SendNpmTelemetry() {
	if !npMgr.TelemetryEnabled {
		return
	}

CONNECT:
	tb := telemetry.NewTelemetryBuffer("")
	for {
		tb.TryToConnectToTelemetryService()
		if tb.Connected {
			break
		}

		time.Sleep(time.Second * telemetryRetryTimeInSeconds)
	}

	heartbeat := time.NewTicker(time.Minute * heartbeatIntervalInMinutes).C
	report := npMgr.reportManager.Report
	for {
		select {
		case <-heartbeat:
			clusterState := npMgr.GetClusterState()
			v := reflect.ValueOf(report).Elem().FieldByName("ClusterState")
			if v.CanSet() {
				v.FieldByName("PodCount").SetInt(int64(clusterState.PodCount))
				v.FieldByName("NsCount").SetInt(int64(clusterState.NsCount))
				v.FieldByName("NwPolicyCount").SetInt(int64(clusterState.NwPolicyCount))
			}
			reflect.ValueOf(report).Elem().FieldByName("ErrorMessage").SetString("heartbeat")
		case msg := <-reports:
			reflect.ValueOf(report).Elem().FieldByName("ErrorMessage").SetString(msg.(string))
			fmt.Println(msg.(string))
		}

		reflect.ValueOf(report).Elem().FieldByName("Timestamp").SetString(time.Now().UTC().String())
		// TODO: Remove below line after the host change is rolled out
		reflect.ValueOf(report).Elem().FieldByName("EventMessage").SetString(time.Now().UTC().String())

		report, err := npMgr.reportManager.ReportToBytes()
		if err != nil {
			log.Logf("ReportToBytes failed: %v", err)
			continue
		}

		// If write fails, try to re-establish connections as server/client
		if _, err = tb.Write(report); err != nil {
			log.Logf("Telemetry write failed: %v", err)
			tb.Close()
			goto CONNECT
		}
	}
}
