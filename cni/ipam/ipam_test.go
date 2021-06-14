// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package ipam

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/current"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/Azure/azure-container-networking/common"
)

var ipamQueryUrl = "localhost:42424"
var ipamQueryResponse = "" +
	"<Interfaces>" +
	"	<Interface MacAddress=\"*\" IsPrimary=\"true\">" +
	"		<IPSubnet Prefix=\"10.0.0.0/16\">" +
	"			<IPAddress Address=\"10.0.0.4\" IsPrimary=\"true\"/>" +
	"			<IPAddress Address=\"10.0.0.5\" IsPrimary=\"false\"/>" +
	"			<IPAddress Address=\"10.0.0.6\" IsPrimary=\"false\"/>" +
	"			<IPAddress Address=\"10.0.0.7\" IsPrimary=\"false\"/>" +
	"		</IPSubnet>" +
	"	</Interface>" +
	"</Interfaces>"

func TestIpam(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ipam Suite")
}

// Handles queries from IPAM source.
func handleIpamQuery(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(ipamQueryResponse))
}

func parseResult(stdinData []byte) (*cniTypesCurr.Result, error) {
	result := &cniTypesCurr.Result{}
	if err := json.Unmarshal(stdinData, result); err != nil {
		return nil, err
	}
	return result, nil
}

func getStdinData(cniversion, subnet, ipAddress string) []byte {
	stdinData := fmt.Sprintf(
		`{
			"cniversion": "%s",
			"ipam": {
				"type": "internal",
				"subnet": "%s",
				"ipAddress": "%s"
			}
		}`, cniversion, subnet, ipAddress)

	return []byte(stdinData)
}

var (
	plugin      *ipamPlugin
	testAgent   *common.Listener
	arg         *cniSkel.CmdArgs
	err         error
	endpointID1 = uuid.New().String()

	//this usedAddresses map is to test not duplicate IP's
	// have been provided throughout this test execution
	UsedAddresses = map[string]string{}

	//below is the network,used to test if the IP's provided by IPAM
	// is in the the space requested.
	network = net.IPNet{IP: net.ParseIP("10.0.0.0"), Mask: net.CIDRMask(16, 32)}

	_ = BeforeSuite(func() {
		// TODO: Ensure that the other testAgent has bees released.
		time.Sleep(1 * time.Second)
		// Create a fake local agent to handle requests from IPAM plugin.
		u, _ := url.Parse("tcp://" + ipamQueryUrl)
		testAgent, err = common.NewListener(u)
		Expect(err).NotTo(HaveOccurred())

		testAgent.AddHandler("/", handleIpamQuery)

		err = testAgent.Start(make(chan error, 1))
		Expect(err).NotTo(HaveOccurred())

		arg = &cniSkel.CmdArgs{}
	})

	_ = AfterSuite(func() {
		// Cleanup.
		plugin.Stop()
		testAgent.Stop()
	})

	_ = Describe("Test IPAM", func() {

		Context("IPAM start", func() {

			var config common.PluginConfig

			It("Create IPAM plugin", func() {
				// Create the plugin.
				plugin, err = NewPlugin("ipamtest", &config)
				Expect(err).NotTo(HaveOccurred())
			})

			It("Start IPAM plugin", func() {
				// Configure test mode.
				plugin.SetOption(common.OptEnvironment, common.OptEnvironmentAzure)
				plugin.SetOption(common.OptAPIServerURL, "null")
				plugin.SetOption(common.OptIpamQueryUrl, "http://"+ipamQueryUrl)
				// Start the plugin.
				err = plugin.Start(&config)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Describe("Test IPAM ADD and DELETE pool", func() {

			var result *cniTypesCurr.Result

			Context("When ADD with nothing, call for ipam triggering request pool and address", func() {
				It("Request pool and ADD successfully", func() {
					arg.StdinData = getStdinData("0.4.0", "", "")
					err = plugin.Add(arg)
					Expect(err).ShouldNot(HaveOccurred())
					result, err = parseResult(arg.StdinData)
					Expect(err).ShouldNot(HaveOccurred())

					AssertAddressNotInUse(result.IPs[0].Address.IP.String())

					TrackAddressUsage(result.IPs[0].Address.IP.String(), "")

					AssertProperAddressSpace(result.IPs[0].Address)
				})
			})

			Context("When DELETE with subnet and address, call for ipam triggering release address", func() {
				It("DELETE address successfully", func() {
					arg.StdinData = getStdinData("0.4.0", "10.0.0.0/16", result.IPs[0].Address.IP.String())
					err = plugin.Delete(arg)
					Expect(err).ShouldNot(HaveOccurred())

					delete(UsedAddresses, result.IPs[0].Address.IP.String())

				})
			})

			Context("When DELETE with subnet, call for ipam triggering releasing pool", func() {
				It("DELETE pool successfully", func() {
					arg.StdinData = getStdinData("0.4.0", "10.0.0.0/16", "")
					err = plugin.Delete(arg)
					Expect(err).ShouldNot(HaveOccurred())
				})
			})
		})

		Describe("Test IPAM ADD and DELETE address", func() {

			Context("When address is given", func() {
				It("Request pool and address successfully", func() {
					arg.StdinData = getStdinData("0.4.0", "", "10.0.0.6")
					err = plugin.Add(arg)
					Expect(err).ShouldNot(HaveOccurred())
					result, err := parseResult(arg.StdinData)
					Expect(err).ShouldNot(HaveOccurred())

					AssertAddressNotInUse(result.IPs[0].Address.IP.String())

					TrackAddressUsage(result.IPs[0].Address.IP.String(), "")

					AssertProperAddressSpace(result.IPs[0].Address)
				})
			})

			Context("When subnet is given", func() {
				It("Request a usable address successfully", func() {
					arg.StdinData = getStdinData("0.4.0", "10.0.0.0/16", "")
					err = plugin.Add(arg)

					Expect(err).ShouldNot(HaveOccurred())
					result, err := parseResult(arg.StdinData)
					Expect(err).ShouldNot(HaveOccurred())

					AssertAddressNotInUse(result.IPs[0].Address.IP.String())

					TrackAddressUsage(result.IPs[0].Address.IP.String(), "")

					AssertProperAddressSpace(result.IPs[0].Address)
				})
			})

			Context("When container id is given with subnet", func() {
				It("Request a usable address successfully", func() {
					arg.StdinData = getStdinData("0.4.0", "10.0.0.0/16", "")
					arg.ContainerID = endpointID1
					err = plugin.Add(arg)

					Expect(err).ShouldNot(HaveOccurred())
					result, err := parseResult(arg.StdinData)
					Expect(err).ShouldNot(HaveOccurred())

					AssertAddressNotInUse(result.IPs[0].Address.IP.String())

					TrackAddressUsage(result.IPs[0].Address.IP.String(), arg.ContainerID)

					AssertProperAddressSpace(result.IPs[0].Address)

					//release the container ID for next test
					arg.ContainerID = ""
				})
			})
		})

		Describe("Test IPAM DELETE", func() {

			Context("Delete when only container id is given", func() {
				It("Deleted", func() {
					arg.StdinData = getStdinData("0.4.0", "10.0.0.0/16", "")
					arg.ContainerID = endpointID1

					err = plugin.Delete(arg)
					Expect(err).ShouldNot(HaveOccurred())

					address := UsedAddresses[arg.ContainerID]

					RemoveAddressUsage(address, arg.ContainerID)

				})
			})

			Context("When address and subnet is given", func() {
				It("Release address successfully", func() {

					nextAddress := GetNextAddress()
					arg.StdinData = getStdinData("0.4.0", "10.0.0.0/16", nextAddress)
					err = plugin.Delete(arg)
					Expect(err).ShouldNot(HaveOccurred())

					RemoveAddressUsage(nextAddress, "")
				})
			})

			Context("When pool is in use", func() {
				It("Fail to request pool", func() {
					arg.StdinData = getStdinData("0.4.0", "", "")
					err = plugin.Add(arg)
					Expect(err).Should(HaveOccurred())
				})
			})

			Context("When address and subnet is given", func() {
				It("Release address successfully", func() {
					nextAddress := GetNextAddress()
					arg.StdinData = getStdinData("0.4.0", "10.0.0.0/16", nextAddress)
					err = plugin.Delete(arg)
					Expect(err).ShouldNot(HaveOccurred())

					RemoveAddressUsage(nextAddress, "")
				})
			})

			Context("When subnet is given", func() {
				It("Release pool successfully", func() {
					arg.StdinData = getStdinData("0.4.0", "10.0.0.0/16", "")
					err = plugin.Delete(arg)
					Expect(err).ShouldNot(HaveOccurred())
				})
			})

			Context("When pool is not use", func() {
				It("Confirm pool was released by succesfully requesting pool", func() {
					arg.StdinData = getStdinData("0.4.0", "", "")
					err = plugin.Add(arg)
					Expect(err).ShouldNot(HaveOccurred())
				})
			})
		})
	})
)

func GetNextAddress() string {
	//return first value
	for a := range UsedAddresses {
		return a
	}
	return ""
}

func AssertAddressNotInUse(address string) {
	//confirm if IP is in use by other invocation
	_, exists := UsedAddresses[address]

	Expect(exists).Should(BeFalse())
}

func TrackAddressUsage(address, containerId string) {
	// set the IP as in use
	// this is just for tracking in this test
	UsedAddresses[address] = address

	if containerId != "" {
		// set the container as in use
		UsedAddresses[containerId] = address
	}
}

func RemoveAddressUsage(address, containerId string) {
	delete(UsedAddresses, address)
	if containerId != "" {
		delete(UsedAddresses, arg.ContainerID)
		arg.ContainerID = ""
	}
}

func AssertProperAddressSpace(address net.IPNet) {
	//validate the IP is part of this network IP space
	Expect(network.Contains(address.IP)).Should(Equal(true))
	Expect(address.Mask).Should(Equal(network.Mask))
}
