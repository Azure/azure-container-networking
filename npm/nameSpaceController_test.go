// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"reflect"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/npm/ipsm"
	"github.com/Azure/azure-container-networking/npm/util"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
)

var (
	alwaysReady        = func() bool { return true }
	noResyncPeriodFunc = func() time.Duration { return 0 }
)

type expectedNsValues struct {
	expectedLenOfPodMap    int
	expectedLenOfNsMap     int
	expectedLenOfWorkQueue int
}

type nameSpaceFixture struct {
	t *testing.T

	kubeclient *k8sfake.Clientset
	// Objects to put in the store.
	nsLister []*corev1.Namespace
	// Actions expected to happen on the client.
	kubeactions []core.Action
	// Objects from here preloaded into NewSimpleFake.
	kubeobjects []runtime.Object

	// (TODO) will remove npMgr if possible
	npMgr        *NetworkPolicyManager
	ipsMgr       *ipsm.IpsetManager
	nsController *nameSpaceController
	kubeInformer kubeinformers.SharedInformerFactory
}

func newNsFixture(t *testing.T) *nameSpaceFixture {
	f := &nameSpaceFixture{
		t:           t,
		nsLister:    []*corev1.Namespace{},
		kubeobjects: []runtime.Object{},
		npMgr:       newNPMgr(t),
		ipsMgr:      ipsm.NewIpsetManager(),
	}
	return f
}

func (f *nameSpaceFixture) newNsController() {
	f.kubeclient = k8sfake.NewSimpleClientset(f.kubeobjects...)
	f.kubeInformer = kubeinformers.NewSharedInformerFactory(f.kubeclient, noResyncPeriodFunc())

	f.nsController = NewNameSpaceController(f.kubeInformer.Core().V1().Namespaces(), f.kubeclient, f.npMgr)
	f.nsController.nameSpaceListerSynced = alwaysReady

	for _, ns := range f.nsLister {
		f.kubeInformer.Core().V1().Namespaces().Informer().GetIndexer().Add(ns)
	}
}

func (f *nameSpaceFixture) ipSetSave(ipsetConfigFile string) {
	//  call /sbin/ipset save -file /var/log/ipset-test.conf
	f.t.Logf("Start storing ipset to %s", ipsetConfigFile)
	if err := f.ipsMgr.Save(ipsetConfigFile); err != nil {
		f.t.Errorf("TestAddPod failed @ ipsMgr.Save")
	}
}

func (f *nameSpaceFixture) ipSetRestore(ipsetConfigFile string) {
	//  call /sbin/ipset restore -file /var/log/ipset-test.conf
	f.t.Logf("Start re-storing ipset to %s", ipsetConfigFile)
	if err := f.ipsMgr.Restore(ipsetConfigFile); err != nil {
		f.t.Errorf("TestAddPod failed @ ipsMgr.Restore")
	}
}
func newNPMgr(t *testing.T) *NetworkPolicyManager {
	npMgr := &NetworkPolicyManager{
		NsMap:            make(map[string]*Namespace),
		PodMap:           make(map[string]*NpmPod),
		RawNpMap:         make(map[string]*networkingv1.NetworkPolicy),
		ProcessedNpMap:   make(map[string]*networkingv1.NetworkPolicy),
		TelemetryEnabled: false,
	}

	// This initialization important as without this NPM will panic
	allNs, _ := newNs(util.KubeAllNamespacesFlag)
	npMgr.NsMap[util.KubeAllNamespacesFlag] = allNs
	return npMgr
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
	f.nsLister = append(f.nsLister, nsObj)
	f.kubeobjects = append(f.kubeobjects, nsObj)

	f.newNsController()
	stopCh := make(chan struct{})
	defer close(stopCh)
	f.kubeInformer.Start(stopCh)

	t.Logf("Calling add namespace event")
	f.nsController.addNamespace(nsObj)
	f.nsController.processNextWorkItem()
}

func updateNamespace(t *testing.T, f *nameSpaceFixture, oldNsObj, newNsObj *corev1.Namespace) {
	addNamespace(t, f, oldNsObj)
	t.Logf("Complete add namespace event")

	t.Logf("Updating kubeinformer namespace object")
	f.kubeInformer.Core().V1().Namespaces().Informer().GetIndexer().Update(newNsObj)

	t.Logf("Calling update namespace event")
	f.nsController.updateNamespace(oldNsObj, newNsObj)
	f.nsController.processNextWorkItem()
}

func deleteNamespace(t *testing.T, f *nameSpaceFixture, nsObj *corev1.Namespace) {
	addNamespace(t, f, nsObj)
	t.Logf("Complete add namespace event")

	t.Logf("Updating kubeinformer namespace object")
	f.kubeInformer.Core().V1().Namespaces().Informer().GetIndexer().Delete(nsObj)

	t.Logf("Calling delete namespace event")
	f.nsController.deleteNamespace(nsObj)
	f.nsController.processNextWorkItem()
}

func TestNewNs(t *testing.T) {
	if _, err := newNs("test"); err != nil {
		t.Errorf("TestnewNs failed @ newNs")
	}
}

func TestAllNsList(t *testing.T) {
	npMgr := &NetworkPolicyManager{}

	ipsMgr := ipsm.NewIpsetManager()
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestAllNsList failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestAllNsList failed @ ipsMgr.Restore")
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
	f := newNsFixture(t)
	f.ipSetSave(util.IpsetTestConfigFile)
	defer f.ipSetRestore(util.IpsetTestConfigFile)

	nsObj := newNameSpace(
		"test-namespace",
		"0",
		map[string]string{
			"app": "test-namespace",
		},
	)

	addNamespace(t, f, nsObj)

	testCases := []expectedNsValues{
		{0, 2, 0},
	}
	checkNsTestResult("TestAddNamespace", f, testCases)

	if _, exists := f.npMgr.NsMap[util.GetNSNameWithPrefix(nsObj.Name)]; exists {
		t.Errorf("TestAddNamespace failed @ npMgr.nsMap check")
	}
}

func TestUpdateNamespace(t *testing.T) {
	f := newNsFixture(t)
	f.ipSetSave(util.IpsetTestConfigFile)
	defer f.ipSetRestore(util.IpsetTestConfigFile)

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
	updateNamespace(t, f, oldNsObj, newNsObj)

	testCases := []expectedNsValues{
		{0, 2, 0},
	}
	checkNsTestResult("TestUpdateNamespace", f, testCases)

	if _, exists := f.npMgr.NsMap[util.GetNSNameWithPrefix(newNsObj.Name)]; exists {
		t.Errorf("TestUpdateNamespace failed @ npMgr.nsMap check")
	}

	if !reflect.DeepEqual(
		newNsObj.Labels,
		f.npMgr.NsMap[util.GetNSNameWithPrefix(oldNsObj.Name)].LabelsMap,
	) {
		t.Fatalf("TestUpdateNamespace failed @ npMgr.nsMap labelMap check")
	}
}

func TestAddNamespaceLabel(t *testing.T) {
	f := newNsFixture(t)
	f.ipSetSave(util.IpsetTestConfigFile)
	defer f.ipSetRestore(util.IpsetTestConfigFile)

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
	updateNamespace(t, f, oldNsObj, newNsObj)

	testCases := []expectedNsValues{
		{0, 2, 0},
	}
	checkNsTestResult("TestAddNamespaceLabel", f, testCases)

	if _, exists := f.npMgr.NsMap[util.GetNSNameWithPrefix(newNsObj.Name)]; exists {
		t.Errorf("TestAddNamespaceLabel failed @ npMgr.nsMap check")
	}

	if !reflect.DeepEqual(
		newNsObj.Labels,
		f.npMgr.NsMap[util.GetNSNameWithPrefix(oldNsObj.Name)].LabelsMap,
	) {
		t.Fatalf("TestAddNamespaceLabel failed @ npMgr.nsMap labelMap check")
	}
}

func TestAddNamespaceLabelSameRv(t *testing.T) {
	f := newNsFixture(t)
	f.ipSetSave(util.IpsetTestConfigFile)
	defer f.ipSetRestore(util.IpsetTestConfigFile)

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
	updateNamespace(t, f, oldNsObj, newNsObj)

	testCases := []expectedNsValues{
		{0, 2, 0},
	}
	checkNsTestResult("TestAddNamespaceLabelSameRv", f, testCases)

	if _, exists := f.npMgr.NsMap[util.GetNSNameWithPrefix(newNsObj.Name)]; exists {
		t.Errorf("TestAddNamespaceLabelSameRv failed @ npMgr.nsMap check")
	}

	if !reflect.DeepEqual(
		oldNsObj.Labels,
		f.npMgr.NsMap[util.GetNSNameWithPrefix(oldNsObj.Name)].LabelsMap,
	) {
		t.Fatalf("TestAddNamespaceLabelSameRv failed @ npMgr.nsMap labelMap check")
	}
}

func TestDeleteandUpdateNamespaceLabel(t *testing.T) {
	f := newNsFixture(t)
	f.ipSetSave(util.IpsetTestConfigFile)
	defer f.ipSetRestore(util.IpsetTestConfigFile)

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
	updateNamespace(t, f, oldNsObj, newNsObj)

	testCases := []expectedNsValues{
		{0, 2, 0},
	}
	checkNsTestResult("TestDeleteandUpdateNamespaceLabel", f, testCases)

	if _, exists := f.npMgr.NsMap[util.GetNSNameWithPrefix(newNsObj.Name)]; exists {
		t.Errorf("TestDeleteandUpdateNamespaceLabel failed @ npMgr.nsMap check")
	}

	if !reflect.DeepEqual(
		oldNsObj.Labels,
		f.npMgr.NsMap[util.GetNSNameWithPrefix(oldNsObj.Name)].LabelsMap,
	) {
		t.Fatalf("TestDeleteandUpdateNamespaceLabel failed @ npMgr.nsMap labelMap check")
	}
}

func TestDeleteNamespace(t *testing.T) {
	f := newNsFixture(t)
	f.ipSetSave(util.IpsetTestConfigFile)
	defer f.ipSetRestore(util.IpsetTestConfigFile)

	nsObj := newNameSpace(
		"test-namespace",
		"0",
		map[string]string{
			"app": "test-namespace",
		},
	)
	deleteNamespace(t, f, nsObj)

	testCases := []expectedNsValues{
		{0, 1, 0},
	}
	checkNsTestResult("TestDeleteNamespace", f, testCases)

	if _, exists := f.npMgr.NsMap[util.GetNSNameWithPrefix(nsObj.Name)]; exists {
		t.Errorf("TestDeleteNamespace failed @ npMgr.nsMap check")
	}
}

func checkNsTestResult(testName string, f *nameSpaceFixture, testCases []expectedNsValues) {
	for _, test := range testCases {
		if got := len(f.npMgr.PodMap); got != test.expectedLenOfPodMap {
			f.t.Errorf("PodMap length = %d, want %d", got, test.expectedLenOfPodMap)
		}
		if got := len(f.npMgr.NsMap); got != test.expectedLenOfNsMap {
			f.t.Errorf("npMgr length = %d, want %d", got, test.expectedLenOfNsMap)
		}
		if got := f.nsController.workqueue.Len(); got != test.expectedLenOfWorkQueue {
			f.t.Errorf("Workqueue length = %d, want %d", got, test.expectedLenOfWorkQueue)
		}
	}
}
