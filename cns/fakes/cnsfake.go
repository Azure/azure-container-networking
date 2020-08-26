package fakes

import (
	"encoding/json"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
)

type HTTPServiceFake struct {
	PoolMonitor cns.IPAMPoolMonitor
}

func NewHTTPServiceFake() *HTTPServiceFake {
	return &HTTPServiceFake{}
}

func (fake *HTTPServiceFake) SendNCSnapShotPeriodically(int, chan bool) {

}

func (fake *HTTPServiceFake) SetNodeOrchestrator(*cns.SetOrchestratorTypeRequest) {

}

func (fake *HTTPServiceFake) SyncNodeStatus(string, string, string, json.RawMessage) (int, string) {
	return 0, ""
}

func (fake *HTTPServiceFake) GetAvailableIPConfigs() []cns.IPConfigurationStatus {
	return []cns.IPConfigurationStatus{}
}

func (fake *HTTPServiceFake) GetPodIPConfigState() map[string]cns.IPConfigurationStatus {
	return make(map[string]cns.IPConfigurationStatus)
}

func (fake *HTTPServiceFake) MarkIPsAsPending(numberToMark int) (map[string]cns.SecondaryIPConfig, error) {
	return make(map[string]cns.SecondaryIPConfig), nil
}

func (fake *HTTPServiceFake) GetOption(string) interface{} {
	return nil
}

func (fake *HTTPServiceFake) SetOption(string, interface{}) {

}

func (fake *HTTPServiceFake) Start(*common.ServiceConfig) error {
	return nil
}

func (fake *HTTPServiceFake) Stop() {

}
