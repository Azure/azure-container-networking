package kubecontroller

import (
	"github.com/Azure/azure-container-networking/cns"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

// CRDStatusToNCRequest translates a crd status to network container request
func CRDStatusToNCRequest(crdStatus *nnc.NodeNetworkConfigStatus) (*cns.CreateNetworkContainerRequest, error) {
	//TODO: Translate CRD status into network container request
	//Mat will pick up from here
	return nil, nil
}

// CNSToCRDSpec translates CNS's list of Ips to be released and requested ip count into a CRD Spec
func CNSToCRDSpec() (*nnc.NodeNetworkConfigSpec, error) {
	//TODO: Translate list of ips to be released and requested ip count to CRD spec
	return nil, nil
}
