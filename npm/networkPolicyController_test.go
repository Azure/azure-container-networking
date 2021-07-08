// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/Azure/azure-container-networking/npm/ipsm"
	"github.com/Azure/azure-container-networking/npm/iptm"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/metrics/promutil"

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

	kubeclient *k8sfake.Clientset
	// Objects to put in the store.
	netPolLister []*networkingv1.NetworkPolicy
	// (TODO) Actions expected to happen on the client. Will use this to check action.
	kubeactions []core.Action
	// Objects from here preloaded into NewSimpleFake.
	kubeobjects []runtime.Object

	// (TODO) will remove npMgr if possible
	npMgr  *NetworkPolicyManager
	ipsMgr *ipsm.IpsetManager
	iptMgr *iptm.IptablesManager

	netPolController *networkPolicyController
	kubeInformer     kubeinformers.SharedInformerFactory

	// to test whether unnecessary enqueuing event into workqueue was correctly filtered in eventhandler code of network policy controller
	isEnqueueEventIntoWorkQueue bool
}

func newNetPolFixture(t *testing.T, utilexec exec.Interface) *netPolFixture {
	f := &netPolFixture{
		t:                           t,
		netPolLister:                []*networkingv1.NetworkPolicy{},
		kubeobjects:                 []runtime.Object{},
		npMgr:                       newNPMgr(t, utilexec),
		ipsMgr:                      ipsm.NewIpsetManager(utilexec),
		iptMgr:                      iptm.NewIptablesManager(utilexec, iptm.NewFakeIptOperationShim()),
		isEnqueueEventIntoWorkQueue: true,
	}

	f.npMgr.RawNpMap = make(map[string]*networkingv1.NetworkPolicy)

	// While running "make test-all", metrics hold states which was executed in previous unit test.
	// (TODO): Need to fix to remove this fundamental dependency
	metrics.ReInitializeAllMetrics()

	return f
}

func (f *netPolFixture) newNetPolController(stopCh chan struct{}) {
	f.kubeclient = k8sfake.NewSimpleClientset(f.kubeobjects...)
	f.kubeInformer = kubeinformers.NewSharedInformerFactory(f.kubeclient, noResyncPeriodFunc())

	f.netPolController = NewNetworkPolicyController(f.kubeInformer.Networking().V1().NetworkPolicies(), f.kubeclient, f.npMgr)
	f.netPolController.netPolListerSynced = alwaysReady

	for _, netPol := range f.netPolLister {
		f.kubeInformer.Networking().V1().NetworkPolicies().Informer().GetIndexer().Add(netPol)
	}

	// Do not start informer to avoid unnecessary event triggers
	// (TODO): Leave stopCh and below commented code to enhance UTs to even check event triggers as well later if possible
	//f.kubeInformer.Start(stopCh)
}

func (f *netPolFixture) saveIpTables(iptablesConfigFile string) {
	if err := f.iptMgr.Save(iptablesConfigFile); err != nil {
		f.t.Errorf("Failed to save iptables rules")
	}
}

func (f *netPolFixture) restoreIpTables(iptablesConfigFile string) {
	if err := f.iptMgr.Restore(iptablesConfigFile); err != nil {
		f.t.Errorf("Failed to restore iptables rules")
	}
}

func (f *netPolFixture) saveIpSet(ipsetConfigFile string) {
	//  call /sbin/ipset save -file /var/log/ipset-test.conf
	f.t.Logf("Start storing ipset to %s", ipsetConfigFile)
	if err := f.ipsMgr.Save(ipsetConfigFile); err != nil {
		f.t.Errorf("Failed to save ipsets")
	}
}
func (f *netPolFixture) restoreIpSet(ipsetConfigFile string) {
	//  call /sbin/ipset restore -file /var/log/ipset-test.conf
	f.t.Logf("Start re-storing ipset to %s", ipsetConfigFile)
	if err := f.ipsMgr.Restore(ipsetConfigFile); err != nil {
		f.t.Errorf("failed to restore ipsets")
	}
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
		f.isEnqueueEventIntoWorkQueue = false
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
		f.isEnqueueEventIntoWorkQueue = false
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
		f.isEnqueueEventIntoWorkQueue = false
		return
	}

	f.netPolController.processNextWorkItem()
}

type expectedNetPolValues struct {
	expectedLenOfNsMap                int
	expectedLenOfRawNpMap             int
	expectedLenOfWorkQueue            int
	expectedIsAzureNpmChainCreated    bool
	expectedEnqueueEventIntoWorkQueue bool
	// prometheus metrics
	expectedNumPoliciesMetric                   int
	expectedNumPoliciesMetricError              error
	expectedCountOfAddPolicyExecTimeMetric      int
	expectedCountOfAddPolicyExecTimeMetricError error
}

func checkNetPolTestResult(testName string, f *netPolFixture, testCases []expectedNetPolValues) {
	for _, test := range testCases {
		if got := len(f.npMgr.NsMap); got != test.expectedLenOfNsMap {
			f.t.Errorf("npMgr namespace map length = %d, want %d", got, test.expectedLenOfNsMap)
		}

		if got := len(f.netPolController.npMgr.RawNpMap); got != test.expectedLenOfRawNpMap {
			f.t.Errorf("Raw NetPol Map length = %d, want %d", got, test.expectedLenOfRawNpMap)
		}

		if got := f.netPolController.workqueue.Len(); got != test.expectedLenOfWorkQueue {
			f.t.Errorf("Workqueue length = %d, want %d", got, test.expectedLenOfWorkQueue)
		}

		if got := f.netPolController.isAzureNpmChainCreated; got != test.expectedIsAzureNpmChainCreated {
			f.t.Errorf("isAzureNpmChainCreated %v, want %v", got, test.expectedIsAzureNpmChainCreated)
		}

		if got := f.isEnqueueEventIntoWorkQueue; got != test.expectedEnqueueEventIntoWorkQueue {
			f.t.Errorf("isEnqueueEventIntoWorkQueue %v, want %v", got, test.expectedEnqueueEventIntoWorkQueue)
		}

		// Check prometheus metrics
		expectedNumPoliciesMetrics, expectedNumPoliciesMetricsError := promutil.GetValue(metrics.NumPolicies)
		if expectedNumPoliciesMetrics != test.expectedNumPoliciesMetric {
			f.t.Errorf("NumPolicies metric length = %d, want %d", expectedNumPoliciesMetrics, test.expectedNumPoliciesMetric)
		}
		if expectedNumPoliciesMetricsError != test.expectedNumPoliciesMetricError {
			f.t.Errorf("NumPolicies metric error = %s, want %s", expectedNumPoliciesMetricsError, test.expectedNumPoliciesMetricError)
		}

		expectedCountOfAddPolicyExecTimeMetric, expectedCountOfAddPolicyExecTimeMetricError := promutil.GetCountValue(metrics.AddPolicyExecTime)
		if expectedCountOfAddPolicyExecTimeMetric != test.expectedCountOfAddPolicyExecTimeMetric {
			f.t.Errorf("CountOfAddPolicyExecTime metric length = %d, want %d", expectedCountOfAddPolicyExecTimeMetric, test.expectedCountOfAddPolicyExecTimeMetric)
		}
		if expectedCountOfAddPolicyExecTimeMetricError != test.expectedCountOfAddPolicyExecTimeMetricError {
			f.t.Errorf("CountOfAddPolicyExecTime metric error = %s, want %s", expectedCountOfAddPolicyExecTimeMetricError, test.expectedCountOfAddPolicyExecTimeMetricError)
		}
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

	testCases := []expectedNetPolValues{
		{1, 2, 0, true, true, 2, nil, 2, nil},
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
		{1, 1, 0, true, true, 1, nil, 1, nil},
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
		{1, 0, 0, false, true, 0, nil, 1, nil},
	}
	checkNetPolTestResult("TestDelNetPol", f, testCases)
}

func TestDeleteNetworkPolicyWithTombstone(t *testing.T) {
	netPolObj := createNetPol()

	fexec := exec.New()
	f := newNetPolFixture(t, fexec)
	f.isEnqueueEventIntoWorkQueue = false
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
	testCases := []expectedNetPolValues{
		{1, 0, 0, false, false, 0, nil, 0, nil},
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
		{1, 0, 0, false, true, 0, nil, 1, nil},
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
	testCases := []expectedNetPolValues{
		{1, 1, 0, true, false, 1, nil, 1, nil},
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
		{1, 1, 0, true, true, 1, nil, 2, nil},
	}
	checkNetPolTestResult("TestUpdateNetPol", f, testCases)
}
