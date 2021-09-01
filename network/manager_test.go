package network

import (
	"errors"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

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
	})
})
