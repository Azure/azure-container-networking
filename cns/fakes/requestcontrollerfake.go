//go:build !ignore_uncovered
// +build !ignore_uncovered

package fakes

import (
	"context"
	"net"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/google/uuid"
)

type RequestControllerFake struct {
	cnscli *HTTPServiceFake
	NNC    *v1alpha.NodeNetworkConfig
	ip     net.IP
}

func NewRequestControllerFake(cnsService *HTTPServiceFake, scalar v1alpha.Scaler, subnetAddressSpace string, numberOfIPConfigs int) *RequestControllerFake {
	rc := &RequestControllerFake{
		cnscli: cnsService,
		NNC: &v1alpha.NodeNetworkConfig{
			Spec: v1alpha.NodeNetworkConfigSpec{},
			Status: v1alpha.NodeNetworkConfigStatus{
				Scaler: scalar,
				NetworkContainers: []v1alpha.NetworkContainer{
					{
						SubnetAddressSpace: subnetAddressSpace,
					},
				},
			},
		},
	}

	rc.ip, _, _ = net.ParseCIDR(subnetAddressSpace)

	rc.CarveIPConfigsAndAddToStatusAndCNS(numberOfIPConfigs)
	rc.NNC.Spec.RequestedIPCount = int64(numberOfIPConfigs)

	return rc
}

func (rc *RequestControllerFake) CarveIPConfigsAndAddToStatusAndCNS(numberOfIPConfigs int) []cns.IPConfigurationStatus {
	var cnsIPConfigs []cns.IPConfigurationStatus
	for i := 0; i < numberOfIPConfigs; i++ {

		ipconfigCRD := v1alpha.IPAssignment{
			Name: uuid.New().String(),
			IP:   rc.ip.String(),
		}
		rc.NNC.Status.NetworkContainers[0].IPAssignments = append(rc.NNC.Status.NetworkContainers[0].IPAssignments, ipconfigCRD)

		ipconfigCNS := cns.IPConfigurationStatus{
			ID:        ipconfigCRD.Name,
			IPAddress: ipconfigCRD.IP,
			State:     cns.Available,
		}
		cnsIPConfigs = append(cnsIPConfigs, ipconfigCNS)

		incrementIP(rc.ip)
	}

	rc.cnscli.IPStateManager.AddIPConfigs(cnsIPConfigs)

	return cnsIPConfigs
}

func (rc *RequestControllerFake) Init(context.Context) error {
	return nil
}

func (rc *RequestControllerFake) Start(context.Context) error {
	return nil
}

func (rc *RequestControllerFake) IsStarted() bool {
	return true
}

func remove(slice []v1alpha.IPAssignment, s int) []v1alpha.IPAssignment {
	return append(slice[:s], slice[s+1:]...)
}

func (rc *RequestControllerFake) Reconcile(removePendingReleaseIPs bool) error {
	diff := int(rc.NNC.Spec.RequestedIPCount) - len(rc.cnscli.GetPodIPConfigState())

	if diff > 0 {
		// carve the difference of test IPs and add them to CNS, assume dnc has populated the CRD status
		rc.CarveIPConfigsAndAddToStatusAndCNS(diff)
	} else if diff < 0 {
		// Assume DNC has removed the IPConfigs from the status

		// mimic DNC removing IPConfigs from the CRD
		for _, notInUseIPConfigName := range rc.NNC.Spec.IPsNotInUse {

			// remove ipconfig from status
			index := 0
			for _, ipconfig := range rc.NNC.Status.NetworkContainers[0].IPAssignments {
				if notInUseIPConfigName == ipconfig.Name {
					break
				}
				index++
			}
			rc.NNC.Status.NetworkContainers[0].IPAssignments = remove(rc.NNC.Status.NetworkContainers[0].IPAssignments, index)

		}
	}

	// remove ipconfig from CNS
	if removePendingReleaseIPs {
		rc.cnscli.IPStateManager.RemovePendingReleaseIPConfigs(rc.NNC.Spec.IPsNotInUse)
	}

	// update
	rc.cnscli.PoolMonitor.Update(rc.NNC)
	return nil
}

func incrementIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}
