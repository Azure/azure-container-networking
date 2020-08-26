package ipampoolmonitor

import (
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/fakes"
)

func TestInterfaces(t *testing.T) {
	fakecns := fakes.NewHTTPServiceFake()
	fakerc := fakes.NewRequestControllerFake()

	fakecns.PoolMonitor = NewCNSIPAMPoolMonitor(fakecns, fakerc)

	scalarUnits := cns.ScalarUnits{}

	fakecns.PoolMonitor.UpdatePoolLimitsTransacted(scalarUnits)
}
