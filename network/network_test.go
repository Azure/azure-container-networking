package network

import (
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Azure/azure-container-networking/store"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"net"
	"testing"
)

func TestNetwork(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Network Suite")
}

func getIfName() (string, error) {

	ifName := ""

	interfaces, err := net.Interfaces()
	if err != nil {
		return ifName, err
	}

	for _, iface := range interfaces {
		ifName = iface.Name
		break
	}
	ifName = "eth0"
	return ifName, err
}

var (
	_ = Describe("Test Network", func() {

		var (
			nm *networkManager
			config common.PluginConfig
			ifName string
			err error
		)

		BeforeSuite(func() {
			//nm, err = NewNetworkManager()
			nm = &networkManager{
				ExternalInterfaces: make(map[string]*externalInterface),
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(nm).NotTo(BeNil())
			storeFileName := "./testfiles/store.json"
			config.Store, err = store.NewJsonFileStore(storeFileName)
			err = nm.Initialize(&config)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterSuite(func() {
			nm.Uninitialize()
		})

		Context("", func() {
			It("Add External Interface", func() {
				ifName, err = getIfName()
				Expect(err).NotTo(HaveOccurred())
				err = nm.AddExternalInterface(ifName, "10.0.0.0/16")
				Expect(err).NotTo(HaveOccurred())
				Expect(len(nm.ExternalInterfaces)).To(Equal(1))

				nwkId := "test01"

				nwInfo := NetworkInfo{
					Id:           nwkId,
					Mode:         "bridge",
					MasterIfName: ifName,
					Subnets: []SubnetInfo{
						{
							Family:  platform.AfINET,
							Prefix:  net.IPNet{IP:net.IPv4(10,0,0,0), Mask:net.IPv4Mask(255,255,0,0)},
							Gateway: net.IPv4(10,0,0,1),
						},
					},
					BridgeName:                    "bridge0",
					EnableSnatOnHost:              false,
					//DNS:                           nil,
					//Policies:                      nil,
					NetNs:                         "args.Netns",
					DisableHairpinOnHostInterface: true,
				}

				nwInfo.Options = make(map[string]interface{})

				err = nm.CreateNetwork(&nwInfo)
				Expect(err).NotTo(HaveOccurred())
				_, ok := nm.ExternalInterfaces[ifName].Networks[nwkId]
				Expect(ok).To(Equal(true))

				nwInfoGet, err := nm.GetNetworkInfo(nwkId)
				Expect(err).NotTo(HaveOccurred())
				Expect(nwInfoGet.BridgeName).To(Equal("bridge0"))

				err = nm.DeleteNetwork(nwkId)
				Expect(err).NotTo(HaveOccurred())
				_, ok = nm.ExternalInterfaces[ifName].Networks[nwkId]
				Expect(ok).To(Equal(false))

				nwInfoGet, err = nm.GetNetworkInfo(nwkId)
				Expect(err).To(HaveOccurred())
				Expect(nwInfoGet).To(BeNil())
			})
		})
	})
)