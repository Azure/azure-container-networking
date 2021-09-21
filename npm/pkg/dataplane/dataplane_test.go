package dataplane

import (
	"testing"

	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
)

func TestNewDataPlane(t *testing.T) {
	metrics.InitializeAll()
	dp := NewDataPlane()

	if dp == nil {
		t.Error("NewDataPlane() returned nil")
	}

	err := dp.CreateIPSet("test", ipsets.NameSpace)
	if err != nil {
		t.Error("CreateIPSet() returned error")
	}
}
