package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Azure/azure-container-networking/npm/http/api"
	"github.com/stretchr/testify/assert"

	"github.com/Azure/azure-container-networking/npm"
)

func TestGetNpmMgrHandler(t *testing.T) {
	assert := assert.New(t)
	npMgr := &npm.NetworkPolicyManager{
		PodMap: map[string]*npm.NpmPod{
			"": &npm.NpmPod{
				Name: "testpod",
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
