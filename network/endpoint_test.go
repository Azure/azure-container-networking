// Copyright 2019 Microsoft. All rights reserved.
// MIT License

package network

import (
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/platform"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
)

func TestEndpoint(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Endpoint Suite")
}

var _ = Describe("Test Endpoint", func() {
	Describe("Test getEndpoint", func() {
		Context("When endpoint not exists", func() {
			It("Should raise errEndpointNotFound", func() {
				nw := &network{
					Endpoints: map[string]*endpoint{},
				}
				ep, err := nw.getEndpoint("invalid")
				Expect(err).To(Equal(errEndpointNotFound))
				Expect(ep).To(BeNil())
			})
		})

		Context("When endpoint exists", func() {
			It("Should return endpoint with no err", func() {
				epId := "epId"
				nw := &network{
					Endpoints: map[string]*endpoint{},
				}
				nw.Endpoints[epId] = &endpoint{
					Id: epId,
				}
				ep, err := nw.getEndpoint(epId)
				Expect(err).NotTo(HaveOccurred())
				Expect(ep.Id).To(Equal(epId))
			})
		})
	})

	Describe("Test getEndpointByPOD", func() {
		Context("When multiple endpoints found", func() {
			It("Should raise errMultipleEndpointsFound", func() {
				podName := "test"
				podNS := "ns"
				nw := &network{
					Endpoints: map[string]*endpoint{},
				}
				nw.Endpoints["pod1"] = &endpoint{
					PODName:      podName,
					PODNameSpace: podNS,
				}
				nw.Endpoints["pod2"] = &endpoint{
					PODName:      podName,
					PODNameSpace: podNS,
				}
				ep, err := nw.getEndpointByPOD(podName, podNS, true)
				Expect(err).To(Equal(errMultipleEndpointsFound))
				Expect(ep).To(BeNil())
			})
		})

		Context("When endpoint not found", func() {
			It("Should raise errEndpointNotFound", func() {
				nw := &network{
					Endpoints: map[string]*endpoint{},
				}
				ep, err := nw.getEndpointByPOD("invalid", "", false)
				Expect(err).To(Equal(errEndpointNotFound))
				Expect(ep).To(BeNil())
			})
		})

		Context("When one endpoint found", func() {
			It("Should return endpoint", func() {
				podName := "test"
				podNS := "ns"
				nw := &network{
					Endpoints: map[string]*endpoint{},
				}
				nw.Endpoints["pod"] = &endpoint{
					PODName:      podName,
					PODNameSpace: podNS,
				}
				ep, err := nw.getEndpointByPOD(podName, podNS, true)
				Expect(err).NotTo(HaveOccurred())
				Expect(ep.PODName).To(Equal(podName))
			})
		})
	})

	Describe("Test podNameMatches", func() {
		Context("When doExactMatch flag is set", func() {
			It("Should exact match", func() {
				actual := "nginx"
				valid := "nginx"
				invalid := "nginx-deployment-5c689d88bb"
				Expect(podNameMatches(valid, actual, true)).To(BeTrue())
				Expect(podNameMatches(invalid, actual, true)).To(BeFalse())
			})
		})

		Context("When doExactMatch flag is not set", func() {
			It("Should not exact match", func() {
				actual := "nginx"
				valid1 := "nginx"
				valid2 := "nginx-deployment-5c689d88bb"
				invalid := "nginx-deployment-5c689d88bb-qwq47"
				Expect(podNameMatches(valid1, actual, false)).To(BeTrue())
				Expect(podNameMatches(valid2, actual, false)).To(BeTrue())
				Expect(podNameMatches(invalid, actual, false)).To(BeFalse())
			})
		})
	})

	Describe("Test attach", func() {
		Context("When SandboxKey in use", func() {
			It("Should raise errEndpointInUse", func() {
				ep := &endpoint{
					SandboxKey: "key",
				}
				err := ep.attach("")
				Expect(err).To(Equal(errEndpointInUse))
			})
		})

		Context("When SandboxKey not in use", func() {
			It("Should set SandboxKey", func() {
				sandboxKey := "key"
				ep := &endpoint{}
				err := ep.attach(sandboxKey)
				Expect(err).NotTo(HaveOccurred())
				Expect(ep.SandboxKey).To(Equal(sandboxKey))
			})
		})
	})

	Describe("Test detach", func() {
		Context("When SandboxKey not in use", func() {
			It("Should raise errEndpointNotInUse", func() {
				ep := &endpoint{}
				err := ep.detach()
				Expect(err).To(Equal(errEndpointNotInUse))
			})
		})

		Context("When SandboxKey in use", func() {
			It("Should set SandboxKey empty", func() {
				ep := &endpoint{
					SandboxKey: "key",
				}
				err := ep.detach()
				Expect(err).NotTo(HaveOccurred())
				Expect(ep.SandboxKey).To(BeEmpty())
			})
		})
	})

	Describe("Test endpointImpl", func() {
		Context("When endpoint add/delete succeed", func() {
			nw := &network{
				Endpoints: map[string]*endpoint{},
			}
			epInfo := &EndpointInfo{
				Id:   "768e8deb-eth1",
				Data: make(map[string]interface{}),
			}
			epInfo.Data[VlanIDKey] = 100

			It("Should be added", func() {
				// Add endpoint with valid id
				ep, err := nw.newEndpointImpl(nil, netlink.NewMockNetlink(false, ""), platform.NewMockExecClient(false),
					netio.NewMockNetIO(false, 0), NewMockEndpointClient(false), epInfo)
				Expect(err).NotTo(HaveOccurred())
				Expect(ep).NotTo(BeNil())
				Expect(ep.Id).To(Equal(epInfo.Id))
				Expect(ep.Gateways).To(BeEmpty())
			})
			It("should have fields set", func() {
				nw2 := &network{
					Endpoints: map[string]*endpoint{},
					extIf:     &externalInterface{IPv4Gateway: net.ParseIP("192.168.0.1")},
				}
				ep, err := nw2.newEndpointImpl(nil, netlink.NewMockNetlink(false, ""), platform.NewMockExecClient(false),
					netio.NewMockNetIO(false, 0), NewMockEndpointClient(false), epInfo)
				Expect(err).NotTo(HaveOccurred())
				Expect(ep).NotTo(BeNil())
				Expect(ep.Id).To(Equal(epInfo.Id))
				Expect(ep.Gateways).NotTo(BeNil())
				Expect(len(ep.Gateways)).To(Equal(1))
				Expect(ep.Gateways[0].String()).To(Equal("192.168.0.1"))
				Expect(ep.VlanID).To(Equal(epInfo.Data[VlanIDKey].(int)))
			})
			It("Should be not added", func() {
				// Adding an endpoint with an id.
				mockCli := NewMockEndpointClient(false)
				err := mockCli.AddEndpoints(epInfo)
				Expect(err).ToNot(HaveOccurred())
				// Adding endpoint with same id should fail and delete should cleanup the state
				ep2, err := nw.newEndpointImpl(nil, netlink.NewMockNetlink(false, ""), platform.NewMockExecClient(false),
					netio.NewMockNetIO(false, 0), mockCli, epInfo)
				Expect(err).To(HaveOccurred())
				Expect(ep2).To(BeNil())
				assert.Contains(GinkgoT(), err.Error(), "Endpoint already exists")
				Expect(len(mockCli.endpoints)).To(Equal(0))
			})
			It("Should be deleted", func() {
				// Adding an endpoint with an id.
				mockCli := NewMockEndpointClient(false)
				ep2, err := nw.newEndpointImpl(nil, netlink.NewMockNetlink(false, ""), platform.NewMockExecClient(false),
					netio.NewMockNetIO(false, 0), mockCli, epInfo)
				Expect(err).ToNot(HaveOccurred())
				Expect(ep2).ToNot(BeNil())
				Expect(len(mockCli.endpoints)).To(Equal(1))
				// Deleting the endpoint
				//nolint:errcheck // ignore error
				nw.deleteEndpointImpl(netlink.NewMockNetlink(false, ""), platform.NewMockExecClient(false), mockCli, ep2)
				Expect(len(mockCli.endpoints)).To(Equal(0))
				// Deleting same endpoint with same id should not fail
				//nolint:errcheck // ignore error
				nw.deleteEndpointImpl(netlink.NewMockNetlink(false, ""), platform.NewMockExecClient(false), mockCli, ep2)
				Expect(len(mockCli.endpoints)).To(Equal(0))
			})
		})
		Context("When endpoint add failed", func() {
			It("Should not be added to the network", func() {
				nw := &network{
					Endpoints: map[string]*endpoint{},
				}
				epInfo := &EndpointInfo{
					Id: "768e8deb-eth1",
				}
				ep, err := nw.newEndpointImpl(nil, netlink.NewMockNetlink(false, ""), platform.NewMockExecClient(false),
					netio.NewMockNetIO(false, 0), NewMockEndpointClient(true), epInfo)
				Expect(err).To(HaveOccurred())
				Expect(ep).To(BeNil())
				ep, err = nw.newEndpointImpl(nil, netlink.NewMockNetlink(false, ""), platform.NewMockExecClient(false),
					netio.NewMockNetIO(false, 0), NewMockEndpointClient(false), epInfo)
				Expect(err).NotTo(HaveOccurred())
				Expect(ep).NotTo(BeNil())
				Expect(ep.Id).To(Equal(epInfo.Id))
			})
		})
	})

	Describe("Test updateEndpoint", func() {
		Context("When endpoint not found", func() {
			It("Should raise errEndpointNotFound", func() {
				nm := &networkManager{}

				nw := &network{}
				existingEpInfo := &EndpointInfo{
					Id: "768e8deb-eth1",
				}
				targetEpInfo := &EndpointInfo{}
				err := nm.updateEndpoint(nw, existingEpInfo, targetEpInfo)
				Expect(err).To(Equal(errEndpointNotFound))
			})
		})
	})

	Describe("Test GetPodNameWithoutSuffix", func() {
		Context("When podnames have suffix or not", func() {
			It("Should return podname without suffix", func() {
				testData := map[string]string{
					"nginx-deployment-5c689d88bb":       "nginx",
					"nginx-deployment-5c689d88bb-qwq47": "nginx-deployment",
					"nginx":                             "nginx",
				}
				for testValue, expectedPodName := range testData {
					podName := GetPodNameWithoutSuffix(testValue)
					Expect(podName).To(Equal(expectedPodName))
				}
			})
		})
	})
})
