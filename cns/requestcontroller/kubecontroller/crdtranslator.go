package kubecontroller

import (
	"github.com/Azure/azure-container-networking/cns"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

// CRDStatusToCNS translates a crd status to cns recognizable data
func CRDStatusToCNS(crdStatus *nnc.NodeNetworkConfigStatus) (*cns.CreateNetworkContainerRequest, []*cns.ContainerIPConfigState, error) {
	//TODO: translate status to CNS state
	return nil, nil, nil
}

// CNSToCRDSpec translates CNS's list of Ips to be released and requested ip count into a CRD Spec
func CNSToCRDSpec() (*nnc.NodeNetworkConfigSpec, error) {
	//TODO: Translate list of ips to be released and requested ip count to CRD spec
	return nil, nil
}
