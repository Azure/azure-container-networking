package kubecontroller

import (
	"github.com/Azure/azure-container-networking/cns"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

// CRDStatusToCNS translates a crd status to cns recognizable data
func CRDStatusToCNS(crdStatus *nnc.NodeNetworkConfigStatus) (*cns.CreateNetworkContainerRequest, []*cns.ContainerIPConfigState, error) {
	//TODO: translate status to CNS state
	var (
		ipConfigs []*cns.ContainerIPConfigState
	)
	for _, nc := range crdStatus.NetworkContainers {
		for _, ipAssignment := range nc.IPAssignments {
			ipConfig := &cns.ContainerIPConfigState{
				IPConfig: cns.IPSubnet{
					IPAddress: ipAssignment.IP,
				},
				ID:   ipAssignment.Name,
				NCID: nc.ID,
			}
			ipConfigs = append(ipConfigs, ipConfig)
		}
	}
	return nil, nil, nil
}

// CNSToCRDSpec translates CNS's list of Ips to be released and requested ip count into a CRD Spec
func CNSToCRDSpec() (*nnc.NodeNetworkConfigSpec, error) {
	//TODO: Translate list of ips to be released and requested ip count to CRD spec
	return nil, nil
}
