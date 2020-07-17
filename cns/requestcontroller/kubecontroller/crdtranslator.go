package kubecontroller

import (
	"github.com/Azure/azure-container-networking/cns"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

<<<<<<< HEAD
// CRDStatusToCNS translates a crd status to cns recognizable data
func CRDStatusToCNS(crdStatus *nnc.NodeNetworkConfigStatus) ([]*cns.ContainerIPConfigState, error) {
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
	return ipConfigs, nil
}

// CNSToCRDSpec translates CNS's list of Ips to be released and requested ip count into a CRD Spec
func CNSToCRDSpec(ipConfigs []*cns.ContainerIPConfigState, requestedIPCount int) (*nnc.NodeNetworkConfigSpec, error) {
	var (
		spec *nnc.NodeNetworkConfigSpec
	)

	for _, ipConfig := range ipConfigs {
		spec.IPsNotInUse = append(spec.IPsNotInUse, ipConfig.IPConfig.IPAddress)
	}
	spec.RequestedIPCount = int64(requestedIPCount)

	return spec, nil
=======
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
>>>>>>> reconcile-on-start
}
