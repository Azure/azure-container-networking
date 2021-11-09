// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package controllers

import (
	"reflect"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	dpmocks "github.com/Azure/azure-container-networking/npm/pkg/dataplane/mocks"
	"github.com/Azure/azure-container-networking/npm/util"
	gomock "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

var noResyncPeriodFunc = func() time.Duration { return 0 }

type expectedNsValues struct {
	expectedLenOfNsMap     int
	expectedLenOfWorkQueue int
}

type nameSpaceFixture struct {
	t *testing.T

	nsLister []*corev1.Namespace
	// Objects from here preloaded into NewSimpleFake.
	kubeobjects []runtime.Object

	dp           dataplane.GenericDataplane
	nsController *NamespaceController
	kubeInformer kubeinformers.SharedInformerFactory
}

func newNsFixture(t *testing.T, dp dataplane.GenericDataplane) *nameSpaceFixture {
	f := &nameSpaceFixture{
		t:           t,
		nsLister:    []*corev1.Namespace{},
		kubeobjects: []runtime.Object{},
		dp:          dp,
	}
	return f
}

func (f *nameSpaceFixture) newNsController(_ chan struct{}) {
	kubeclient := k8sfake.NewSimpleClientset(f.kubeobjects...)
	f.kubeInformer = kubeinformers.NewSharedInformerFactory(kubeclient, noResyncPeriodFunc())

	npmNamespaceCache := &NpmNamespaceCache{NsMap: make(map[string]*Namespace)}
	f.nsController = NewNamespaceController(
		f.kubeInformer.Core().V1().Namespaces(), f.dp, npmNamespaceCache)

	for _, ns := range f.nsLister {
		err := f.kubeInformer.Core().V1().Namespaces().Informer().GetIndexer().Add(ns)
		if err != nil {
			f.t.Errorf("Error adding namespace to informer: %v", err)
		}
	}
	// Do not start informer to avoid unnecessary event triggers.
	// (TODO) Leave stopCh and below commented code to enhance UTs to even check event triggers as well later if possible
	// f.kubeInformer.Start()
}

func newNameSpace(name, rv string, labels map[string]string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Labels:          labels,
			ResourceVersion: rv,
		},
	}
}

func addNamespace(t *testing.T, f *nameSpaceFixture, nsObj *corev1.Namespace) {
	t.Logf("Calling add namespace event")
	f.nsController.addNamespace(nsObj)
	if f.nsController.workqueue.Len() == 0 {
		t.Logf("Add Namespace: worker queue length is 0 ")
		return
	}
	f.nsController.processNextWorkItem()
}

func updateNamespace(t *testing.T, f *nameSpaceFixture, oldNsObj, newNsObj *corev1.Namespace) {
	addNamespace(t, f, oldNsObj)
	t.Logf("Complete add namespace event")

	t.Logf("Updating kubeinformer namespace object")
	err := f.kubeInformer.Core().V1().Namespaces().Informer().GetIndexer().Update(newNsObj)
	if err != nil {
		f.t.Errorf("Error updating namespace to informer: %v", err)
	}

	t.Logf("Calling update namespace event")
	f.nsController.updateNamespace(oldNsObj, newNsObj)
	if f.nsController.workqueue.Len() == 0 {
		t.Logf("Update Namespace: worker queue length is 0 ")
		return
	}
	f.nsController.processNextWorkItem()
}

func deleteNamespace(t *testing.T, f *nameSpaceFixture, nsObj *corev1.Namespace, isDeletedFinalStateUnknownObject IsDeletedFinalStateUnknownObject) {
	addNamespace(t, f, nsObj)
	t.Logf("Complete add namespace event")

	t.Logf("Updating kubeinformer namespace object")
	err := f.kubeInformer.Core().V1().Namespaces().Informer().GetIndexer().Delete(nsObj)
	if err != nil {
		f.t.Errorf("Error deleting namespace to informer: %v", err)
	}
	t.Logf("Calling delete namespace event")
	if isDeletedFinalStateUnknownObject {
		tombstone := cache.DeletedFinalStateUnknown{
			Key: nsObj.Name,
			Obj: nsObj,
		}
		f.nsController.deleteNamespace(tombstone)
	} else {
		f.nsController.deleteNamespace(nsObj)
	}

	if f.nsController.workqueue.Len() == 0 {
		t.Logf("Delete Namespace: worker queue length is 0 ")
		return
	}
	f.nsController.processNextWorkItem()
}

func TestAddNamespace(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dp := dpmocks.NewMockGenericDataplane(ctrl)
	f := newNsFixture(t, dp)

	nsObj := newNameSpace(
		"test-namespace",
		"0",
		map[string]string{
			"app": "test-namespace",
		},
	)
	f.nsLister = append(f.nsLister, nsObj)
	f.kubeobjects = append(f.kubeobjects, nsObj)

	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newNsController(stopCh)

	// DPMock expect section
	setsToAddNamespaceTo := []*ipsets.IPSetMetadata{
		ipsets.NewIPSetMetadata("test-namespace", ipsets.Namespace),
		kubeAllNamespaces,
		ipsets.NewIPSetMetadata("app", ipsets.KeyLabelOfNamespace),
		ipsets.NewIPSetMetadata("app:test-namespace", ipsets.KeyValueLabelOfNamespace),
	}

	dp.EXPECT().AddToLists(setsToAddNamespaceTo[1:], setsToAddNamespaceTo[:1]).Return(nil).Times(1)
	dp.EXPECT().ApplyDataPlane().Return(nil).Times(1)

	// Call into add NS
	addNamespace(t, f, nsObj)

	// Cache and state validation section
	testCases := []expectedNsValues{
		{1, 0},
	}
	checkNsTestResult("TestAddNamespace", f, testCases)

	if _, exists := f.nsController.npmNamespaceCache.NsMap[nsObj.Name]; !exists {
		t.Errorf("TestAddNamespace failed @ npMgr.nsMap check")
	}
}

func TestUpdateNamespace(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dp := dpmocks.NewMockGenericDataplane(ctrl)
	f := newNsFixture(t, dp)

	oldNsObj := newNameSpace(
		"test-namespace",
		"0",
		map[string]string{
			"app": "test-namespace",
		},
	)

	newNsObj := newNameSpace(
		"test-namespace",
		"1",
		map[string]string{
			"app": "new-test-namespace",
		},
	)
	f.nsLister = append(f.nsLister, oldNsObj)
	f.kubeobjects = append(f.kubeobjects, oldNsObj)

	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newNsController(stopCh)

	// DPMock expect section
	setsToAddNamespaceTo := []*ipsets.IPSetMetadata{
		ipsets.NewIPSetMetadata("test-namespace", ipsets.Namespace),
		kubeAllNamespaces,
		ipsets.NewIPSetMetadata("app", ipsets.KeyLabelOfNamespace),
		ipsets.NewIPSetMetadata("app:test-namespace", ipsets.KeyValueLabelOfNamespace),
	}

	dp.EXPECT().AddToLists(setsToAddNamespaceTo[1:], setsToAddNamespaceTo[:1]).Return(nil).Times(1)
	dp.EXPECT().ApplyDataPlane().Return(nil).Times(2)
	dp.EXPECT().RemoveFromList(setsToAddNamespaceTo[3], setsToAddNamespaceTo[:1]).Return(nil).Times(1)

	setsToAddNamespaceToNew := []*ipsets.IPSetMetadata{
		ipsets.NewIPSetMetadata("app:new-test-namespace", ipsets.KeyValueLabelOfNamespace),
	}
	dp.EXPECT().AddToLists(setsToAddNamespaceToNew, setsToAddNamespaceTo[:1]).Return(nil).Times(1)

	// Call into update NS
	updateNamespace(t, f, oldNsObj, newNsObj)

	// Cache and state validation section
	testCases := []expectedNsValues{
		{1, 0},
	}
	checkNsTestResult("TestUpdateNamespace", f, testCases)

	if _, exists := f.nsController.npmNamespaceCache.NsMap[newNsObj.Name]; !exists {
		t.Errorf("TestUpdateNamespace failed @ npMgr.nsMap check")
	}

	if !reflect.DeepEqual(
		newNsObj.Labels,
		f.nsController.npmNamespaceCache.NsMap[oldNsObj.Name].LabelsMap,
	) {
		t.Fatalf("TestUpdateNamespace failed @ npMgr.nsMap labelMap check")
	}
}

func TestAddNamespaceLabel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dp := dpmocks.NewMockGenericDataplane(ctrl)
	f := newNsFixture(t, dp)

	oldNsObj := newNameSpace(
		"test-namespace",
		"0",
		map[string]string{
			"app": "test-namespace",
		},
	)
	newNsObj := newNameSpace(
		"test-namespace",
		"1",
		map[string]string{
			"app":    "new-test-namespace",
			"update": "true",
		},
	)
	f.nsLister = append(f.nsLister, oldNsObj)
	f.kubeobjects = append(f.kubeobjects, oldNsObj)

	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newNsController(stopCh)

	// DPMock expect section
	setsToAddNamespaceTo := []*ipsets.IPSetMetadata{
		ipsets.NewIPSetMetadata("test-namespace", ipsets.Namespace),
		kubeAllNamespaces,
		ipsets.NewIPSetMetadata("app", ipsets.KeyLabelOfNamespace),
		ipsets.NewIPSetMetadata("app:test-namespace", ipsets.KeyValueLabelOfNamespace),
	}

	dp.EXPECT().AddToLists(setsToAddNamespaceTo[1:], setsToAddNamespaceTo[:1]).Return(nil).Times(1)
	dp.EXPECT().ApplyDataPlane().Return(nil).Times(2)
	dp.EXPECT().RemoveFromList(setsToAddNamespaceTo[3], setsToAddNamespaceTo[:1]).Return(nil).Times(1)

	setsToAddNamespaceToNew := []*ipsets.IPSetMetadata{
		ipsets.NewIPSetMetadata("app:new-test-namespace", ipsets.KeyValueLabelOfNamespace),
		ipsets.NewIPSetMetadata("update", ipsets.KeyLabelOfNamespace),
		ipsets.NewIPSetMetadata("update:true", ipsets.KeyValueLabelOfNamespace),
	}
	for i := 0; i < len(setsToAddNamespaceToNew); i++ {
		dp.EXPECT().AddToLists([]*ipsets.IPSetMetadata{setsToAddNamespaceToNew[i]}, setsToAddNamespaceTo[:1]).Return(nil).Times(1)
	}

	// Call into update NS
	updateNamespace(t, f, oldNsObj, newNsObj)

	testCases := []expectedNsValues{
		{1, 0},
	}
	checkNsTestResult("TestAddNamespaceLabel", f, testCases)

	if _, exists := f.nsController.npmNamespaceCache.NsMap[newNsObj.Name]; !exists {
		t.Errorf("TestAddNamespaceLabel failed @ nsMap check")
	}

	if !reflect.DeepEqual(
		newNsObj.Labels,
		f.nsController.npmNamespaceCache.NsMap[oldNsObj.Name].LabelsMap,
	) {
		t.Fatalf("TestAddNamespaceLabel failed @ nsMap labelMap check")
	}
}

func TestAddNamespaceLabelSameRv(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dp := dpmocks.NewMockGenericDataplane(ctrl)
	f := newNsFixture(t, dp)

	oldNsObj := newNameSpace(
		"test-namespace",
		"0",
		map[string]string{
			"app": "test-namespace",
		},
	)

	newNsObj := newNameSpace(
		"test-namespace",
		"0",
		map[string]string{
			"app":    "new-test-namespace",
			"update": "true",
		},
	)
	f.nsLister = append(f.nsLister, oldNsObj)
	f.kubeobjects = append(f.kubeobjects, oldNsObj)

	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newNsController(stopCh)

	// DPMock expect section
	setsToAddNamespaceTo := []*ipsets.IPSetMetadata{
		ipsets.NewIPSetMetadata("test-namespace", ipsets.Namespace),
		kubeAllNamespaces,
		ipsets.NewIPSetMetadata("app", ipsets.KeyLabelOfNamespace),
		ipsets.NewIPSetMetadata("app:test-namespace", ipsets.KeyValueLabelOfNamespace),
	}

	dp.EXPECT().AddToLists(setsToAddNamespaceTo[1:], setsToAddNamespaceTo[:1]).Return(nil).Times(1)
	dp.EXPECT().ApplyDataPlane().Return(nil).Times(1)

	updateNamespace(t, f, oldNsObj, newNsObj)

	testCases := []expectedNsValues{
		{1, 0},
	}
	checkNsTestResult("TestAddNamespaceLabelSameRv", f, testCases)

	if _, exists := f.nsController.npmNamespaceCache.NsMap[newNsObj.Name]; !exists {
		t.Errorf("TestAddNamespaceLabelSameRv failed @ nsMap check")
	}

	if !reflect.DeepEqual(
		oldNsObj.Labels,
		f.nsController.npmNamespaceCache.NsMap[oldNsObj.Name].LabelsMap,
	) {
		t.Fatalf("TestAddNamespaceLabelSameRv failed @ nsMap labelMap check")
	}
}

func TestDeleteandUpdateNamespaceLabel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dp := dpmocks.NewMockGenericDataplane(ctrl)
	f := newNsFixture(t, dp)

	oldNsObj := newNameSpace(
		"test-namespace",
		"0",
		map[string]string{
			"app":    "old-test-namespace",
			"update": "true",
			"group":  "test",
		},
	)

	newNsObj := newNameSpace(
		"test-namespace",
		"1",
		map[string]string{
			"app":    "old-test-namespace",
			"update": "false",
		},
	)
	f.nsLister = append(f.nsLister, oldNsObj)
	f.kubeobjects = append(f.kubeobjects, oldNsObj)

	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newNsController(stopCh)

	// DPMock expect section
	setsToAddNamespaceTo := []*ipsets.IPSetMetadata{
		ipsets.NewIPSetMetadata("test-namespace", ipsets.Namespace),
		kubeAllNamespaces,
		ipsets.NewIPSetMetadata("app", ipsets.KeyLabelOfNamespace),
		ipsets.NewIPSetMetadata("app:old-test-namespace", ipsets.KeyValueLabelOfNamespace),
		ipsets.NewIPSetMetadata("update", ipsets.KeyLabelOfNamespace),
		ipsets.NewIPSetMetadata("update:true", ipsets.KeyValueLabelOfNamespace),
		ipsets.NewIPSetMetadata("group", ipsets.KeyLabelOfNamespace),
		ipsets.NewIPSetMetadata("group:test", ipsets.KeyValueLabelOfNamespace),
	}

	// Sometimes this UT fails because the order in which slice is created is not deterministic.
	// and reflect.deepequal returns false if the order of slice is not equal.
	// But we have multiple checks in following code which validate the desired behavior so using gomock.Any
	// makes no difference
	dp.EXPECT().AddToLists(gomock.Any(), setsToAddNamespaceTo[:1]).Return(nil).Times(1)
	dp.EXPECT().ApplyDataPlane().Return(nil).Times(2)
	setsToAddNamespaceToNew := []*ipsets.IPSetMetadata{
		ipsets.NewIPSetMetadata("update:false", ipsets.KeyValueLabelOfNamespace),
	}

	// Remove calls
	for i := 5; i < len(setsToAddNamespaceTo); i++ {
		dp.EXPECT().RemoveFromList(setsToAddNamespaceTo[i], setsToAddNamespaceTo[:1]).Return(nil).Times(1)
	}
	// Add calls
	dp.EXPECT().AddToLists(setsToAddNamespaceToNew, setsToAddNamespaceTo[:1]).Return(nil).Times(1)

	updateNamespace(t, f, oldNsObj, newNsObj)

	testCases := []expectedNsValues{
		{1, 0},
	}
	checkNsTestResult("TestDeleteandUpdateNamespaceLabel", f, testCases)

	if _, exists := f.nsController.npmNamespaceCache.NsMap[newNsObj.Name]; !exists {
		t.Errorf("TestDeleteandUpdateNamespaceLabel failed @ nsMap check")
	}

	if !reflect.DeepEqual(
		newNsObj.Labels,
		f.nsController.npmNamespaceCache.NsMap[oldNsObj.Name].LabelsMap,
	) {
		t.Fatalf("TestDeleteandUpdateNamespaceLabel failed @ nsMap labelMap check")
	}
}

// TestNewNameSpaceUpdate will test the case where the key is same but the object is different.
// this happens when NSA delete event is missed and deleted from NPMLocalCache,
// but NSA gets added again. This will result in an update event with old and new with different UUIDs
func TestNewNameSpaceUpdate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dp := dpmocks.NewMockGenericDataplane(ctrl)
	f := newNsFixture(t, dp)

	oldNsObj := newNameSpace(
		"test-namespace",
		"10",
		map[string]string{
			"app":    "old-test-namespace",
			"update": "true",
			"group":  "test",
		},
	)
	oldNsObj.SetUID("test1")

	newNsObj := newNameSpace(
		"test-namespace",
		"11",
		map[string]string{
			"app":    "old-test-namespace",
			"update": "false",
		},
	)
	f.nsLister = append(f.nsLister, oldNsObj)
	f.kubeobjects = append(f.kubeobjects, oldNsObj)

	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newNsController(stopCh)
	newNsObj.SetUID("test2")

	// DPMock expect section
	setsToAddNamespaceTo := []*ipsets.IPSetMetadata{
		ipsets.NewIPSetMetadata("test-namespace", ipsets.Namespace),
		kubeAllNamespaces,
		ipsets.NewIPSetMetadata("app", ipsets.KeyLabelOfNamespace),
		ipsets.NewIPSetMetadata("app:old-test-namespace", ipsets.KeyValueLabelOfNamespace),
		ipsets.NewIPSetMetadata("update", ipsets.KeyLabelOfNamespace),
		ipsets.NewIPSetMetadata("update:true", ipsets.KeyValueLabelOfNamespace),
		ipsets.NewIPSetMetadata("group", ipsets.KeyLabelOfNamespace),
		ipsets.NewIPSetMetadata("group:test", ipsets.KeyValueLabelOfNamespace),
	}

	// Sometimes this UT fails because the order in which slice is created is not deterministic.
	// and reflect.deepequal returns false if the order of slice is not equal.
	// But we have multiple checks in following code which validate the desired behavior so using gomock.Any
	// makes no difference
	dp.EXPECT().AddToLists(gomock.Any(), setsToAddNamespaceTo[:1]).Return(nil).Times(1)
	dp.EXPECT().ApplyDataPlane().Return(nil).Times(2)

	setsToAddNamespaceToNew := []*ipsets.IPSetMetadata{
		ipsets.NewIPSetMetadata("update:false", ipsets.KeyValueLabelOfNamespace),
	}

	// Remove calls
	for i := 5; i < len(setsToAddNamespaceTo); i++ {
		dp.EXPECT().RemoveFromList(setsToAddNamespaceTo[i], setsToAddNamespaceTo[:1]).Return(nil).Times(1)
	}
	// Add calls
	dp.EXPECT().AddToLists(setsToAddNamespaceToNew, setsToAddNamespaceTo[:1]).Return(nil).Times(1)

	updateNamespace(t, f, oldNsObj, newNsObj)

	testCases := []expectedNsValues{
		{1, 0},
	}
	checkNsTestResult("TestDeleteandUpdateNamespaceLabel", f, testCases)

	if _, exists := f.nsController.npmNamespaceCache.NsMap[newNsObj.Name]; !exists {
		t.Errorf("TestDeleteandUpdateNamespaceLabel failed @ nsMap check")
	}

	if !reflect.DeepEqual(
		newNsObj.Labels,
		f.nsController.npmNamespaceCache.NsMap[oldNsObj.Name].LabelsMap,
	) {
		t.Fatalf("TestDeleteandUpdateNamespaceLabel failed @ nsMap labelMap check")
	}
}

func TestDeleteNamespace(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dp := dpmocks.NewMockGenericDataplane(ctrl)
	f := newNsFixture(t, dp)

	nsObj := newNameSpace(
		"test-namespace",
		"0",
		map[string]string{
			"app": "test-namespace",
		},
	)
	f.nsLister = append(f.nsLister, nsObj)
	f.kubeobjects = append(f.kubeobjects, nsObj)

	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newNsController(stopCh)

	// DPMock expect section
	setsToAddNamespaceTo := []*ipsets.IPSetMetadata{
		ipsets.NewIPSetMetadata("test-namespace", ipsets.Namespace),
		kubeAllNamespaces,
		ipsets.NewIPSetMetadata("app", ipsets.KeyLabelOfNamespace),
		ipsets.NewIPSetMetadata("app:test-namespace", ipsets.KeyValueLabelOfNamespace),
	}

	dp.EXPECT().AddToLists(setsToAddNamespaceTo[1:], setsToAddNamespaceTo[:1]).Return(nil).Times(1)
	dp.EXPECT().ApplyDataPlane().Return(nil).Times(2)

	// Remove calls
	for i := 1; i < len(setsToAddNamespaceTo); i++ {
		dp.EXPECT().RemoveFromList(setsToAddNamespaceTo[i], setsToAddNamespaceTo[:1]).Return(nil).Times(1)
	}
	dp.EXPECT().DeleteIPSet(setsToAddNamespaceTo[0]).Return().Times(1)

	deleteNamespace(t, f, nsObj, DeletedFinalStateknownObject)

	testCases := []expectedNsValues{
		{0, 0},
	}
	checkNsTestResult("TestDeleteNamespace", f, testCases)

	if _, exists := f.nsController.npmNamespaceCache.NsMap[nsObj.Name]; exists {
		t.Errorf("TestDeleteNamespace failed @ nsMap check")
	}
}

func TestDeleteNamespaceWithTombstone(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dp := dpmocks.NewMockGenericDataplane(ctrl)
	f := newNsFixture(t, dp)
	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newNsController(stopCh)

	nsObj := newNameSpace(
		"test-namespace",
		"0",
		map[string]string{
			"app": "test-namespace",
		},
	)
	tombstone := cache.DeletedFinalStateUnknown{
		Key: nsObj.Name,
		Obj: nsObj,
	}

	f.nsController.deleteNamespace(tombstone)

	testCases := []expectedNsValues{
		{0, 1},
	}
	checkNsTestResult("TestDeleteNamespaceWithTombstone", f, testCases)
}

func TestDeleteNamespaceWithTombstoneAfterAddingNameSpace(t *testing.T) {
	nsObj := newNameSpace(
		"test-namespace",
		"0",
		map[string]string{
			"app": "test-namespace",
		},
	)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dp := dpmocks.NewMockGenericDataplane(ctrl)
	f := newNsFixture(t, dp)
	f.nsLister = append(f.nsLister, nsObj)
	f.kubeobjects = append(f.kubeobjects, nsObj)
	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newNsController(stopCh)

	// DPMock expect section
	setsToAddNamespaceTo := []*ipsets.IPSetMetadata{
		ipsets.NewIPSetMetadata("test-namespace", ipsets.Namespace),
		kubeAllNamespaces,
		ipsets.NewIPSetMetadata("app", ipsets.KeyLabelOfNamespace),
		ipsets.NewIPSetMetadata("app:test-namespace", ipsets.KeyValueLabelOfNamespace),
	}

	dp.EXPECT().AddToLists(setsToAddNamespaceTo[1:], setsToAddNamespaceTo[:1]).Return(nil).Times(1)
	dp.EXPECT().ApplyDataPlane().Return(nil).Times(2)

	// Remove calls
	for i := 1; i < len(setsToAddNamespaceTo); i++ {
		dp.EXPECT().RemoveFromList(setsToAddNamespaceTo[i], setsToAddNamespaceTo[:1]).Return(nil).Times(1)
	}
	dp.EXPECT().DeleteIPSet(setsToAddNamespaceTo[0]).Return().Times(1)

	deleteNamespace(t, f, nsObj, DeletedFinalStateUnknownObject)
	testCases := []expectedNsValues{
		{0, 0},
	}
	checkNsTestResult("TestDeleteNamespaceWithTombstoneAfterAddingNameSpace", f, testCases)
}

func TestIsSystemNs(t *testing.T) {
	nsObj := newNameSpace("kube-system", "0", map[string]string{"test": "new"})

	if !isSystemNs(nsObj) {
		t.Errorf("TestIsSystemNs failed @ nsObj isSystemNs check")
	}
}

func checkNsTestResult(testName string, f *nameSpaceFixture, testCases []expectedNsValues) {
	for _, test := range testCases {
		if got := len(f.nsController.npmNamespaceCache.NsMap); got != test.expectedLenOfNsMap {
			f.t.Errorf("Test: %s, NsMap length = %d, want %d. Map: %+v",
				testName, got, test.expectedLenOfNsMap, f.nsController.npmNamespaceCache.NsMap)
		}
		if got := f.nsController.workqueue.Len(); got != test.expectedLenOfWorkQueue {
			f.t.Errorf("Test: %s, Workqueue length = %d, want %d", testName, got, test.expectedLenOfWorkQueue)
		}
	}
}

func TestNSMapMarshalJSON(t *testing.T) {
	npmNSCache := &NpmNamespaceCache{NsMap: make(map[string]*Namespace)}
	nsName := "ns-test"
	ns := &Namespace{
		name: nsName,
		LabelsMap: map[string]string{
			"test-key": "test-value",
		},
	}

	npmNSCache.NsMap[nsName] = ns
	nsMapRaw, err := npmNSCache.MarshalJSON()
	require.NoError(t, err)

	expect := []byte(`{"ns-test":{"LabelsMap":{"test-key":"test-value"}}}`)
	assert.ElementsMatch(t, expect, nsMapRaw)
}

func isSystemNs(nsObj *corev1.Namespace) bool {
	return nsObj.ObjectMeta.Name == util.KubeSystemFlag
}
