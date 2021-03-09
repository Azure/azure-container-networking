package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Azure/azure-container-networking/npm/http/api"
	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/Azure/azure-container-networking/npm"
)

func TestGetNpmMgrHandler(t *testing.T) {
	assert := assert.New(t)
	npMgr := &npm.NetworkPolicyManager{
		NsMap: map[string]*npm.Namespace{
			"test": &npm.Namespace{
				PodMap: map[types.UID]*corev1.Pod{
					"": &corev1.Pod{
						TypeMeta: metav1.TypeMeta{},
						ObjectMeta: metav1.ObjectMeta{
							Name: "testpod",
						},
						Spec:   corev1.PodSpec{},
						Status: corev1.PodStatus{},
					},
				},
			},
		},
	}
	n := NewNpmRestServer("")
	handler := n.GetNpmMgr(npMgr)

	req, err := http.NewRequest(http.MethodGet, api.NPMMgrPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	var ns npm.NetworkPolicyManager
	err = json.NewDecoder(rr.Body).Decode(&ns)
	if err != nil {
		t.Fatal(err)
	}

	assert.Exactly(&ns, npMgr)
}
