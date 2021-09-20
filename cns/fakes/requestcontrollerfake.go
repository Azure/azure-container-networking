//go:build !ignore_uncovered
// +build !ignore_uncovered

package fakes

import (
	"context"
	"net"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/singletenantcontroller"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/google/uuid"
)

var _ singletenantcontroller.RequestController = (*RequestControllerFake)(nil)

type RequestControllerFake struct {
	fakecns   *HTTPServiceFake
	cachedCRD v1alpha.NodeNetworkConfig
	ip        net.IP
}

func NewRequestControllerFake(cnsService *HTTPServiceFake, scalar v1alpha.Scaler, subnetAddressSpace string, numberOfIPConfigs int) *RequestControllerFake {
	rc := &RequestControllerFake{
		fakecns: cnsService,
		cachedCRD: v1alpha.NodeNetworkConfig{
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
	rc.cachedCRD.Spec.RequestedIPCount = int64(numberOfIPConfigs)

	return rc
}

func (rc *RequestControllerFake) CarveIPConfigsAndAddToStatusAndCNS(numberOfIPConfigs int) []cns.IPConfigurationStatus {
	var cnsIPConfigs []cns.IPConfigurationStatus
	for i := 0; i < numberOfIPConfigs; i++ {

		ipconfigCRD := v1alpha.IPAssignment{
			Name: uuid.New().String(),
			IP:   rc.ip.String(),
		}
		rc.cachedCRD.Status.NetworkContainers[0].IPAssignments = append(rc.cachedCRD.Status.NetworkContainers[0].IPAssignments, ipconfigCRD)

		ipconfigCNS := cns.IPConfigurationStatus{
			ID:        ipconfigCRD.Name,
			IPAddress: ipconfigCRD.IP,
			State:     cns.Available,
		}
		cnsIPConfigs = append(cnsIPConfigs, ipconfigCNS)

		incrementIP(rc.ip)
	}

	rc.fakecns.IPStateManager.AddIPConfigs(cnsIPConfigs)

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

func (rc *RequestControllerFake) UpdateCRDSpec(_ context.Context, desiredSpec v1alpha.NodeNetworkConfigSpec) error {
	rc.cachedCRD.Spec = desiredSpec
	return nil
}

func remove(slice []v1alpha.IPAssignment, s int) []v1alpha.IPAssignment {
	return append(slice[:s], slice[s+1:]...)
}

func (rc *RequestControllerFake) Reconcile(removePendingReleaseIPs bool) error {
	diff := int(rc.cachedCRD.Spec.RequestedIPCount) - len(rc.fakecns.GetPodIPConfigState())

	if diff > 0 {
		// carve the difference of test IPs and add them to CNS, assume dnc has populated the CRD status
		rc.CarveIPConfigsAndAddToStatusAndCNS(diff)
	} else if diff < 0 {
		// Assume DNC has removed the IPConfigs from the status

		// mimic DNC removing IPConfigs from the CRD
		for _, notInUseIPConfigName := range rc.cachedCRD.Spec.IPsNotInUse {

			// remove ipconfig from status
			index := 0
			for _, ipconfig := range rc.cachedCRD.Status.NetworkContainers[0].IPAssignments {
				if notInUseIPConfigName == ipconfig.Name {
					break
				}
				index++
			}
			rc.cachedCRD.Status.NetworkContainers[0].IPAssignments = remove(rc.cachedCRD.Status.NetworkContainers[0].IPAssignments, index)

		}
	}

	// remove ipconfig from CNS
	if removePendingReleaseIPs {
		rc.fakecns.IPStateManager.RemovePendingReleaseIPConfigs(rc.cachedCRD.Spec.IPsNotInUse)
	}

	// update
	rc.fakecns.PoolMonitor.Update(rc.cachedCRD.Status.Scaler, rc.cachedCRD.Spec)

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
