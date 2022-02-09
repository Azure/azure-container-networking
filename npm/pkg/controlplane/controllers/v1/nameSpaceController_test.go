// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package controllers

import (
	"flag"
	"reflect"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/npm/ipsm"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/metrics/promutil"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/exec"
)

var (
	alwaysReady        = func() bool { return true }
	noResyncPeriodFunc = func() time.Duration { return 0 }

	// turn on with: go run . --lists-unsupported
	debugWhenListsUnsupported = flag.Bool("lists-unsupported", false, "Set to true for correct prometheus metrics if ipset lists are unsupported.")
)

type expectedNsValues struct {
	expectedLenOfPodMap    int
	expectedLenOfNsMap     int
	expectedLenOfWorkQueue int
	nsPromVals
}

type nsPromVals struct {
	expectedAddExecCount    int
	expectedUpdateExecCount int
	expectedDeleteExecCount int
}

func (p *nsPromVals) testPrometheusMetrics(t *testing.T) {
	addExecCount, err := metrics.GetControllerNamespaceExecCount(metrics.CreateOp, false)
	promutil.NotifyIfErrors(t, err)
	require.Equal(t, p.expectedAddExecCount, addExecCount, "Count for add execution time didn't register correctly in Prometheus")

	addErrorExecCount, err := metrics.GetControllerNamespaceExecCount(metrics.CreateOp, true)
	promutil.NotifyIfErrors(t, err)
	require.Equal(t, 0, addErrorExecCount, "Count for add error execution time should be 0")

	updateExecCount, err := metrics.GetControllerNamespaceExecCount(metrics.UpdateOp, flipBool(false, *debugWhenListsUnsupported))
	promutil.NotifyIfErrors(t, err)
	require.Equal(t, p.expectedUpdateExecCount, updateExecCount, "Count for update execution time didn't register correctly in Prometheus")

	updateErrorExecCount, err := metrics.GetControllerNamespaceExecCount(metrics.UpdateOp, flipBool(true, *debugWhenListsUnsupported))
	promutil.NotifyIfErrors(t, err)
	require.Equal(t, 0, updateErrorExecCount, "Count for update error execution time should be 0")

	deleteExecCount, err := metrics.GetControllerNamespaceExecCount(metrics.DeleteOp, flipBool(false, *debugWhenListsUnsupported))
	promutil.NotifyIfErrors(t, err)
	require.Equal(t, p.expectedDeleteExecCount, deleteExecCount, "Count for delete execution time didn't register correctly in Prometheus")

	deleteErrorExecCount, err := metrics.GetControllerNamespaceExecCount(metrics.DeleteOp, flipBool(true, *debugWhenListsUnsupported))
	promutil.NotifyIfErrors(t, err)
	require.Equal(t, 0, deleteErrorExecCount, "Count for delete error execution time should be 0")
}

func flipBool(b, shouldFlip bool) bool {
	if shouldFlip {
		return !b
	}
	return b
}

type nameSpaceFixture struct {
	t *testing.T

	nsLister []*corev1.Namespace
	// Actions expected to happen on the client.
	kubeactions []core.Action
	// Objects from here preloaded into NewSimpleFake.
	kubeobjects []runtime.Object

	ipsMgr       *ipsm.IpsetManager
	nsController *NamespaceController
	kubeInformer kubeinformers.SharedInformerFactory
}

func newNsFixture(t *testing.T, utilexec exec.Interface) *nameSpaceFixture {
	f := &nameSpaceFixture{
		t:           t,
		nsLister:    []*corev1.Namespace{},
		kubeobjects: []runtime.Object{},
		ipsMgr:      ipsm.NewIpsetManager(utilexec),
	}
	return f
}

func (f *nameSpaceFixture) newNsController(stopCh chan struct{}) {
	kubeclient := k8sfake.NewSimpleClientset(f.kubeobjects...)
	f.kubeInformer = kubeinformers.NewSharedInformerFactory(kubeclient, noResyncPeriodFunc())

	npmNamespaceCache := &NpmNamespaceCache{NsMap: make(map[string]*Namespace)}
	f.nsController = NewNameSpaceController(
		f.kubeInformer.Core().V1().Namespaces(), f.ipsMgr, npmNamespaceCache)

	for _, ns := range f.nsLister {
		f.kubeInformer.Core().V1().Namespaces().Informer().GetIndexer().Add(ns)
	}

	metrics.ReinitializeAll()

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
	f.kubeInformer.Core().V1().Namespaces().Informer().GetIndexer().Update(newNsObj)

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
	f.kubeInformer.Core().V1().Namespaces().Informer().GetIndexer().Delete(nsObj)

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
	fexec := exec.New()
	f := newNsFixture(t, fexec)

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

	addNamespace(t, f, nsObj)

	// already exists (will be a no-op)
	addNamespace(t, f, nsObj)

	testCases := []expectedNsValues{
		{0, 1, 0, nsPromVals{1, 0, 0}},
	}
	checkNsTestResult("TestAddNamespace", f, testCases)

	if _, exists := f.nsController.npmNamespaceCache.NsMap[util.GetNSNameWithPrefix(nsObj.Name)]; !exists {
		t.Errorf("TestAddNamespace failed @ npMgr.nsMap check")
	}
}

func TestUpdateNamespace(t *testing.T) {
	fexec := exec.New()
	f := newNsFixture(t, fexec)

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
	updateNamespace(t, f, oldNsObj, newNsObj)

	testCases := []expectedNsValues{
		{0, 1, 0, nsPromVals{1, 1, 0}},
	}
	checkNsTestResult("TestUpdateNamespace", f, testCases)

	if _, exists := f.nsController.npmNamespaceCache.NsMap[util.GetNSNameWithPrefix(newNsObj.Name)]; !exists {
		t.Errorf("TestUpdateNamespace failed @ npMgr.nsMap check")
	}

	if !reflect.DeepEqual(
		newNsObj.Labels,
		f.nsController.npmNamespaceCache.NsMap[util.GetNSNameWithPrefix(oldNsObj.Name)].LabelsMap,
	) {
		t.Fatalf("TestUpdateNamespace failed @ npMgr.nsMap labelMap check")
	}
}

func TestAddNamespaceLabel(t *testing.T) {
	fexec := exec.New()
	f := newNsFixture(t, fexec)

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
	updateNamespace(t, f, oldNsObj, newNsObj)

	testCases := []expectedNsValues{
		{0, 1, 0, nsPromVals{1, 1, 0}},
	}
	checkNsTestResult("TestAddNamespaceLabel", f, testCases)

	if _, exists := f.nsController.npmNamespaceCache.NsMap[util.GetNSNameWithPrefix(newNsObj.Name)]; !exists {
		t.Errorf("TestAddNamespaceLabel failed @ nsMap check")
	}

	if !reflect.DeepEqual(
		newNsObj.Labels,
		f.nsController.npmNamespaceCache.NsMap[util.GetNSNameWithPrefix(oldNsObj.Name)].LabelsMap,
	) {
		t.Fatalf("TestAddNamespaceLabel failed @ nsMap labelMap check")
	}
}

func TestAddNamespaceLabelSameRv(t *testing.T) {
	fexec := exec.New()
	f := newNsFixture(t, fexec)

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
	updateNamespace(t, f, oldNsObj, newNsObj)

	// no need to reconcile because only the rv changes, so we don't see a prometheus update exec count
	testCases := []expectedNsValues{
		{0, 1, 0, nsPromVals{1, 0, 0}},
	}
	checkNsTestResult("TestAddNamespaceLabelSameRv", f, testCases)

	if _, exists := f.nsController.npmNamespaceCache.NsMap[util.GetNSNameWithPrefix(newNsObj.Name)]; !exists {
		t.Errorf("TestAddNamespaceLabelSameRv failed @ nsMap check")
	}

	if !reflect.DeepEqual(
		oldNsObj.Labels,
		f.nsController.npmNamespaceCache.NsMap[util.GetNSNameWithPrefix(oldNsObj.Name)].LabelsMap,
	) {
		t.Fatalf("TestAddNamespaceLabelSameRv failed @ nsMap labelMap check")
	}
}

func TestDeleteandUpdateNamespaceLabel(t *testing.T) {
	fexec := exec.New()
	f := newNsFixture(t, fexec)

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
	updateNamespace(t, f, oldNsObj, newNsObj)

	testCases := []expectedNsValues{
		{0, 1, 0, nsPromVals{1, 1, 0}},
	}
	checkNsTestResult("TestDeleteandUpdateNamespaceLabel", f, testCases)

	if _, exists := f.nsController.npmNamespaceCache.NsMap[util.GetNSNameWithPrefix(newNsObj.Name)]; !exists {
		t.Errorf("TestDeleteandUpdateNamespaceLabel failed @ nsMap check")
	}

	if !reflect.DeepEqual(
		newNsObj.Labels,
		f.nsController.npmNamespaceCache.NsMap[util.GetNSNameWithPrefix(oldNsObj.Name)].LabelsMap,
	) {
		t.Fatalf("TestDeleteandUpdateNamespaceLabel failed @ nsMap labelMap check")
	}
}

// TestNewNameSpaceUpdate will test the case where the key is same but the object is different.
// this happens when NSA delete event is missed and deleted from NPMLocalCache,
// but NSA gets added again. This will result in an update event with old and new with different UUIDs
func TestNewNameSpaceUpdate(t *testing.T) {
	fexec := exec.New()
	f := newNsFixture(t, fexec)

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
		"9",
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
	updateNamespace(t, f, oldNsObj, newNsObj)

	testCases := []expectedNsValues{
		{0, 1, 0, nsPromVals{1, 1, 0}},
	}
	checkNsTestResult("TestNewNameSpaceUpdate", f, testCases)

	if _, exists := f.nsController.npmNamespaceCache.NsMap[util.GetNSNameWithPrefix(newNsObj.Name)]; !exists {
		t.Errorf("TestNewNameSpaceUpdate failed @ nsMap check")
	}

	if !reflect.DeepEqual(
		newNsObj.Labels,
		f.nsController.npmNamespaceCache.NsMap[util.GetNSNameWithPrefix(oldNsObj.Name)].LabelsMap,
	) {
		t.Fatalf("TestNewNameSpaceUpdate failed @ nsMap labelMap check")
	}
}

func TestDeleteNamespace(t *testing.T) {
	fexec := exec.New()
	f := newNsFixture(t, fexec)

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
	deleteNamespace(t, f, nsObj, DeletedFinalStateknownObject)

	testCases := []expectedNsValues{
		{0, 0, 0, nsPromVals{1, 0, 1}},
	}
	checkNsTestResult("TestDeleteNamespace", f, testCases)

	if _, exists := f.nsController.npmNamespaceCache.NsMap[util.GetNSNameWithPrefix(nsObj.Name)]; exists {
		t.Errorf("TestDeleteNamespace failed @ nsMap check")
	}
}

func TestDeleteNamespaceWithTombstone(t *testing.T) {
	fexec := exec.New()
	f := newNsFixture(t, fexec)
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
	// the above function only adds to the workqueue
	testCases := []expectedNsValues{
		{0, 0, 1, nsPromVals{0, 0, 0}},
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
	fexec := exec.New()
	f := newNsFixture(t, fexec)
	f.nsLister = append(f.nsLister, nsObj)
	f.kubeobjects = append(f.kubeobjects, nsObj)
	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newNsController(stopCh)

	deleteNamespace(t, f, nsObj, DeletedFinalStateUnknownObject)
	testCases := []expectedNsValues{
		{0, 0, 0, nsPromVals{1, 0, 1}},
	}
	checkNsTestResult("TestDeleteNamespaceWithTombstoneAfterAddingNameSpace", f, testCases)
}

func TestGetNamespaceObjFromNsObj(t *testing.T) {
	ns := newNs("test-ns")
	ns.LabelsMap = map[string]string{
		"test": "new",
	}

	nsObj := ns.getNamespaceObjFromNsObj()

	if !reflect.DeepEqual(ns.LabelsMap, nsObj.ObjectMeta.Labels) {
		t.Errorf("TestGetNamespaceObjFromNsObj failed @ nsObj labels check")
	}
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
			f.t.Errorf("NsMap length = %d, want %d. Map: %+v",
				got, test.expectedLenOfNsMap, f.nsController.npmNamespaceCache.NsMap)
		}
		if got := f.nsController.workqueue.Len(); got != test.expectedLenOfWorkQueue {
			f.t.Errorf("Workqueue length = %d, want %d", got, test.expectedLenOfWorkQueue)
		}
		test.nsPromVals.testPrometheusMetrics(f.t)
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
