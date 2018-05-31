package npm

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Azure/azure-container-networking/cni/npm/ipsm"
	"github.com/Azure/azure-container-networking/cni/npm/iptm"
)

func TestAddPod(t *testing.T) {

	npMgr := &NetworkPolicyManager{
		nsMap:  make(map[string]*namespace),
		ipsMgr: ipsm.NewIpsetManager(),
		iptMgr: iptm.NewIptablesManager(),
	}

	testPodIP := "1.2.3.4"
	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID:       "test",
			Namespace: "default",
			Name:      "test",
			Labels:    map[string]string{"app": "test"}},
		Status: corev1.PodStatus{PodIP: testPodIP},
	}

	if err := npMgr.AddPod(testPod); err != nil {
		t.Errorf("TestAddPod failed")
	}
}
