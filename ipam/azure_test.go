// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package ipam

import (
	"encoding/json"
	"github.com/Azure/azure-container-networking/common"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/current"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"net/http"
	"net/url"
	"testing"
	"time"
)

var ipamQueryUrl = "localhost:42424"
var ipamQueryResponse = "" +
	"<Interfaces>" +
	"	<Interface MacAddress=\"*\" IsPrimary=\"true\">" +
	"		<IPSubnet Prefix=\"10.0.0.0/16\">" +
	"			<IPAddress Address=\"10.0.0.4\" IsPrimary=\"true\"/>" +
	"			<IPAddress Address=\"10.0.0.5\" IsPrimary=\"false\"/>" +
	"		</IPSubnet>" +
	"		<IPSubnet Prefix=\"10.1.0.0/16\">" +
	"			<IPAddress Address=\"10.1.0.4\" IsPrimary=\"false\"/>" +
	"		</IPSubnet>" +
	"	</Interface>" +
	"</Interfaces>"

// Handles queries from IPAM source.
func handleIpamQuery(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(ipamQueryResponse))
}

func parseResult(stdinData []byte) (*cniTypesCurr.Result, error) {
	result := &cniTypesCurr.Result{}
	err := json.Unmarshal(stdinData, result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func TestAzure(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Azure source Suite")
}

var (
	_ = Describe("Test azure source", func() {

		var (
			testAgent *common.Listener
			err error
		)

		BeforeSuite(func() {
			// Create a fake local agent to handle requests from IPAM plugin.
			u, _ := url.Parse("tcp://" + ipamQueryUrl)
			testAgent, err = common.NewListener(u)
			Expect(err).NotTo(HaveOccurred())

			testAgent.AddHandler("/", handleIpamQuery)

			err = testAgent.Start(make(chan error, 1))
			Expect(err).NotTo(HaveOccurred())
		})

		AfterSuite(func() {
			// Cleanup.
			testAgent.Stop()
		})

		Describe("Test create Azure source", func() {

			Context("When create new azure source with empty options", func() {
				It("Should return as default", func() {
					options := make(map[string]interface{})
					azure, err := newAzureSource(options)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(azure.name).Should(Equal("Azure"))
					Expect(azure.queryUrl).Should(Equal(azureQueryUrl))
					Expect(azure.queryInterval).Should(Equal(azureQueryInterval))
				})
			})

			Context("When create new azure source with options", func() {
				It("Should return with default queryInterval", func() {
					options := make(map[string]interface{})
					second := 7
					queryInterval := time.Duration(second) * time.Second
					queryUrl := "http://testqueryurl:12121/test"
					options[common.OptIpamQueryInterval] = second
					options[common.OptIpamQueryUrl] = queryUrl
					azure, err := newAzureSource(options)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(azure.name).Should(Equal("Azure"))
					Expect(azure.queryUrl).Should(Equal(queryUrl))
					Expect(azure.queryInterval).Should(Equal(queryInterval))
				})
			})
		})

		Describe("Test Azure source refresh", func() {
			Context("When create new azure source with options", func() {
				It("Should return with default queryInterval", func() {
					options := make(map[string]interface{})
					options[common.OptEnvironment] = common.OptEnvironmentAzure
					options[common.OptAPIServerURL] = "null"
					options[common.OptIpamQueryUrl] = "http://"+ipamQueryUrl

					am, err := createAddressManager(options)
					Expect(err).ToNot(HaveOccurred())

					amImpl := am.(*addressManager)

					err = amImpl.source.refresh()
					Expect(err).ToNot(HaveOccurred())

					as, ok := amImpl.AddrSpaces["local"]
					Expect(ok).To(BeTrue())

					pool, ok := as.Pools["10.0.0.0/16"]
					Expect(ok).To(BeTrue())

					_, ok = pool.Addresses["10.0.0.4"]
					Expect(ok).NotTo(BeTrue())

					_, ok = pool.Addresses["10.0.0.5"]
					Expect(ok).To(BeTrue())

					_, ok = pool.Addresses["10.1.0.4"]
					Expect(ok).NotTo(BeTrue())

					pool, ok = as.Pools["10.1.0.0/16"]
					Expect(ok).To(BeTrue())

					_, ok = pool.Addresses["10.1.0.4"]
					Expect(ok).To(BeTrue())
				})
			})
		})
	})
)