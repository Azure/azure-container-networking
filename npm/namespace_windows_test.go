// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"testing"

	"github.com/Azure/azure-container-networking/npm/hnsm"
	"github.com/Azure/azure-container-networking/telemetry"

	"github.com/Azure/azure-container-networking/npm/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAllNsList(t *testing.T) {
	npMgr := &NetworkPolicyManager{}

	tMgr := hnsm.NewTagManager()
	if err := tMgr.Save(util.TagTestConfigFile); err != nil {
		t.Errorf("TestAllNsList failed @ tMgr.Save")
	}

	defer func() {
		if err := tMgr.Restore(util.TagTestConfigFile); err != nil {
			t.Errorf("TestAllNsList failed @ tMgr.Restore")
		}
	}()

	if err := npMgr.InitAllNsList(); err != nil {
		t.Errorf("TestAllNsList failed @ InitAllNsList")
	}

	if err := npMgr.UninitAllNsList(); err != nil {
		t.Errorf("TestAllNsList failed @ UninitAllNsList")
	}
}

func TestAddNamespace(t *testing.T) {
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

	tMgr := hnsm.NewTagManager()
	if err := tMgr.Save(util.TagTestConfigFile); err != nil {
		t.Errorf("TestAddNamespace failed @ tMgr.Save")
	}

	defer func() {
		if err := tMgr.Restore(util.TagTestConfigFile); err != nil {
			t.Errorf("TestAddNamespace failed @ tMgr.Restore")
		}
	}()

	nsObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-namespace",
			Labels: map[string]string{
				"app": "test-namespace",
			},
		},
	}

	if err := npMgr.AddNamespace(nsObj); err != nil {
		t.Errorf("TestAddNamespace @ npMgr.AddNamespace")
	}
}

func TestUpdateNamespace(t *testing.T) {
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

	tMgr := hnsm.NewTagManager()
	if err := tMgr.Save(util.TagTestConfigFile); err != nil {
		t.Errorf("TestUpdateNamespace failed @ tMgr.Save")
	}

	defer func() {
		if err := tMgr.Restore(util.TagTestConfigFile); err != nil {
			t.Errorf("TestUpdateNamespace failed @ tMgr.Restore")
		}
	}()

	oldNsObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "old-test-namespace",
			Labels: map[string]string{
				"app": "old-test-namespace",
			},
		},
	}

	newNsObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "new-test-namespace",
			Labels: map[string]string{
				"app": "new-test-namespace",
			},
		},
	}

	if err := npMgr.AddNamespace(oldNsObj); err != nil {
		t.Errorf("TestUpdateNamespace failed @ npMgr.AddNamespace")
	}

	if err := npMgr.UpdateNamespace(oldNsObj, newNsObj); err != nil {
		t.Errorf("TestUpdateNamespace failed @ npMgr.UpdateNamespace")
	}
}

func TestDeleteNamespace(t *testing.T) {
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

	tMgr := hnsm.NewTagManager()
	if err := tMgr.Save(util.TagTestConfigFile); err != nil {
		t.Errorf("TestDeleteNamespace failed @ tMgr.Save")
	}

	defer func() {
		if err := tMgr.Restore(util.TagTestConfigFile); err != nil {
			t.Errorf("TestDeleteNamespace failed @ tMgr.Restore")
		}
	}()

	nsObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-namespace",
			Labels: map[string]string{
				"app": "test-namespace",
			},
		},
	}

	if err := npMgr.AddNamespace(nsObj); err != nil {
		t.Errorf("TestDeleteNamespace @ npMgr.AddNamespace")
	}

	if err := npMgr.DeleteNamespace(nsObj); err != nil {
		t.Errorf("TestDeleteNamespace @ npMgr.DeleteNamespace")
	}
}

func TestMain(m *testing.M) {
	aclMgr := hnsm.NewACLPolicyManager()
	aclMgr.Save(util.ACLConfigFile)

	tMgr := hnsm.NewTagManager()
	tMgr.Save(util.TagConfigFile)

	m.Run()

	aclMgr.Restore(util.ACLConfigFile)
	tMgr.Restore(util.TagConfigFile)
}
