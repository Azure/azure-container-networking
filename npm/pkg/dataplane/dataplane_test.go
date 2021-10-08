package dataplane

import (
	"testing"

	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/mocks"
	"github.com/golang/mock/gomock"
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

// gomock sample usage for generated mock dataplane
func TestAddToList(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := mocks.NewMockGenericDataplane(ctrl)
	m.EXPECT().AddToList("test", []string{"test"}).Return(nil)
	m.AddToList("test", []string{"test"})
}
