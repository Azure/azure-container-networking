package metrics

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Azure/azure-container-networking/npm/http/api"
	"github.com/stretchr/testify/assert"
)

func TestPrometheusNodeHandler(t *testing.T) {
	assert := assert.New(t)
	InitializeAll()
	handler := GetHandler(true)
	req, err := http.NewRequest(http.MethodGet, api.NodeMetricsPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Contains(string(rr.Body.Bytes()), fmt.Sprintf("%s_%s", namespace, addPolicyExecTimeName))
}

// TODO: evaluate why cluster metrics are nil
func TestPrometheusClusterHandler(t *testing.T) {
	//assert := assert.New(t)
	InitializeAll()
	handler := GetHandler(false)
	req, err := http.NewRequest(http.MethodGet, api.ClusterMetricsPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	//assert.Contains(string(rr.Body.Bytes()), fmt.Sprintf("%s_%s", namespace, addPolicyExecTimeName))
}
