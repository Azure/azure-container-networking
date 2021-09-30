package server

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Azure/azure-container-networking/npm"
	"github.com/Azure/azure-container-networking/npm/http/api"
	"github.com/Azure/azure-container-networking/npm/ipsm"
	controllersv1 "github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/v1"
	"github.com/stretchr/testify/assert"
)

func TestGetNPMCacheHandler(t *testing.T) {
	assert := assert.New(t)

	nodeName := "nodename"
	npmCacheEncoder := npm.CacheEncoder(nodeName)
	n := &NPMRestServer{}
	handler := n.npmCacheHandler(npmCacheEncoder)

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

	byteArray, err := ioutil.ReadAll(rr.Body)
	if err != nil {
		t.Errorf("failed to read response's data : %w", err)
	}

	actual := &controllersv1.Cache{}
	err = json.Unmarshal(byteArray, actual)
	if err != nil {
		t.Fatalf("failed to unmarshal %s due to %v", string(byteArray), err)
	}

	expected := &controllersv1.Cache{
		NodeName: nodeName,
		NsMap:    make(map[string]*controllersv1.Namespace),
		PodMap:   make(map[string]*controllersv1.NpmPod),
		ListMap:  make(map[string]*ipsm.Ipset),
		SetMap:   make(map[string]*ipsm.Ipset),
	}

	assert.Exactly(expected, actual)
}
