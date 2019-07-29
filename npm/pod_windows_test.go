// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"testing"

	"github.com/kalebmorris/azure-container-networking/npm/hcnm"
	"github.com/kalebmorris/azure-container-networking/npm/util"
	"github.com/kalebmorris/azure-container-networking/telemetry"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAddPod(t *testing.T) {
	npMgr := &NetworkPolicyManager{
		nsMap:            make(map[string]*namespace),
		TelemetryEnabled: false,
		reportManager: &telemetry.ReportManager{
			ContentType: telemetry.ContentType,
			Report:      &telemetry.NPMReport{},
		},
	}

	allNs, err := newNs(util.KubeAllNamespacesFlag)
	if err != nil {
		panic(err.Error)
	}
	npMgr.nsMap[util.KubeAllNamespacesFlag] = allNs

	tMgr := hcnm.NewTagManager()
	if err := tMgr.Save(util.TagTestConfigFile); err != nil {
		t.Errorf("TestAddPod failed @ tMgr.Save")
	}

	defer func() {
		if err := tMgr.Restore(util.TagTestConfigFile); err != nil {
			t.Errorf("TestAddPod failed @ tMgr.Restore")
		}
	}()

	podObj := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-pod",
			Labels: map[string]string{
				"app": "test-pod",
			},
		},
		Status: corev1.PodStatus{
			Phase: "Running",
			PodIP: "1.2.3.4",
		},
	}
	if err := npMgr.AddPod(podObj); err != nil {
		t.Errorf("TestAddPod failed @ AddPod")
	}
}

func TestUpdatePod(t *testing.T) {
	npMgr := &NetworkPolicyManager{
		nsMap:            make(map[string]*namespace),
		TelemetryEnabled: false,
		reportManager: &telemetry.ReportManager{
			ContentType: telemetry.ContentType,
			Report:      &telemetry.NPMReport{},
		},
	}

	allNs, err := newNs(util.KubeAllNamespacesFlag)
	if err != nil {
		panic(err.Error)
	}
	npMgr.nsMap[util.KubeAllNamespacesFlag] = allNs

	tMgr := hcnm.NewTagManager()
	if err := tMgr.Save(util.TagTestConfigFile); err != nil {
		t.Errorf("TestUpdatePod failed @ tMgr.Save")
	}

	defer func() {
		if err := tMgr.Restore(util.TagTestConfigFile); err != nil {
			t.Errorf("TestUpdatePod failed @ tMgr.Restore")
		}
	}()

	oldPodObj := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "old-test-pod",
			Labels: map[string]string{
				"app": "old-test-pod",
			},
		},
		Status: corev1.PodStatus{
			Phase: "Running",
			PodIP: "1.2.3.4",
		},
	}

	newPodObj := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "new-test-pod",
			Labels: map[string]string{
				"app": "new-test-pod",
			},
		},
		Status: corev1.PodStatus{
			Phase: "Running",
			PodIP: "4.3.2.1",
		},
	}

	if err := npMgr.AddPod(oldPodObj); err != nil {
		t.Errorf("TestUpdatePod failed @ AddPod")
	}

	if err := npMgr.UpdatePod(oldPodObj, newPodObj); err != nil {
		t.Errorf("TestUpdatePod failed @ UpdatePod")
	}
}

func TestDeletePod(t *testing.T) {
	npMgr := &NetworkPolicyManager{
		nsMap:            make(map[string]*namespace),
		TelemetryEnabled: false,
		reportManager: &telemetry.ReportManager{
			ContentType: telemetry.ContentType,
			Report:      &telemetry.NPMReport{},
		},
	}

	allNs, err := newNs(util.KubeAllNamespacesFlag)
	if err != nil {
		panic(err.Error)
	}
	npMgr.nsMap[util.KubeAllNamespacesFlag] = allNs

	tMgr := hcnm.NewTagManager()
	if err := tMgr.Save(util.TagTestConfigFile); err != nil {
		t.Errorf("TestDeletePod failed @ tMgr.Save")
	}

	defer func() {
		if err := tMgr.Restore(util.TagTestConfigFile); err != nil {
			t.Errorf("TestDeletePod failed @ tMgr.Restore")
		}
	}()

	podObj := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-pod",
			Labels: map[string]string{
				"app": "test-pod",
			},
		},
		Status: corev1.PodStatus{
			Phase: "Running",
			PodIP: "1.2.3.4",
		},
	}
	if err := npMgr.AddPod(podObj); err != nil {
		t.Errorf("TestDeletePod failed @ AddPod")
	}

	if err := npMgr.DeletePod(podObj); err != nil {
		t.Errorf("TestDeletePod failed @ DeletePod")
	}
}
