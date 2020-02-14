// Copyright 2019 Microsoft. All rights reserved.
// MIT License

package network

import (
	"github.com/Azure/azure-container-networking/platform"
	//"github.com/Azure/azure-container-networking/common"
	//"github.com/Azure/azure-container-networking/platform"
	//"github.com/Azure/azure-container-networking/store"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"net"

	//"net"
	"testing"
)


func TestEndpoint(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Network Suite")
}

var (
	_ = Describe("Test Endpoint", func() {

		var (
			nw *network
		)

		Describe("", func() {
			Context("", func() {
				It("", func() {
					testAddExternalInterface("10.0.0.0/16")

					nwId := "test01"
					ns := "ns01"
					bridgeName := "bridge0"

					nwInfo := NetworkInfo{
						Id:           nwId,
						Mode:         "bridge",
						MasterIfName: ifName,
						Subnets: []SubnetInfo{
							{
								Family:  platform.AfINET,
								Prefix:  net.IPNet{IP:net.IPv4(10,0,0,0), Mask:net.IPv4Mask(255,255,0,0)},
								Gateway: net.IPv4(10,0,0,1),
							},
						},
						BridgeName:                    bridgeName,
						EnableSnatOnHost:              false,
						//DNS:                           nil,
						//Policies:                      nil,
						NetNs:                         ns,
						DisableHairpinOnHostInterface: true,
					}

					nwInfo.Options = make(map[string]interface{})

					testCreateNetwork(nwInfo)

					nw, err = nm.getNetwork(nwId)
					Expect(err).NotTo(HaveOccurred())
					Expect(nw.Id).To(Equal(nwInfo.Id))
					
					err = nm.DeleteNetwork(nwId)
					Expect(err).NotTo(HaveOccurred())
					_, ok = nm.ExternalInterfaces[ifName].Networks[nwId]
					Expect(ok).To(Equal(false))
				})
			})
		})

		Describe("Test GetPodName", func() {
			Context("GetPodName", func() {
				It("GetPodName successfully", func() {

					testData := map[string]string{
						"nginx-deployment-5c689d88bb":       "nginx",
						"nginx-deployment-5c689d88bb-qwq47": "nginx-deployment",
						"nginx": "nginx",
					}

					for testValue, expectedPodName := range testData {
						podName := GetPodNameWithoutSuffix(testValue)
						Expect(podName).To(Equal(expectedPodName))
					}
				})
			})
		})
	})
)
