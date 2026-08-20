package network

import (
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/iptables"
	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/platform"
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

	Describe("Test Add and Delete Endpoint Linux", func() {
		nw2 := &network{
			Endpoints: map[string]*endpoint{},
			Mode:      opModeBridge,
			extIf:     &externalInterface{IPv4Gateway: net.ParseIP("192.168.0.1")},
		}
		_, dummyIPNet, _ := net.ParseCIDR("10.0.0.0/24")
		epInfo2 := &EndpointInfo{
			EndpointID:  "768e8deb-eth1",
			Data:        make(map[string]interface{}),
			IfName:      eth0IfName,
			NICType:     cns.InfraNIC,
			ContainerID: "0ea7476f26d192f067abdc8b3df43ce3cdbe324386e1c010cb48de87eefef480",
			Mode:        opModeTransparent,
			IPAddresses: []net.IPNet{*dummyIPNet},
		}
		It("Should ignore the network struct network mode and use the epInfo network mode during add", func() {
			// check that we select the transparent endpoint based on epINfo, in which case the below command is run
			transparentRun := false
			checkTransparentRun := func(cmd string) (string, error) {
				if cmd == "echo 1 > /proc/sys/net/ipv4/conf/azv768e8de/proxy_arp" {
					transparentRun = true
				}
				return "", nil
			}
			pl := platform.NewMockExecClient(false)
			pl.SetExecRawCommand(checkTransparentRun)

			ep, err := nw2.newEndpointImpl(nil, netlink.NewMockNetlink(false, ""), pl,
				netio.NewMockNetIO(false, 0), nil, NewMockNamespaceClient(), iptables.NewClient(), &mockDHCP{}, epInfo2)
			Expect(err).NotTo(HaveOccurred())
			Expect(ep).NotTo(BeNil())
			Expect(ep.Id).To(Equal(epInfo2.EndpointID))
			Expect(transparentRun).To(BeTrue())
		})
		It("Should use the passed in mode during delete", func() {
			// check that we select the transparent endpoint based on epInfo, in which case we remove routes
			transparentRun := false
			checkTransparentRun := func(_ *netlink.Route) error {
				transparentRun = true
				return nil
			}
			nl := netlink.NewMockNetlink(false, "")
			nl.SetDeleteRouteValidationFn(checkTransparentRun)

			ep2, err := nw2.newEndpointImpl(nil, netlink.NewMockNetlink(false, ""), platform.NewMockExecClient(false),
				netio.NewMockNetIO(false, 0), nil, NewMockNamespaceClient(), iptables.NewClient(), &mockDHCP{}, epInfo2)
			Expect(err).ToNot(HaveOccurred())
			Expect(ep2).ToNot(BeNil())
			// Deleting the endpoint
			//nolint:errcheck // ignore error
			nw2.deleteEndpointImpl(nl, platform.NewMockExecClient(false), nil, netio.NewMockNetIO(false, 0),
				NewMockNamespaceClient(), iptables.NewClient(), &mockDHCP{}, ep2, opModeTransparent)
			Expect(transparentRun).To(BeTrue())
		})
	})
})
