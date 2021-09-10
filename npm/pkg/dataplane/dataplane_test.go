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
	set := ipsets.NewIPSet("test", ipsets.NameSpace)

	err := dp.CreateIPSet(set)
	if err != nil {
		t.Error("CreateIPSet() returned error")
	}
}
