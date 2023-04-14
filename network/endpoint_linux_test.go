package network

import (
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

func TestEndpointLinux(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Endpoint Suite")
}

var _ = Describe("Test TestEndpointLinux", func() {
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
				//nolint:goerr113 // for testing
				return errors.New("Cannot add route")
			})

			netiocl := netio.NewMockNetIO(false, 0)
			netiocl.SetGetInterfaceValidatonFn(func(ifName string) (*net.Interface, error) {
				Expect(ifName).To(Equal(""))
				return &net.Interface{
					Index: 0,
				}, errors.Wrapf(netio.ErrInterfaceNil, "Cannot get interface")
			})

			err := addRoutes(nlc, netiocl, "", []RouteInfo{{Dst: *dst, DevName: ""}})
			Expect(err).ToNot(BeNil())
		})
	})
})
