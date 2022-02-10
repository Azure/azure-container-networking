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
	handler := GetHandler(NodeMetrics)
	req, err := http.NewRequest(http.MethodGet, api.NodeMetricsPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Contains(rr.Body.String(), fmt.Sprintf("%s_%s", namespace, addPolicyExecTimeName))
}

func TestPrometheusClusterHandler(t *testing.T) {
	assert := assert.New(t)
	InitializeAll()
	handler := GetHandler(ClusterMetrics)
	req, err := http.NewRequest(http.MethodGet, api.ClusterMetricsPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Contains(rr.Body.String(), fmt.Sprintf("%s_%s", namespace, numPoliciesName))
}
