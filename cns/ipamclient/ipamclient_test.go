package ipamclient

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/cnm/ipam"
	"github.com/Azure/azure-container-networking/common"
)

var mux *http.ServeMux
var ipamQueryUrl = "localhost:42424"
var ic *IpamClient

// Wraps the test run with service setup and teardown.
func TestMain(m *testing.M) {

	// Create a fake IPAM plugin to handle requests from CNS plugin.
	u, _ := url.Parse("tcp://" + ipamQueryUrl)
	ipamAgent, err := common.NewListener(u)
	if err != nil {
		fmt.Printf("Failed to create agent, err:%v.\n", err)
		return
	}
	ipamAgent.AddHandler(ipam.GetAddressSpacesPath, handleIpamAsIDQuery)
	ipamAgent.AddHandler(ipam.RequestPoolPath, handlePoolIDQuery)
	ipamAgent.AddHandler(ipam.RequestAddressPath, handleReserveIPQuery)
	ipamAgent.AddHandler(ipam.ReleasePoolPath, handleReleaseIPQuery)
	ipamAgent.AddHandler(ipam.GetPoolInfoPath, handleIPUtilizationQuery)

	err = ipamAgent.Start(make(chan error, 1))
	if err != nil {
		fmt.Printf("Failed to start agent, err:%v.\n", err)
		return
	}
	ic, err = NewIpamClient("http://" + ipamQueryUrl)
	if err != nil {
		fmt.Printf("Ipam client creation failed %+v", err)
	}

	// Run tests.
	exitCode := m.Run()

	ipamAgent.Stop()

	os.Exit(exitCode)

}

// Handles queries from GetAddressSpace.
func handleIpamAsIDQuery(w http.ResponseWriter, r *http.Request) {
	var addressSpaceResp = "{\"LocalDefaultAddressSpace\": \"local\", \"GlobalDefaultAddressSpace\": \"global\"}"
	w.Write([]byte(addressSpaceResp))
}

// Handles queries from GetPoolID
func handlePoolIDQuery(w http.ResponseWriter, r *http.Request) {
	var requestPoolResp = "{\"PoolID\":\"10.0.0.0/16\", \"Pool\": \"\"}"
	w.Write([]byte(requestPoolResp))
}

// Handles queries from ReserveIPAddress.
func handleReserveIPQuery(w http.ResponseWriter, r *http.Request) {
	var reserveIPResp = "{\"Address\":\"10.0.0.2/16\"}"
	w.Write([]byte(reserveIPResp))
}

// Handles queries from ReleaseIPAddress.
func handleReleaseIPQuery(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("{}"))
}

// Handles queries from GetIPAddressUtiltization.
func handleIPUtilizationQuery(w http.ResponseWriter, r *http.Request) {
	var ipUtilizationResp = "{\"Capacity\":10, \"Available\":7, \"UnhealthyAddresses\":[\"10.0.0.5\",\"10.0.0.6\",\"10.0.0.7\"]}"
	w.Write([]byte(ipUtilizationResp))
}

// Decodes plugin's responses to test requests.
func decodeResponse(w *httptest.ResponseRecorder, response interface{}) error {
	if w.Code != http.StatusOK {
		return fmt.Errorf("Request failed with HTTP error %d", w.Code)
	}

	if w.Body == nil {
		return fmt.Errorf("Response body is empty")
	}

	return json.NewDecoder(w.Body).Decode(&response)
}

// Tests IpamClient GetAddressSpace function to get AddressSpaceID.
func TestAddressSpaces(t *testing.T) {
	asID, err := ic.GetAddressSpace()
	if err != nil {
		t.Errorf("GetAddressSpace failed with %v\n", err)
		return
	}

	if asID != "local" {
		t.Errorf("GetAddressSpace failed with invalid as id %s", asID)
	}
}

// Tests IpamClient GetPoolID function to get PoolID.
func TestGetPoolID(t *testing.T) {
	subnet := "10.0.0.0/16"

	asID, err := ic.GetAddressSpace()
	if err != nil {
		t.Errorf("GetAddressSpace failed with %v\n", err)
		return
	}

	poolID, err := ic.GetPoolID(asID, subnet)
	if err != nil {
		t.Errorf("GetPoolID failed with %v\n", err)
		return
	}

	if poolID != "10.0.0.0/16" {
		t.Errorf("GetPoolId failed with invalid pool id %s", poolID)
	}
}

// Tests IpamClient ReserveIPAddress function to request IP for ID.
func TestReserveIP(t *testing.T) {
	subnet := "10.0.0.0/16"

	asID, err := ic.GetAddressSpace()
	if err != nil {
		t.Errorf("GetAddressSpace failed with %v\n", err)
		return
	}

	poolID, err := ic.GetPoolID(asID, subnet)
	if err != nil {
		t.Errorf("GetPoolID failed with %v\n", err)
		return
	}

	addr1, err := ic.ReserveIPAddress(poolID, "id1")
	if err != nil {
		t.Errorf("GetReserveIP failed with %v\n", err)
		return
	}
	if addr1 != "10.0.0.2/16" {
		t.Errorf("GetReserveIP returned ivnvalid IP %s\n", addr1)
		return
	}
	addr2, err := ic.ReserveIPAddress(poolID, "id1")
	if err != nil {
		t.Errorf("GetReserveIP failed with %v\n", err)
		return
	}
	if addr1 != addr2 {
		t.Errorf("GetReserveIP with id returned ivnvalid IP1 %s IP2 %s\n", addr1, addr2)
		return
	}

}

// Tests IpamClient ReleaseIPAddress function to release IP associated with ID.
func TestReleaseIP(t *testing.T) {
	subnet := "10.0.0.0/16"

	asID, err := ic.GetAddressSpace()
	if err != nil {
		t.Errorf("GetAddressSpace failed with %v\n", err)
		return
	}

	poolID, err := ic.GetPoolID(asID, subnet)
	if err != nil {
		t.Errorf("GetPoolID failed with %v\n", err)
		return
	}

	addr1, err := ic.ReserveIPAddress(poolID, "id1")
	if err != nil {
		t.Errorf("GetReserveIP failed with %v\n", err)
		return
	}
	if addr1 != "10.0.0.2/16" {
		t.Errorf("GetReserveIP returned ivnvalid IP %s\n", addr1)
		return
	}

	err = ic.ReleaseIPAddress(poolID, "id1")
	if err != nil {
		t.Errorf("Release reservation failed with %v\n", err)
		return
	}
}

// Tests IpamClient GetIPAddressUtilization function to retrieve IP Utilization info.
func TestIPAddressUtilization(t *testing.T) {
	subnet := "10.0.0.0/16"

	asID, err := ic.GetAddressSpace()
	if err != nil {
		t.Errorf("GetAddressSpace failed with %v\n", err)
		return
	}

	poolID, err := ic.GetPoolID(asID, subnet)
	if err != nil {
		t.Errorf("GetPoolID failed with %v\n", err)
		return
	}

	capacity, available, unhealthyAddrs, err := ic.GetIPAddressUtilization(poolID)
	if err != nil {
		t.Errorf("GetIPUtilization failed with %v\n", err)
		return
	}

	if capacity != 10 && available != 7 && len(unhealthyAddrs) == 3 {
		t.Errorf("GetIPUtilization returned invalid either capacity %v / available %v count/ unhealthyaddrs %v \n", capacity, available, unhealthyAddrs)
		return
	}

	log.Printf("Capacity %v Available %v Unhealthy %v", capacity, available, unhealthyAddrs)
}
