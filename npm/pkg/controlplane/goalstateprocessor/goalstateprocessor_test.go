package goalstateprocessor

import (
	"context"
	"testing"

	dpmocks "github.com/Azure/azure-container-networking/npm/pkg/dataplane/mocks"
	"github.com/Azure/azure-container-networking/npm/pkg/protos"
	"github.com/golang/mock/gomock"
)

func TestNewGoalStateProcessor(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dp := dpmocks.NewMockGenericDataplane(ctrl)
	var inputChan chan *protos.Events
	ctx, cancel := context.WithCancel(context.Background())
	gsp := NewGoalStateProcessor(ctx, "node1", "pod1", inputChan, dp)
	gsp.run()

	cancel()
}
