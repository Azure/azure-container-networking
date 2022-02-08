// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package controllers

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/Azure/azure-container-networking/npm/ipsm"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/metrics/promutil"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/exec"
)

type netPolFixture struct {
	t *testing.T

	// Objects to put in the store.
	netPolLister []*networkingv1.NetworkPolicy
	// (TODO) Actions expected to happen on the client. Will use this to check action.
	kubeactions []core.Action
	// Objects from here preloaded into NewSimpleFake.
	kubeobjects []runtime.Object

	ipsMgr           *ipsm.IpsetManager
	netPolController *NetworkPolicyController
	kubeInformer     kubeinformers.SharedInformerFactory
}

func newNetPolFixture(t *testing.T, utilexec exec.Interface) *netPolFixture {
	f := &netPolFixture{
		t:            t,
		netPolLister: []*networkingv1.NetworkPolicy{},
		kubeobjects:  []runtime.Object{},
		ipsMgr:       ipsm.NewIpsetManager(utilexec),
	}
	return f
}

func (f *netPolFixture) newNetPolController(stopCh chan struct{}) {
	kubeclient := k8sfake.NewSimpleClientset(f.kubeobjects...)
	f.kubeInformer = kubeinformers.NewSharedInformerFactory(kubeclient, noResyncPeriodFunc())

	f.netPolController = NewNetworkPolicyController(f.kubeInformer.Networking().V1().NetworkPolicies(), f.ipsMgr, util.PlaceAzureChainAfterKubeServices)

	for _, netPol := range f.netPolLister {
		f.kubeInformer.Networking().V1().NetworkPolicies().Informer().GetIndexer().Add(netPol)
	}

	metrics.ReinitializeAll()

	// Do not start informer to avoid unnecessary event triggers
	// (TODO): Leave stopCh and below commented code to enhance UTs to even check event triggers as well later if possible
	// f.kubeInformer.Start(stopCh)
}

// (TODO): make createNetPol flexible
func createNetPol() *networkingv1.NetworkPolicy {
	tcp := corev1.ProtocolTCP
	port8000 := intstr.FromInt(8000)
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "allow-ingress",
			Namespace: "test-nwpolicy",
		},
		Spec: networkingv1.NetworkPolicySpec{
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test"},
							},
						},
						{
							IPBlock: &networkingv1.IPBlock{
								CIDR: "0.0.0.0/0",
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{{
						Protocol: &tcp,
						Port:     &port8000,
					}},
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					To: []networkingv1.NetworkPolicyPeer{{
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test"},
						},
					}},
					Ports: []networkingv1.NetworkPolicyPort{{
						Protocol: &tcp,
						Port:     &intstr.IntOrString{StrVal: "8000"}, // namedPort
					}},
				},
			},
		},
	}
}

func addNetPol(t *testing.T, f *netPolFixture, netPolObj *networkingv1.NetworkPolicy) {
	// simulate "network policy" add event and add network policy object to sharedInformer cache
	f.netPolController.addNetworkPolicy(netPolObj)

	if f.netPolController.workqueue.Len() == 0 {
		return
	}

	f.netPolController.processNextWorkItem()
}

func deleteNetPol(t *testing.T, f *netPolFixture, netPolObj *networkingv1.NetworkPolicy, isDeletedFinalStateUnknownObject IsDeletedFinalStateUnknownObject) {
	addNetPol(t, f, netPolObj)
	t.Logf("Complete adding network policy event")

	// simulate network policy deletion event and delete network policy object from sharedInformer cache
	f.kubeInformer.Networking().V1().NetworkPolicies().Informer().GetIndexer().Delete(netPolObj)
	if isDeletedFinalStateUnknownObject {
		netPolKey := getKey(netPolObj, t)
		tombstone := cache.DeletedFinalStateUnknown{
			Key: netPolKey,
			Obj: netPolObj,
		}
		f.netPolController.deleteNetworkPolicy(tombstone)
	} else {
		f.netPolController.deleteNetworkPolicy(netPolObj)
	}

	if f.netPolController.workqueue.Len() == 0 {
		return
	}

	f.netPolController.processNextWorkItem()
}

func updateNetPol(t *testing.T, f *netPolFixture, oldNetPolObj, netNetPolObj *networkingv1.NetworkPolicy) {
	addNetPol(t, f, oldNetPolObj)
	t.Logf("Complete adding network policy event")

	// simulate network policy update event and update the network policy to shared informer's cache
	f.kubeInformer.Networking().V1().NetworkPolicies().Informer().GetIndexer().Update(netNetPolObj)
	f.netPolController.updateNetworkPolicy(oldNetPolObj, netNetPolObj)

	if f.netPolController.workqueue.Len() == 0 {
		return
	}

	f.netPolController.processNextWorkItem()
}

type expectedNetPolValues struct {
	expectedLenOfRawNpMap          int
	expectedLenOfWorkQueue         int
	expectedIsAzureNpmChainCreated bool
	netPolPromVals
}

type netPolPromVals struct {
	expectedNumPolicies     int
	expectedAddExecCount    int
	expectedUpdateExecCount int
	expectedDeleteExecCount int
}

// for local testing, prepend the following to your go test command: sudo -E env 'PATH=$PATH'
func (p *netPolPromVals) testPrometheusMetrics(t *testing.T) {
	numPolicies, err := metrics.GetNumPolicies()
	promutil.NotifyIfErrors(t, err)
	if numPolicies != p.expectedNumPolicies {
		require.FailNowf(t, "", "Number of policies didn't register correctly in Prometheus. Expected %d. Got %d.", p.expectedNumPolicies, numPolicies)
	}

	addExecCount, err := metrics.GetControllerPolicyExecCount(metrics.CreateOp, false)
	promutil.NotifyIfErrors(t, err)
	require.Equal(t, p.expectedAddExecCount, addExecCount, "Count for add execution time didn't register correctly in Prometheus")

	addErrorExecCount, err := metrics.GetControllerPolicyExecCount(metrics.CreateOp, true)
	promutil.NotifyIfErrors(t, err)
	require.Equal(t, 0, addErrorExecCount, "Count for add error execution time should be 0")

	updateExecCount, err := metrics.GetControllerPolicyExecCount(metrics.UpdateOp, false)
	promutil.NotifyIfErrors(t, err)
	require.Equal(t, p.expectedUpdateExecCount, updateExecCount, "Count for update execution time didn't register correctly in Prometheus")

	updateErrorExecCount, err := metrics.GetControllerPolicyExecCount(metrics.UpdateOp, true)
	promutil.NotifyIfErrors(t, err)
	require.Equal(t, 0, updateErrorExecCount, "Count for update error execution time should be 0")

	deleteExecCount, err := metrics.GetControllerPolicyExecCount(metrics.DeleteOp, false)
	promutil.NotifyIfErrors(t, err)
	require.Equal(t, p.expectedDeleteExecCount, deleteExecCount, "Count for delete execution time didn't register correctly in Prometheus")

	deleteErrorExecCount, err := metrics.GetControllerPolicyExecCount(metrics.DeleteOp, true)
	promutil.NotifyIfErrors(t, err)
	require.Equal(t, 0, deleteErrorExecCount, "Count for delete error execution time should be 0")
}

func checkNetPolTestResult(testName string, f *netPolFixture, testCases []expectedNetPolValues) {
	for _, test := range testCases {
		if got := len(f.netPolController.rawNpMap); got != test.expectedLenOfRawNpMap {
			f.t.Errorf("Raw NetPol Map length = %d, want %d", got, test.expectedLenOfRawNpMap)
		}

		if got := f.netPolController.workqueue.Len(); got != test.expectedLenOfWorkQueue {
			f.t.Errorf("Workqueue length = %d, want %d", got, test.expectedLenOfWorkQueue)
		}

		if got := f.netPolController.isAzureNpmChainCreated; got != test.expectedIsAzureNpmChainCreated {
			f.t.Errorf("isAzureNpmChainCreated %v, want %v", got, test.expectedIsAzureNpmChainCreated)
		}

		test.netPolPromVals.testPrometheusMetrics(f.t)
	}
}

func TestAddMultipleNetworkPolicies(t *testing.T) {
	netPolObj1 := createNetPol()

	// deep copy netPolObj1 and change namespace, name, and porttype (to namedPort) since current createNetPol is not flexble.
	netPolObj2 := netPolObj1.DeepCopy()
	netPolObj2.Namespace = fmt.Sprintf("%s-new", netPolObj1.Namespace)
	netPolObj2.Name = fmt.Sprintf("%s-new", netPolObj1.Name)
	// namedPort
	netPolObj2.Spec.Ingress[0].Ports[0].Port = &intstr.IntOrString{StrVal: netPolObj2.Name}

	fexec := exec.New()
	f := newNetPolFixture(t, fexec)
	f.netPolLister = append(f.netPolLister, netPolObj1, netPolObj2)
	f.kubeobjects = append(f.kubeobjects, netPolObj1, netPolObj2)
	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newNetPolController(stopCh)

	addNetPol(t, f, netPolObj1)
	addNetPol(t, f, netPolObj2)

	// already exists (will be a no-op)
	addNetPol(t, f, netPolObj1)

	testCases := []expectedNetPolValues{
		{2, 0, true, netPolPromVals{2, 2, 0, 0}},
	}
	checkNetPolTestResult("TestAddMultipleNetPols", f, testCases)
}

func TestAddNetworkPolicy(t *testing.T) {
	netPolObj := createNetPol()

	fexec := exec.New()
	f := newNetPolFixture(t, fexec)
	f.netPolLister = append(f.netPolLister, netPolObj)
	f.kubeobjects = append(f.kubeobjects, netPolObj)
	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newNetPolController(stopCh)

	addNetPol(t, f, netPolObj)
	testCases := []expectedNetPolValues{
		{1, 0, true, netPolPromVals{1, 1, 0, 0}},
	}

	checkNetPolTestResult("TestAddNetPol", f, testCases)
}

func TestDeleteNetworkPolicy(t *testing.T) {
	netPolObj := createNetPol()

	fexec := exec.New()
	f := newNetPolFixture(t, fexec)
	f.netPolLister = append(f.netPolLister, netPolObj)
	f.kubeobjects = append(f.kubeobjects, netPolObj)
	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newNetPolController(stopCh)

	deleteNetPol(t, f, netPolObj, DeletedFinalStateknownObject)
	testCases := []expectedNetPolValues{
		{0, 0, false, netPolPromVals{0, 1, 0, 1}},
	}
	checkNetPolTestResult("TestDelNetPol", f, testCases)
}

func TestDeleteNetworkPolicyWithTombstone(t *testing.T) {
	netPolObj := createNetPol()

	fexec := exec.New()
	f := newNetPolFixture(t, fexec)
	f.netPolLister = append(f.netPolLister, netPolObj)
	f.kubeobjects = append(f.kubeobjects, netPolObj)
	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newNetPolController(stopCh)

	netPolKey := getKey(netPolObj, t)
	tombstone := cache.DeletedFinalStateUnknown{
		Key: netPolKey,
		Obj: netPolObj,
	}

	f.netPolController.deleteNetworkPolicy(tombstone)
	// the above function only adds to the workqueue
	testCases := []expectedNetPolValues{
		{0, 1, false, netPolPromVals{0, 0, 0, 0}},
	}
	checkNetPolTestResult("TestDeleteNetworkPolicyWithTombstone", f, testCases)
}

func TestDeleteNetworkPolicyWithTombstoneAfterAddingNetworkPolicy(t *testing.T) {
	netPolObj := createNetPol()

	fexec := exec.New()
	f := newNetPolFixture(t, fexec)
	f.netPolLister = append(f.netPolLister, netPolObj)
	f.kubeobjects = append(f.kubeobjects, netPolObj)
	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newNetPolController(stopCh)

	deleteNetPol(t, f, netPolObj, DeletedFinalStateUnknownObject)
	testCases := []expectedNetPolValues{
		{0, 0, false, netPolPromVals{0, 1, 0, 1}},
	}
	checkNetPolTestResult("TestDeleteNetworkPolicyWithTombstoneAfterAddingNetworkPolicy", f, testCases)
}

// this unit test is for the case where states of network policy are changed, but network policy controller does not need to reconcile.
// Check it with expectedEnqueueEventIntoWorkQueue variable.
func TestUpdateNetworkPolicy(t *testing.T) {
	oldNetPolObj := createNetPol()

	fexec := exec.New()
	f := newNetPolFixture(t, fexec)
	f.netPolLister = append(f.netPolLister, oldNetPolObj)
	f.kubeobjects = append(f.kubeobjects, oldNetPolObj)
	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newNetPolController(stopCh)

	newNetPolObj := oldNetPolObj.DeepCopy()
	// oldNetPolObj.ResourceVersion value is "0"
	newRV, _ := strconv.Atoi(oldNetPolObj.ResourceVersion)
	newNetPolObj.ResourceVersion = fmt.Sprintf("%d", newRV+1)

	updateNetPol(t, f, oldNetPolObj, newNetPolObj)
	// no need to reconcile because only the rv changes, so we don't see a prometheus update exec count
	testCases := []expectedNetPolValues{
		{1, 0, true, netPolPromVals{1, 1, 0, 0}},
	}
	checkNetPolTestResult("TestUpdateNetPol", f, testCases)
}

func TestLabelUpdateNetworkPolicy(t *testing.T) {
	oldNetPolObj := createNetPol()

	fexec := exec.New()
	f := newNetPolFixture(t, fexec)
	f.netPolLister = append(f.netPolLister, oldNetPolObj)
	f.kubeobjects = append(f.kubeobjects, oldNetPolObj)
	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newNetPolController(stopCh)

	newNetPolObj := oldNetPolObj.DeepCopy()
	// update podSelctor in a new network policy field
	newNetPolObj.Spec.PodSelector = metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app": "test",
			"new": "test",
		},
	}
	// oldNetPolObj.ResourceVersion value is "0"
	newRV, _ := strconv.Atoi(oldNetPolObj.ResourceVersion)
	newNetPolObj.ResourceVersion = fmt.Sprintf("%d", newRV+1)
	updateNetPol(t, f, oldNetPolObj, newNetPolObj)

	testCases := []expectedNetPolValues{
		{1, 0, true, netPolPromVals{1, 1, 1, 0}},
	}
	checkNetPolTestResult("TestUpdateNetPol", f, testCases)
}
