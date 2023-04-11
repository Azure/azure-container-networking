// Copyright 2019 Microsoft. All rights reserved.
// MIT License

package network

import (
	"errors"
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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

	Describe("Test updateEndpoint", func() {
		Context("When endpoint not found", func() {
			It("Should raise errEndpointNotFound", func() {
				nm := &networkManager{}

				nw := &network{}
				existingEpInfo := &EndpointInfo{
					Id: "test",
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
	Describe("Test deleteRoutes", func() {
		_, dst, _ := net.ParseCIDR("192.168.0.0/16")

		It("DeleteRoute with interfacename explicit", func() {
			nlc := netlink.NewMockNetlink(false, "")
			nlc.SetDeleteRouteValidationFn(func(r *netlink.Route) error {
				Expect(r.LinkIndex).To(Equal(5))
				return nil
			})

			netiocl := netio.NewMockNetIO(false, 0)
			netiocl.SetGetInterfaceValidatonFn(func(ifName string) (*net.Interface, error) {
				Expect(ifName).To(Equal("eth0"))
				return &net.Interface{
					Index: 5,
				}, nil
			})

			err := deleteRoutes(nlc, netiocl, "eth0", []RouteInfo{{Dst: *dst, DevName: ""}})
			Expect(err).To(BeNil())
		})
		It("DeleteRoute with interfacename set in Route", func() {
			nlc := netlink.NewMockNetlink(false, "")
			nlc.SetDeleteRouteValidationFn(func(r *netlink.Route) error {
				Expect(r.LinkIndex).To(Equal(6))
				return nil
			})

			netiocl := netio.NewMockNetIO(false, 0)
			netiocl.SetGetInterfaceValidatonFn(func(ifName string) (*net.Interface, error) {
				Expect(ifName).To(Equal("eth1"))
				return &net.Interface{
					Index: 6,
				}, nil
			})

			err := deleteRoutes(nlc, netiocl, "", []RouteInfo{{Dst: *dst, DevName: "eth1"}})
			Expect(err).To(BeNil())
		})
		It("DeleteRoute with no ifindex", func() {
			nlc := netlink.NewMockNetlink(false, "")
			nlc.SetDeleteRouteValidationFn(func(r *netlink.Route) error {
				Expect(r.LinkIndex).To(Equal(0))
				return nil
			})

			netiocl := netio.NewMockNetIO(false, 0)
			netiocl.SetGetInterfaceValidatonFn(func(ifName string) (*net.Interface, error) {
				Expect(ifName).To(Equal("eth1"))
				return &net.Interface{
					Index: 6,
				}, nil
			})

			err := deleteRoutes(nlc, netiocl, "", []RouteInfo{{Dst: *dst, DevName: ""}})
			Expect(err).To(BeNil())
		})
	})
	Describe("Test addRoutes", func() {
		_, dst, _ := net.ParseCIDR("192.168.0.0/16")
		It("AddRoute with interfacename explicit", func() {
			nlc := netlink.NewMockNetlink(false, "")
			nlc.SetAddRouteValidationFn(func(r *netlink.Route) error {
				Expect(r).NotTo(BeNil())
				Expect(r.LinkIndex).To(Equal(5))
				return nil
			})

			netiocl := netio.NewMockNetIO(false, 0)
			netiocl.SetGetInterfaceValidatonFn(func(ifName string) (*net.Interface, error) {
				Expect(ifName).To(Equal("eth0"))
				return &net.Interface{
					Index: 5,
				}, nil
			})

			err := addRoutes(nlc, netiocl, "eth0", []RouteInfo{{Dst: *dst, DevName: ""}})
			Expect(err).To(BeNil())
		})
		It("AddRoute with interfacename set in route", func() {
			nlc := netlink.NewMockNetlink(false, "")
			nlc.SetAddRouteValidationFn(func(r *netlink.Route) error {
				Expect(r.LinkIndex).To(Equal(6))
				return nil
			})

			netiocl := netio.NewMockNetIO(false, 0)
			netiocl.SetGetInterfaceValidatonFn(func(ifName string) (*net.Interface, error) {
				Expect(ifName).To(Equal("eth1"))
				return &net.Interface{
					Index: 6,
				}, nil
			})

			err := addRoutes(nlc, netiocl, "", []RouteInfo{{Dst: *dst, DevName: "eth1"}})
			Expect(err).To(BeNil())
		})
		It("AddRoute with interfacename not set should return error", func() {
			nlc := netlink.NewMockNetlink(false, "")
			nlc.SetAddRouteValidationFn(func(r *netlink.Route) error {
				Expect(r.LinkIndex).To(Equal(0))
				return errors.New("Cannot add route")
			})

			netiocl := netio.NewMockNetIO(false, 0)
			netiocl.SetGetInterfaceValidatonFn(func(ifName string) (*net.Interface, error) {
				Expect(ifName).To(Equal(""))
				return &net.Interface{
					Index: 0,
				}, errors.New("interface not found")
			})

			err := addRoutes(nlc, netiocl, "", []RouteInfo{{Dst: *dst, DevName: ""}})
			Expect(err).ToNot(BeNil())
		})
	})
})
