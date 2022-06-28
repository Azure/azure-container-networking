package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Azure/azure-container-networking/npm"
	"github.com/Azure/azure-container-networking/npm/http/api"
	"github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/common"
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

	byteArray, err := io.ReadAll(rr.Body)
	if err != nil {
		t.Errorf("failed to read response's data : %v", err)
	}

	actual := &common.Cache{}
	err = json.Unmarshal(byteArray, actual)
	if err != nil {
		t.Fatalf("failed to unmarshal %s due to %v", string(byteArray), err)
	}

	expected := &common.Cache{
		NodeName: nodeName,
		NsMap:    make(map[string]*common.Namespace),
		PodMap:   make(map[string]*common.NpmPod),
		ListMap:  make(map[string]string),
		SetMap:   make(map[string]string),
	}

	assert.Exactly(expected, actual)
}
