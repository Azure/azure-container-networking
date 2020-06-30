// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package ipam

import (
	"encoding/json"
	"fmt"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/current"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/platform"
)

var ipamQueryUrl = "localhost:42424"
var ipamQueryResponse = "" +
	"<Interfaces>" +
	"	<Interface MacAddress=\"*\" IsPrimary=\"true\">" +
	"		<IPSubnet Prefix=\"10.0.0.0/16\">" +
	"			<IPAddress Address=\"10.0.0.4\" IsPrimary=\"true\"/>" +
	"			<IPAddress Address=\"10.0.0.5\" IsPrimary=\"false\"/>" +
	"			<IPAddress Address=\"10.0.0.6\" IsPrimary=\"false\"/>" +
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

	plugin *ipamPlugin
	testAgent *common.Listener
	arg *cniSkel.CmdArgs
	err error

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
					address1, _ := platform.ConvertStringToIPNet("10.0.0.5/16")
					address2, _ := platform.ConvertStringToIPNet("10.0.0.6/16")
					Expect(result.IPs[0].Address.IP).Should(Or(Equal(address1.IP), Equal(address2.IP)))
					Expect(result.IPs[0].Address.Mask).Should(Equal(address1.Mask))
				})
			})

			Context("When DELETE with subnet and address, call for ipam triggering release address", func() {
				It("DELETE address successfully", func() {
					arg.StdinData = getStdinData("0.4.0", "10.0.0.0/16", result.IPs[0].Address.IP.String())
					err = plugin.Delete(arg)
					Expect(err).ShouldNot(HaveOccurred())
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
					address, _ := platform.ConvertStringToIPNet("10.0.0.6/16")
					Expect(result.IPs[0].Address.IP).Should(Equal(address.IP))
					Expect(result.IPs[0].Address.Mask).Should(Equal(address.Mask))
				})
			})

			Context("When subnet is given", func() {
				It("Request a usable address successfully", func() {
					arg.StdinData = getStdinData("0.4.0", "10.0.0.0/16", "")
					err = plugin.Add(arg)
					Expect(err).ShouldNot(HaveOccurred())
					result, err := parseResult(arg.StdinData)
					Expect(err).ShouldNot(HaveOccurred())
					address, _ := platform.ConvertStringToIPNet("10.0.0.5/16")
					Expect(result.IPs[0].Address.IP).Should(Equal(address.IP))
					Expect(result.IPs[0].Address.Mask).Should(Equal(address.Mask))
				})
			})
		})

		Describe("Test IPAM DELETE", func() {

			Context("When address and subnet is given", func() {
				It("Release address successfully", func() {
					arg.StdinData = getStdinData("0.4.0", "10.0.0.0/16", "10.0.0.5")
					err = plugin.Delete(arg)
					Expect(err).ShouldNot(HaveOccurred())
				})
			})

			Context("When address and subnet is given", func() {
				It("Release address successfully", func() {
					arg.StdinData = getStdinData("0.4.0", "10.0.0.0/16", "10.0.0.6")
					err = plugin.Delete(arg)
					Expect(err).ShouldNot(HaveOccurred())
				})
			})

			Context("When subnet is given", func() {
				It("Release pool successfully", func() {
					arg.StdinData = getStdinData("0.4.0", "10.0.0.0/16", "")
					err = plugin.Delete(arg)
					Expect(err).ShouldNot(HaveOccurred())
				})
			})
		})
	})
)
