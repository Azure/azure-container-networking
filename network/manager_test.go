package network

import (
	"errors"
	"net"
	"sort"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/store"
	"github.com/Azure/azure-container-networking/testutils"
)

func TestManager(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Manager Suite")
}

var _ = Describe("Test Manager", func() {
	Describe("Test deleteExternalInterface", func() {
		Context("When external interface not found", func() {
			It("Should return nil", func() {
				ifName := "eth0"
				nm := &networkManager{
					ExternalInterfaces: map[string]*externalInterface{},
				}
				err := nm.deleteExternalInterface(ifName)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("When external interface found", func() {
			It("Should delete external interface", func() {
				ifName := "eth0"
				nm := &networkManager{
					ExternalInterfaces: map[string]*externalInterface{},
				}
				nm.ExternalInterfaces[ifName] = &externalInterface{}
				err := nm.deleteExternalInterface(ifName)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("Test restore", func() {
		Context("When restore is nil", func() {
			It("Should return nil", func() {
				nm := &networkManager{}
				err := nm.restore(false)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("When store.Read return ErrKeyNotFound", func() {
			It("Should return nil", func() {
				nm := &networkManager{
					store: &testutils.KeyValueStoreMock{
						ReadError: store.ErrKeyNotFound,
					},
				}
				err := nm.restore(false)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("When store.Read return error", func() {
			It("Should raise error", func() {
				nm := &networkManager{
					store: &testutils.KeyValueStoreMock{
						ReadError: errors.New("error for test"),
					},
				}
				err := nm.restore(false)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("When GetModificationTime error and not rebooted", func() {
			It("Should populate pointers", func() {
				extIfName := "eth0"
				nwId := "nwId"
				nm := &networkManager{
					store: &testutils.KeyValueStoreMock{
						GetModificationTimeError: errors.New("error for test"),
					},
					ExternalInterfaces: map[string]*externalInterface{},
				}
				nm.ExternalInterfaces[extIfName] = &externalInterface{
					Name:     extIfName,
					Networks: map[string]*network{},
				}
				nm.ExternalInterfaces[extIfName].Networks[nwId] = &network{}
				err := nm.restore(false)
				Expect(err).NotTo(HaveOccurred())
				Expect(nm.ExternalInterfaces[extIfName].Networks[nwId].extIf.Name).To(Equal(extIfName))
			})
		})
	})

	Describe("Test save", func() {
		Context("When store is nil", func() {
			It("Should return nil", func() {
				nm := &networkManager{}
				err := nm.save()
				Expect(err).NotTo(HaveOccurred())
				Expect(nm.TimeStamp).To(Equal(time.Time{}))
			})
		})
		Context("When store.Write return error", func() {
			It("Should raise error", func() {
				nm := &networkManager{
					store: &testutils.KeyValueStoreMock{
						WriteError: errors.New("error for test"),
					},
				}
				err := nm.save()
				Expect(err).To(HaveOccurred())
				Expect(nm.TimeStamp).NotTo(Equal(time.Time{}))
			})
		})
	})

	Describe("Test GetNumberOfEndpoints", func() {
		Context("When ExternalInterfaces is nil", func() {
			It("Should return 0", func() {
				nm := &networkManager{}
				num := nm.GetNumberOfEndpoints("", "")
				Expect(num).To(Equal(0))
			})
		})

		Context("When extIf not found", func() {
			It("Should return 0", func() {
				nm := &networkManager{
					ExternalInterfaces: map[string]*externalInterface{},
				}
				num := nm.GetNumberOfEndpoints("eth0", "")
				Expect(num).To(Equal(0))
			})
		})

		Context("When Networks is nil", func() {
			It("Should return 0", func() {
				ifName := "eth0"
				nm := &networkManager{
					ExternalInterfaces: map[string]*externalInterface{},
				}
				nm.ExternalInterfaces[ifName] = &externalInterface{}
				num := nm.GetNumberOfEndpoints(ifName, "")
				Expect(num).To(Equal(0))
			})
		})

		Context("When network not found", func() {
			It("Should return 0", func() {
				ifName := "eth0"
				nm := &networkManager{
					ExternalInterfaces: map[string]*externalInterface{},
				}
				nm.ExternalInterfaces[ifName] = &externalInterface{
					Networks: map[string]*network{},
				}
				num := nm.GetNumberOfEndpoints(ifName, "nwId")
				Expect(num).To(Equal(0))
			})
		})

		Context("When endpoints is nil", func() {
			It("Should return 0", func() {
				ifName := "eth0"
				nwId := "nwId"
				nm := &networkManager{
					ExternalInterfaces: map[string]*externalInterface{},
				}
				nm.ExternalInterfaces[ifName] = &externalInterface{
					Networks: map[string]*network{},
				}
				nm.ExternalInterfaces[ifName].Networks[nwId] = &network{}
				num := nm.GetNumberOfEndpoints(ifName, nwId)
				Expect(num).To(Equal(0))
			})
		})

		Context("When endpoints is found", func() {
			It("Should return the length of endpoints", func() {
				ifName := "eth0"
				nwId := "nwId"
				nm := &networkManager{
					ExternalInterfaces: map[string]*externalInterface{},
				}
				nm.ExternalInterfaces[ifName] = &externalInterface{
					Networks: map[string]*network{},
				}
				nm.ExternalInterfaces[ifName].Networks[nwId] = &network{
					Endpoints: map[string]*endpoint{
						"ep1": {},
						"ep2": {},
						"ep3": {},
					},
				}
				num := nm.GetNumberOfEndpoints(ifName, nwId)
				Expect(num).To(Equal(3))
			})
		})

		Context("When ifName not specifed in GetNumberofEndpoints", func() {
			It("Should range the nm.ExternalInterfaces", func() {
				ifName := "eth0"
				nwId := "nwId"
				nm := &networkManager{
					ExternalInterfaces: map[string]*externalInterface{},
				}
				nm.ExternalInterfaces[ifName] = &externalInterface{
					Networks: map[string]*network{},
				}
				nm.ExternalInterfaces[ifName].Networks[nwId] = &network{
					Endpoints: map[string]*endpoint{
						"ep1": {},
						"ep2": {},
						"ep3": {},
					},
				}
				num := nm.GetNumberOfEndpoints("", nwId)
				Expect(num).To(Equal(3))
			})
		})

		Context("When different fields passed to update endpoint state", func() {
			It("Should error or validate correctly", func() {
				nm := &networkManager{}

				err := nm.UpdateEndpointState([]*endpoint{
					{
						IfName:      "eth0",
						ContainerID: "2bfc3b23e078f0bea48612d5d081ace587599cdac026d23e4d57bd03c85d357c",
					},
					{
						IfName:      "",
						ContainerID: "2bfc3b23e078f0bea48612d5d081ace587599cdac026d23e4d57bd03c85d357c",
					},
				})
				Expect(err).To(HaveOccurred())

				err = nm.UpdateEndpointState([]*endpoint{
					{
						IfName:      "eth1",
						ContainerID: "",
					},
					{
						IfName:      "eth0",
						ContainerID: "",
					},
				})
				Expect(err).To(HaveOccurred())

				err = validateUpdateEndpointState(
					"2bfc3b23e078f0bea48612d5d081ace587599cdac026d23e4d57bd03c85d357c",
					map[string]*restserver.IPInfo{
						"eth1": {},
						"eth2": {},
					})
				Expect(err).To(BeNil())
			})
		})
	})
	Describe("Test EndpointCreate", func() {
		Context("When no endpoints provided", func() {
			It("Should return 0", func() {
				nm := &networkManager{}
				err := nm.EndpointCreate(nil, []*EndpointInfo{})
				Expect(err).NotTo(HaveOccurred())
				num := nm.GetNumberOfEndpoints("", "")
				Expect(num).To(Equal(0))
			})
		})
	})
	Describe("Test GetEndpointInfosFromContainerID", func() {
		Context("When getting containers based on container id regardless of network", func() {
			It("Should return 0", func() {
				nm := &networkManager{
					ExternalInterfaces: map[string]*externalInterface{
						"eth0": {
							Networks: map[string]*network{
								"azure": {
									Endpoints: map[string]*endpoint{
										"12345678-eth0": {
											Id:          "12345678-eth0",
											ContainerID: "12345678",
											// potentially empty nictype
										},
										"abcdefgh-eth0": {
											Id:          "abcdefgh-eth0",
											ContainerID: "abcdefgh",
										},
									},
								},
								"other": {
									Endpoints: map[string]*endpoint{
										"12345678-1": {
											Id:          "12345678-1",
											ContainerID: "12345678",
											NICType:     cns.NodeNetworkInterfaceFrontendNIC,
										},
									},
								},
							},
						},
					},
				}
				epInfos := nm.GetEndpointInfosFromContainerID("12345678")
				sort.Slice(epInfos, func(i, j int) bool {
					return epInfos[i].EndpointID < epInfos[j].EndpointID
				})
				Expect(len(epInfos)).To(Equal(2))

				Expect(epInfos[0].EndpointID).To(Equal("12345678-1"))
				Expect(epInfos[0].NICType).To(Equal(cns.NodeNetworkInterfaceFrontendNIC))
				Expect(epInfos[0].NetworkID).To(Equal("other"))

				Expect(epInfos[1].EndpointID).To(Equal("12345678-eth0"))
				Expect(string(epInfos[1].NICType)).To(Equal(""))
				Expect(epInfos[1].NetworkID).To(Equal("azure"))
			})
		})
	})
	Describe("Test stateless cnsEndpointInfotoCNIEpInfos", func() {
		endpointID := ""
		_, dummyIP, _ := net.ParseCIDR("192.0.2.1/24")
		dummyIPv4Slice := []net.IPNet{
			*dummyIP,
		}
		Context("When converting from cns to cni unmigrated", func() {
			It("Should get the right cni endpoint info data", func() {
				cnsEndpointInfo := restserver.EndpointInfo{
					IfnameToIPMap: map[string]*restserver.IPInfo{
						"": {
							IPv4:          dummyIPv4Slice,
							HostVethName:  "hostVeth1",
							HnsEndpointID: "hnsID1",
							HnsNetworkID:  "hnsNetworkID1",
							MacAddress:    "12:34:56:78:9a:bc",
						},
					},
					PodName:      "test-pod",
					PodNamespace: "test-pod-ns",
				}

				epInfos := cnsEndpointInfotoCNIEpInfos(cnsEndpointInfo, endpointID, "")

				Expect(len(epInfos)).To(Equal(1))
				Expect(epInfos[0]).To(Equal(
					&EndpointInfo{
						IPAddresses:        dummyIPv4Slice,
						IfName:             InfraInterfaceName,
						HostIfName:         "hostVeth1",
						HNSEndpointID:      "hnsID1",
						NICType:            cns.InfraNIC,
						HNSNetworkID:       "hnsNetworkID1",
						MacAddress:         net.HardwareAddr("12:34:56:78:9a:bc"),
						ContainerID:        endpointID,
						EndpointID:         endpointID,
						NetworkContainerID: endpointID,
						PODName:            "test-pod",
						PODNameSpace:       "test-pod-ns",
					},
				), "empty infos received from cns should be auto populated and treated as infra")
			})
		})
		Context("When converting from cns to cni migrated", func() {
			_, dummyIP2, _ := net.ParseCIDR("193.0.2.1/24")
			dummyIPv4Slice2 := []net.IPNet{
				*dummyIP2,
			}
			It("Should get the right cni endpoint info data if there are multiple ip infos", func() {
				cnsEndpointInfo := restserver.EndpointInfo{
					IfnameToIPMap: map[string]*restserver.IPInfo{
						"ifName1": {
							IPv4:          dummyIPv4Slice,
							HostVethName:  "hostVeth1",
							HnsEndpointID: "hnsID1",
							HnsNetworkID:  "hnsNetworkID1",
							MacAddress:    "12:34:56:78:9a:bc",
							NICType:       cns.InfraNIC,
						},
						"ifName2": {
							IPv4:          dummyIPv4Slice2,
							HostVethName:  "hostVeth2",
							HnsEndpointID: "hnsID2",
							HnsNetworkID:  "hnsNetworkID2",
							MacAddress:    "22:34:56:78:9a:bc",
							NICType:       cns.NodeNetworkInterfaceFrontendNIC,
						},
					},
					PodName:      "test-pod",
					PodNamespace: "test-pod-ns",
				}

				epInfos := cnsEndpointInfotoCNIEpInfos(cnsEndpointInfo, endpointID, "")

				Expect(len(epInfos)).To(Equal(2))
				Expect(epInfos).To(ContainElement(
					&EndpointInfo{
						IPAddresses:        dummyIPv4Slice,
						IfName:             "ifName1",
						HostIfName:         "hostVeth1",
						HNSEndpointID:      "hnsID1",
						NICType:            cns.InfraNIC,
						HNSNetworkID:       "hnsNetworkID1",
						MacAddress:         net.HardwareAddr("12:34:56:78:9a:bc"),
						ContainerID:        endpointID,
						EndpointID:         endpointID,
						NetworkContainerID: endpointID,
						PODName:            "test-pod",
						PODNameSpace:       "test-pod-ns",
					},
				))
				Expect(epInfos).To(ContainElement(
					&EndpointInfo{
						IPAddresses:        dummyIPv4Slice2,
						IfName:             "ifName2",
						HostIfName:         "hostVeth2",
						HNSEndpointID:      "hnsID2",
						NICType:            cns.NodeNetworkInterfaceFrontendNIC,
						HNSNetworkID:       "hnsNetworkID2",
						MacAddress:         net.HardwareAddr("22:34:56:78:9a:bc"),
						ContainerID:        endpointID,
						EndpointID:         endpointID,
						NetworkContainerID: endpointID,
						PODName:            "test-pod",
						PODNameSpace:       "test-pod-ns",
					},
				))
			})
		})
	})
	Describe("Test stateless generateCNSIPInfoMap", func() {
		Context("When converting from cni to cns with different combinations", func() {
			It("Should generate the cns endpoint info data from the endpoint structs for infraNIC+DelegatedVMNIC", func() {
				mac1, _ := net.ParseMAC("12:34:56:78:9a:bc")
				mac2, _ := net.ParseMAC("22:34:56:78:9a:bc")
				endpoints := []*endpoint{
					{
						IfName:       "eth0",
						NICType:      cns.InfraNIC,
						HnsId:        "hnsEndpointID1",
						HNSNetworkID: "hnsNetworkID1",
						HostIfName:   "hostIfName1",
						MacAddress:   mac1,
					},
					{
						IfName:       "eth1",
						NICType:      cns.NodeNetworkInterfaceFrontendNIC,
						HnsId:        "hnsEndpointID2",
						HNSNetworkID: "hnsNetworkID2",
						HostIfName:   "hostIfName2",
						MacAddress:   mac2,
					},
				}
				cnsEpInfos := generateCNSIPInfoMap(endpoints)
				Expect(len(cnsEpInfos)).To(Equal(2))

				Expect(cnsEpInfos).To(HaveKey("eth0"))
				Expect(cnsEpInfos["eth0"]).To(Equal(
					&restserver.IPInfo{
						NICType:       cns.InfraNIC,
						HnsEndpointID: "hnsEndpointID1",
						HnsNetworkID:  "hnsNetworkID1",
						HostVethName:  "hostIfName1",
						MacAddress:    "12:34:56:78:9a:bc",
					},
				))

				Expect(cnsEpInfos).To(HaveKey("eth1"))
				Expect(cnsEpInfos["eth1"]).To(Equal(
					&restserver.IPInfo{
						NICType:       cns.NodeNetworkInterfaceFrontendNIC,
						HnsEndpointID: "hnsEndpointID2",
						HnsNetworkID:  "hnsNetworkID2",
						HostVethName:  "hostIfName2",
						MacAddress:    "22:34:56:78:9a:bc",
					},
				))
			})
		})
	})
})

func TestGetEndpointIDByNicType_Cases(t *testing.T) {
	nm := &networkManager{}

	cases := []struct {
		name           string
		stateless      bool
		containerID    string
		ifName         string
		nicType        cns.NICType
		expectedResult string
	}{
		{
			name:           "Stateless InfraNIC",
			stateless:      true,
			containerID:    "container123",
			ifName:         "eth0",
			nicType:        cns.InfraNIC,
			expectedResult: "container123",
		},
		{
			name:           "Stateless SecondaryNIC",
			stateless:      true,
			containerID:    "container123",
			ifName:         "eth1",
			nicType:        cns.DelegatedVMNIC,
			expectedResult: "container123-eth1",
		},
		{
			name:           "Stateful InfraNIC",
			stateless:      false,
			containerID:    "container123456789",
			ifName:         "eth0",
			nicType:        cns.InfraNIC,
			expectedResult: "containe-eth0", // truncated to 8 chars
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nm.statelessCniMode = tc.stateless
			id := nm.GetEndpointIDByNicType(tc.containerID, tc.ifName, tc.nicType)
			if id != tc.expectedResult {
				t.Errorf("expected %s, got %s", tc.expectedResult, id)
			}
		})
	}
}

func TestCnsEndpointInfotoCNIEpInfos_Cases(t *testing.T) {
	cases := []struct {
		name            string
		ifName          string
		ipInfo          restserver.IPInfo
		netNs           string
		expectedNetNs   string
		expectedIfName  string
		expectedNICType cns.NICType
	}{
		{
			name:   "NodeNetworkInterfaceFrontendNIC",
			ifName: "eth1",
			netNs:  "/var/run/netns/testns",
			ipInfo: restserver.IPInfo{
				NICType: cns.NodeNetworkInterfaceFrontendNIC,
			},
			expectedNetNs:   "/var/run/netns/testns",
			expectedIfName:  "eth1",
			expectedNICType: cns.NodeNetworkInterfaceFrontendNIC,
		},
		{
			name:   "InfraNIC",
			ifName: "eth0",
			netNs:  "",
			ipInfo: restserver.IPInfo{
				NICType: cns.InfraNIC,
			},
			expectedNetNs:   "",
			expectedIfName:  "eth0",
			expectedNICType: cns.InfraNIC,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			endpointInfo := restserver.EndpointInfo{
				IfnameToIPMap: map[string]*restserver.IPInfo{
					tc.ifName: &tc.ipInfo,
				},
			}
			epInfos := cnsEndpointInfotoCNIEpInfos(endpointInfo, "container123", tc.netNs)
			if len(epInfos) == 0 {
				t.Fatalf("expected at least one epInfo")
			}
			if epInfos[0].NetNsPath != tc.expectedNetNs {
				t.Errorf("expected NetNsPath %q, got %q", tc.expectedNetNs, epInfos[0].NetNsPath)
			}
			if epInfos[0].IfName != tc.expectedIfName {
				t.Errorf("expected IfName %q, got %q", tc.expectedIfName, epInfos[0].IfName)
			}
			if epInfos[0].NICType != tc.expectedNICType {
				t.Errorf("expected NICType %v, got %v", tc.expectedNICType, epInfos[0].NICType)
			}
		})
	}
}
