// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"reflect"
	"testing"

	"github.com/Azure/azure-container-networking/npm/ipsm"
	"github.com/Azure/azure-container-networking/npm/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsValidPod(t *testing.T) {
	podObj := &corev1.Pod{
		Status: corev1.PodStatus{
			Phase: "Running",
			PodIP: "1.2.3.4",
		},
	}
	if ok := isValidPod(podObj); !ok {
		t.Errorf("TestisValidPod failed @ isValidPod")
	}
}

func TestIsSystemPod(t *testing.T) {
	podObj := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: util.KubeSystemFlag,
		},
	}
	if ok := isSystemPod(podObj); !ok {
		t.Errorf("TestisSystemPod failed @ isSystemPod")
	}
}

func TestAddPod(t *testing.T) {
	npMgr := &NetworkPolicyManager{
		nsMap:            make(map[string]*namespace),
		TelemetryEnabled: false,
	}

	allNs, err := newNs(util.KubeAllNamespacesFlag)
	if err != nil {
		panic(err.Error)
	}
	npMgr.nsMap[util.KubeAllNamespacesFlag] = allNs

	ipsMgr := ipsm.NewIpsetManager()
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestAddPod failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestAddPod failed @ ipsMgr.Restore")
		}
	}()

	podObj := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "test-pod",
			},
		},
		Status: corev1.PodStatus{
			Phase: "Running",
			PodIP: "1.2.3.4",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				corev1.Container{
					Ports: []corev1.ContainerPort{
						corev1.ContainerPort{
							Name:          "app:test-pod",
							ContainerPort: 8080,
						},
					},
				},
			},
		},
	}

	npMgr.Lock()
	if err := npMgr.AddPod(podObj); err != nil {
		t.Errorf("TestAddPod failed @ AddPod")
	}
	npMgr.Unlock()
}

func TestUpdatePod(t *testing.T) {
	npMgr := &NetworkPolicyManager{
		nsMap:            make(map[string]*namespace),
		TelemetryEnabled: false,
	}

	allNs, err := newNs(util.KubeAllNamespacesFlag)
	if err != nil {
		panic(err.Error)
	}
	npMgr.nsMap[util.KubeAllNamespacesFlag] = allNs

	ipsMgr := ipsm.NewIpsetManager()
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestUpdatePod failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestUpdatePod failed @ ipsMgr.Restore")
		}
	}()

	oldPodObj := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "old-test-pod",
			Namespace: "test-namespace",
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
			Name:      "new-test-pod",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "new-test-pod",
			},
			ResourceVersion: "1",
		},
		Status: corev1.PodStatus{
			Phase: "Running",
			PodIP: "4.3.2.1",
		},
	}

	npMgr.Lock()
	if err := npMgr.AddPod(oldPodObj); err != nil {
		t.Errorf("TestUpdatePod failed @ AddPod")
	}

	if err := npMgr.UpdatePod(newPodObj); err != nil {
		t.Errorf("TestUpdatePod failed @ UpdatePod")
	}

	cachedPodObj, exists := npMgr.nsMap["ns-"+newPodObj.Namespace].podMap[string(newPodObj.ObjectMeta.UID)]
	if !exists {
		t.Errorf("TestUpdatePod failed @ pod exists check")
	}

	if !reflect.DeepEqual(cachedPodObj.labels, newPodObj.Labels) {
		t.Errorf("TestUpdatePod failed @ labels check")
	}
	npMgr.Unlock()
}

func TestOldRVUpdatePod(t *testing.T) {
	npMgr := &NetworkPolicyManager{
		nsMap:            make(map[string]*namespace),
		TelemetryEnabled: false,
	}

	allNs, err := newNs(util.KubeAllNamespacesFlag)
	if err != nil {
		panic(err.Error)
	}
	npMgr.nsMap[util.KubeAllNamespacesFlag] = allNs

	ipsMgr := ipsm.NewIpsetManager()
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestOldRVUpdatePod failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestOldRVUpdatePod failed @ ipsMgr.Restore")
		}
	}()

	oldPodObj := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "old-test-pod",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "old-test-pod",
			},
			ResourceVersion: "1",
		},
		Status: corev1.PodStatus{
			Phase: "Running",
			PodIP: "1.2.3.4",
		},
	}

	newPodObj := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "new-test-pod",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "new-test-pod",
			},
			ResourceVersion: "0",
		},
		Status: corev1.PodStatus{
			Phase: "Running",
			PodIP: "4.3.2.1",
		},
	}

	npMgr.Lock()
	if err := npMgr.AddPod(oldPodObj); err != nil {
		t.Errorf("TestOldRVUpdatePod failed @ AddPod")
	}

	if err := npMgr.UpdatePod(newPodObj); err != nil {
		t.Errorf("TestOldRVUpdatePod failed @ UpdatePod")
	}

	cachedPodObj, exists := npMgr.nsMap["ns-"+newPodObj.Namespace].podMap[string(newPodObj.ObjectMeta.UID)]
	if !exists {
		t.Errorf("TestOldRVUpdatePod failed @ pod exists check")
	}

	if cachedPodObj.resourceVersion != 1 {
		t.Errorf("TestOldRVUpdatePod failed @ resourceVersion check")
	}

	if !reflect.DeepEqual(cachedPodObj.labels, oldPodObj.Labels) {
		t.Errorf("TestOldRVUpdatePod failed @ labels check")
	}

	npMgr.Unlock()
}

func TestDeletePod(t *testing.T) {
	npMgr := &NetworkPolicyManager{
		nsMap:            make(map[string]*namespace),
		TelemetryEnabled: false,
	}

	allNs, err := newNs(util.KubeAllNamespacesFlag)
	if err != nil {
		panic(err.Error)
	}
	npMgr.nsMap[util.KubeAllNamespacesFlag] = allNs

	ipsMgr := ipsm.NewIpsetManager()
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestDeletePod failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestDeletePod failed @ ipsMgr.Restore")
		}
	}()

	podObj := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "test-pod",
			},
		},
		Status: corev1.PodStatus{
			Phase: "Running",
			PodIP: "1.2.3.4",
		},
	}

	npMgr.Lock()
	if err := npMgr.AddPod(podObj); err != nil {
		t.Errorf("TestDeletePod failed @ AddPod")
	}

	if err := npMgr.DeletePod(podObj); err != nil {
		t.Errorf("TestDeletePod failed @ DeletePod")
	}

	if len(npMgr.nsMap["ns-"+podObj.Namespace].podMap) > 1 {
		t.Errorf("TestDeletePod failed @ podMap length check")
	}
	npMgr.Unlock()
}

func TestAddHostNetworkPod(t *testing.T) {
	npMgr := &NetworkPolicyManager{
		nsMap:            make(map[string]*namespace),
		TelemetryEnabled: false,
	}

	allNs, err := newNs(util.KubeAllNamespacesFlag)
	if err != nil {
		panic(err.Error)
	}
	npMgr.nsMap[util.KubeAllNamespacesFlag] = allNs

	ipsMgr := ipsm.NewIpsetManager()
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestAddHostNetworkPod failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestAddHostNetworkPod failed @ ipsMgr.Restore")
		}
	}()

	podObj := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "test-pod",
			},
		},
		Status: corev1.PodStatus{
			Phase: "Running",
			PodIP: "1.2.3.4",
		},
		Spec: corev1.PodSpec{
			HostNetwork: true,
		},
	}

	npMgr.Lock()
	if err := npMgr.AddPod(podObj); err != nil {
		t.Errorf("TestAddHostNetworkPod failed @ AddPod")
	}

	if len(npMgr.nsMap) > 1 {
		t.Errorf("TestAddHostNetworkPod failed @ nsMap length check")
	}
	npMgr.Unlock()
}

func TestUpdateHostNetworkPod(t *testing.T) {
	npMgr := &NetworkPolicyManager{
		nsMap:            make(map[string]*namespace),
		TelemetryEnabled: false,
	}

	allNs, err := newNs(util.KubeAllNamespacesFlag)
	if err != nil {
		panic(err.Error)
	}
	npMgr.nsMap[util.KubeAllNamespacesFlag] = allNs

	ipsMgr := ipsm.NewIpsetManager()
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestUpdateHostNetworkPod failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestUpdateHostNetworkPod failed @ ipsMgr.Restore")
		}
	}()

	// HostNetwork check is done on the oldPodObj,
	// so intentionally not adding hostnet true in newPodObj
	oldPodObj := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "old-test-pod",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "old-test-pod",
			},
		},
		Status: corev1.PodStatus{
			Phase: "Running",
			PodIP: "1.2.3.4",
		},
		Spec: corev1.PodSpec{
			HostNetwork: true,
		},
	}

	newPodObj := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "new-test-pod",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "new-test-pod",
			},
		},
		Status: corev1.PodStatus{
			Phase: "Running",
			PodIP: "4.3.2.1",
		},
		Spec: corev1.PodSpec{
			HostNetwork: true,
		},
	}

	npMgr.Lock()
	if err := npMgr.AddPod(oldPodObj); err != nil {
		t.Errorf("TestUpdateHostNetworkPod failed @ AddPod")
	}

	if err := npMgr.UpdatePod(newPodObj); err != nil {
		t.Errorf("TestUpdateHostNetworkPod failed @ UpdatePod")
	}

	if len(npMgr.nsMap) > 1 {
		t.Errorf("TestUpdateHostNetworkPod failed @ podMap length check")
	}
	npMgr.Unlock()
}

func TestDeleteHostNetworkPod(t *testing.T) {
	npMgr := &NetworkPolicyManager{
		nsMap:            make(map[string]*namespace),
		TelemetryEnabled: false,
	}

	allNs, err := newNs(util.KubeAllNamespacesFlag)
	if err != nil {
		panic(err.Error)
	}
	npMgr.nsMap[util.KubeAllNamespacesFlag] = allNs

	ipsMgr := ipsm.NewIpsetManager()
	if err := ipsMgr.Save(util.IpsetTestConfigFile); err != nil {
		t.Errorf("TestDeleteHostNetworkPod failed @ ipsMgr.Save")
	}

	defer func() {
		if err := ipsMgr.Restore(util.IpsetTestConfigFile); err != nil {
			t.Errorf("TestDeleteHostNetworkPod failed @ ipsMgr.Restore")
		}
	}()

	podObj := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app": "test-pod",
			},
		},
		Status: corev1.PodStatus{
			Phase: "Running",
			PodIP: "1.2.3.4",
		},
		Spec: corev1.PodSpec{
			HostNetwork: true,
		},
	}

	npMgr.Lock()
	if err := npMgr.AddPod(podObj); err != nil {
		t.Errorf("TestDeleteHostNetworkPod failed @ AddPod")
	}

	if len(npMgr.nsMap) > 1 {
		t.Errorf("TestDeleteHostNetworkPod failed @ podMap length check")
	}

	if err := npMgr.DeletePod(podObj); err != nil {
		t.Errorf("TestDeleteHostNetworkPod failed @ DeletePod")
	}
	npMgr.Unlock()
}
