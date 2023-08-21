package nmagent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/nmagent"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
)

var _ http.RoundTripper = &TestTripper{}

// TestTripper is a RoundTripper with a customizeable RoundTrip method for
// testing purposes
type TestTripper struct {
	RoundTripF func(*http.Request) (*http.Response, error)
}

func (t *TestTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.RoundTripF(req)
}

func TestNMAgentClientJoinNetwork(t *testing.T) {
	joinNetTests := []struct {
		name       string
		id         string
		exp        string
		respStatus int
		shouldErr  bool
	}{
		{
			"happy path",
			"00000000-0000-0000-0000-000000000000",
			"/machine/plugins?comp=nmagent&type=NetworkManagement%2FjoinedVirtualNetworks%2F00000000-0000-0000-0000-000000000000%2Fapi-version%2F1",
			http.StatusOK,
			false,
		},
		{
			"empty network ID",
			"",
			"",
			http.StatusOK, // this shouldn't be checked
			true,
		},
		{
			"internal error",
			"00000000-0000-0000-0000-000000000000",
			"/machine/plugins?comp=nmagent&type=NetworkManagement%2FjoinedVirtualNetworks%2F00000000-0000-0000-0000-000000000000%2Fapi-version%2F1",
			http.StatusInternalServerError,
			true,
		},
	}

	for _, test := range joinNetTests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// create a client
			var got string
			client := nmagent.NewTestClient(&TestTripper{
				RoundTripF: func(req *http.Request) (*http.Response, error) {
					got = req.URL.RequestURI()
					rr := httptest.NewRecorder()
					_, _ = fmt.Fprintf(rr, `{"httpStatusCode":"%d"}`, test.respStatus)
					rr.WriteHeader(http.StatusOK)
					return rr.Result(), nil
				},
			})

			ctx, cancel := testContext(t)
			defer cancel()

			// attempt to join network
			err := client.JoinNetwork(ctx, nmagent.JoinNetworkRequest{test.id})
			checkErr(t, err, test.shouldErr)

			if got != test.exp {
				t.Error("received URL differs from expectation: got", got, "exp:", test.exp)
			}
		})
	}
}

func TestNMAgentClientJoinNetworkRetry(t *testing.T) {
	// we want to ensure that the client will automatically follow up with
	// NMAgent, so we want to track the number of requests that it makes
	invocations := 0
	exp := 10

	client := nmagent.NewTestClient(&TestTripper{
		RoundTripF: func(_ *http.Request) (*http.Response, error) {
			rr := httptest.NewRecorder()
			if invocations < exp {
				rr.WriteHeader(http.StatusProcessing)
				invocations++
			} else {
				rr.WriteHeader(http.StatusOK)
			}
			_, _ = rr.WriteString(`{"httpStatusCode": "200"}`)
			return rr.Result(), nil
		},
	})

	ctx, cancel := testContext(t)
	defer cancel()

	// attempt to join network
	err := client.JoinNetwork(ctx, nmagent.JoinNetworkRequest{"00000000-0000-0000-0000-000000000000"})
	if err != nil {
		t.Fatal("unexpected error: err:", err)
	}

	if invocations != exp {
		t.Error("client did not make the expected number of API calls: got:", invocations, "exp:", exp)
	}
}

func TestNMAgentClientDeleteNetwork(t *testing.T) {
	deleteNetTests := []struct {
		name           string
		id             string
		exp            string
		respStatus     int
		shouldErr      bool
		shouldNotFound bool
	}{
		{
			"happy path",
			"00000000-0000-0000-0000-000000000000",
			"/machine/plugins?comp=nmagent&type=NetworkManagement%2FjoinedVirtualNetworks%2F00000000-0000-0000-0000-000000000000%2Fapi-version%2F1%2Fmethod%2FDELETE",
			http.StatusOK,
			false,
			false,
		},
		{
			"empty network ID",
			"",
			"",
			http.StatusOK, // this shouldn't be checked
			true,
			false,
		},
		{
			"internal error",
			"00000000-0000-0000-0000-000000000000",
			"/machine/plugins?comp=nmagent&type=NetworkManagement%2FjoinedVirtualNetworks%2F00000000-0000-0000-0000-000000000000%2Fapi-version%2F1%2Fmethod%2FDELETE",
			http.StatusInternalServerError,
			true,
			false,
		},
		{
			"network does not exist",
			"00000000-0000-0000-0000-000000000000",
			"/machine/plugins?comp=nmagent&type=NetworkManagement%2FjoinedVirtualNetworks%2F00000000-0000-0000-0000-000000000000%2Fapi-version%2F1%2Fmethod%2FDELETE",
			http.StatusBadRequest,
			true,
			true,
		},
	}

	for _, test := range deleteNetTests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// create a client
			var got string
			client := nmagent.NewTestClient(&TestTripper{
				RoundTripF: func(req *http.Request) (*http.Response, error) {
					got = req.URL.RequestURI()
					rr := httptest.NewRecorder()
					_, _ = fmt.Fprintf(rr, `{"httpStatusCode":"%d"}`, test.respStatus)
					rr.WriteHeader(http.StatusOK)
					return rr.Result(), nil
				},
			})

			ctx, cancel := testContext(t)
			defer cancel()

			// attempt to delete network
			err := client.DeleteNetwork(ctx, nmagent.DeleteNetworkRequest{test.id})
			checkErr(t, err, test.shouldErr)

			var nmaError nmagent.Error
			errors.As(err, &nmaError)
			if nmaError.NotFound() != test.shouldNotFound {
				t.Error("unexpected NotFound value: got:", nmaError.NotFound(), "exp:", test.shouldNotFound)
			}

			if got != test.exp {
				t.Error("received URL differs from expectation: got", got, "exp:", test.exp)
			}
		})
	}
}

func TestWSError(t *testing.T) {
	const wsError string = `
<?xml version="1.0" encoding="utf-8"?>
<Error xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://w
ww.w3.org/2001/XMLSchema">
<Code>InternalError</Code>
<Message>The server encountered an internal error. Please retry the request.
</Message>
<Details></Details>
</Error>
`

	client := nmagent.NewTestClient(&TestTripper{
		RoundTripF: func(_ *http.Request) (*http.Response, error) {
			rr := httptest.NewRecorder()
			rr.WriteHeader(http.StatusInternalServerError)
			_, _ = rr.WriteString(wsError)
			return rr.Result(), nil
		},
	})

	req := nmagent.GetNetworkConfigRequest{
		VNetID: "4815162342",
	}

	ctx, cancel := testContext(t)
	defer cancel()

	_, err := client.GetNetworkConfiguration(ctx, req)

	if err == nil {
		t.Fatal("expected error to not be nil")
	}

	var cerr nmagent.Error
	ok := errors.As(err, &cerr)
	if !ok {
		t.Fatal("error was not an nmagent.Error")
	}

	t.Log(cerr.Error())
	if !strings.Contains(cerr.Error(), "InternalError") {
		t.Error("error did not contain the error content from wireserver")
	}
}

func TestNMAgentGetNetworkConfig(t *testing.T) {
	getTests := []struct {
		name       string
		vnetID     string
		expURL     string
		expVNet    map[string]interface{}
		shouldCall bool
		shouldErr  bool
	}{
		{
			"happy path",
			"00000000-0000-0000-0000-000000000000",
			"/machine/plugins?comp=nmagent&type=NetworkManagement%2FjoinedVirtualNetworks%2F00000000-0000-0000-0000-000000000000%2Fapi-version%2F1",
			map[string]interface{}{
				"httpStatusCode": "200",
				"cnetSpace":      "10.10.1.0/24",
				"defaultGateway": "10.10.0.1",
				"dnsServers": []string{
					"1.1.1.1",
					"1.0.0.1",
				},
				"subnets":     []map[string]interface{}{},
				"vnetSpace":   "10.0.0.0/8",
				"vnetVersion": "12345",
			},
			true,
			false,
		},
	}

	for _, test := range getTests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var got string
			client := nmagent.NewTestClient(&TestTripper{
				RoundTripF: func(req *http.Request) (*http.Response, error) {
					rr := httptest.NewRecorder()
					got = req.URL.RequestURI()
					rr.WriteHeader(http.StatusOK)
					err := json.NewEncoder(rr).Encode(&test.expVNet)
					if err != nil {
						return nil, errors.Wrap(err, "encoding response")
					}

					return rr.Result(), nil
				},
			})

			ctx, cancel := testContext(t)
			defer cancel()

			gotVNet, err := client.GetNetworkConfiguration(ctx, nmagent.GetNetworkConfigRequest{test.vnetID})
			checkErr(t, err, test.shouldErr)

			if got != test.expURL && test.shouldCall {
				t.Error("unexpected URL: got:", got, "exp:", test.expURL)
			}

			// TODO(timraymond): this is ugly
			expVnet := nmagent.VirtualNetwork{
				CNetSpace:      test.expVNet["cnetSpace"].(string),
				DefaultGateway: test.expVNet["defaultGateway"].(string),
				DNSServers:     test.expVNet["dnsServers"].([]string),
				Subnets:        []nmagent.Subnet{},
				VNetSpace:      test.expVNet["vnetSpace"].(string),
				VNetVersion:    test.expVNet["vnetVersion"].(string),
			}
			if !cmp.Equal(gotVNet, expVnet) {
				t.Error("received vnet differs from expected: diff:", cmp.Diff(gotVNet, expVnet))
			}
		})
	}
}

func TestNMAgentGetNetworkConfigRetry(t *testing.T) {
	t.Parallel()

	count := 0
	exp := 10
	client := nmagent.NewTestClient(&TestTripper{
		RoundTripF: func(_ *http.Request) (*http.Response, error) {
			rr := httptest.NewRecorder()
			if count < exp {
				rr.WriteHeader(http.StatusProcessing)
				count++
			} else {
				rr.WriteHeader(http.StatusOK)
			}

			// we still need a fake response
			_, _ = rr.WriteString(`{"httpStatusCode": "200"}`)
			return rr.Result(), nil
		},
	})

	ctx, cancel := testContext(t)
	defer cancel()

	_, err := client.GetNetworkConfiguration(ctx, nmagent.GetNetworkConfigRequest{"00000000-0000-0000-0000-000000000000"})
	if err != nil {
		t.Fatal("unexpected error: err:", err)
	}

	if count != exp {
		t.Error("unexpected number of API calls: exp:", exp, "got:", count)
	}
}

func TestNMAgentPutNetworkContainer(t *testing.T) {
	putNCTests := []struct {
		name       string
		req        *nmagent.PutNetworkContainerRequest
		shouldCall bool
		shouldErr  bool
	}{
		{
			"happy path",
			&nmagent.PutNetworkContainerRequest{
				ID:         "350f1e3c-4283-4f51-83a1-c44253962ef1",
				Version:    uint64(12345),
				VNetID:     "be3a33e-61e3-42c7-bd23-6b949f57bd36",
				SubnetName: "TestSubnet",
				IPv4Addrs:  []string{"10.0.0.43"},
				Policies: []nmagent.Policy{
					{
						ID:   "policyID1",
						Type: "type1",
					},
					{
						ID:   "policyID2",
						Type: "type2",
					},
				},
				VlanID:              1234,
				AuthenticationToken: "swordfish",
				PrimaryAddress:      "10.0.0.1",
			},
			true,
			false,
		},
		{
			"no id",
			&nmagent.PutNetworkContainerRequest{
				Version:    uint64(12345),
				VNetID:     "be3a33e-61e3-42c7-bd23-6b949f57bd36",
				SubnetName: "TestSubnet",
				IPv4Addrs:  []string{"10.0.0.43"},
				Policies: []nmagent.Policy{
					{
						ID:   "policyID1",
						Type: "type1",
					},
					{
						ID:   "policyID2",
						Type: "type2",
					},
				},
				VlanID:              1234,
				AuthenticationToken: "swordfish",
				PrimaryAddress:      "10.0.0.1",
			},
			false,
			true,
		},
	}

	for _, test := range putNCTests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			didCall := false
			client := nmagent.NewTestClient(&TestTripper{
				RoundTripF: func(_ *http.Request) (*http.Response, error) {
					rr := httptest.NewRecorder()
					_, _ = rr.WriteString(`{"httpStatusCode": "200"}`)
					rr.WriteHeader(http.StatusOK)
					didCall = true
					return rr.Result(), nil
				},
			})

			err := client.PutNetworkContainer(context.TODO(), test.req)
			if err != nil && !test.shouldErr {
				t.Fatal("unexpected error: err", err)
			}

			if err == nil && test.shouldErr {
				t.Fatal("expected error but received none")
			}

			if test.shouldCall && !didCall {
				t.Fatal("expected call but received none")
			}

			if !test.shouldCall && didCall {
				t.Fatal("unexpected call. expected no call ")
			}
		})
	}
}

func TestNMAgentDeleteNC(t *testing.T) {
	deleteTests := []struct {
		name      string
		req       nmagent.DeleteContainerRequest
		exp       string
		shouldErr bool
	}{
		{
			"happy path",
			nmagent.DeleteContainerRequest{
				NCID:                "00000000-0000-0000-0000-000000000000",
				PrimaryAddress:      "10.0.0.1",
				AuthenticationToken: "swordfish",
			},
			//nolint:lll // not a useful linter in a test
			"/machine/plugins?comp=nmagent&type=NetworkManagement%2Finterfaces%2F10.0.0.1%2FnetworkContainers%2F00000000-0000-0000-0000-000000000000%2FauthenticationToken%2Fswordfish%2Fapi-version%2F1%2Fmethod%2FDELETE",
			false,
		},
	}

	var got string
	for _, test := range deleteTests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			client := nmagent.NewTestClient(&TestTripper{
				RoundTripF: func(req *http.Request) (*http.Response, error) {
					got = req.URL.RequestURI()
					rr := httptest.NewRecorder()
					_, _ = rr.WriteString(`{"httpStatusCode": "200"}`)
					return rr.Result(), nil
				},
			})

			err := client.DeleteNetworkContainer(context.TODO(), test.req)
			if err != nil && !test.shouldErr {
				t.Fatal("unexpected error: err:", err)
			}

			if err == nil && test.shouldErr {
				t.Fatal("expected error but received none")
			}

			if test.exp != got {
				t.Errorf("received URL differs from expectation:\n\texp: %q:\n\tgot: %q", test.exp, got)
			}
		})
	}
}

func TestNMAgentSupportedAPIs(t *testing.T) {
	tests := []struct {
		name      string
		exp       []string
		expPath   string
		resp      string
		shouldErr bool
	}{
		{
			"empty",
			nil,
			"/machine/plugins?comp=nmagent&type=GetSupportedApis",
			"<SupportedAPIsResponseXML></SupportedAPIsResponseXML>",
			false,
		},
		{
			"happy",
			[]string{"foo"},
			"/machine/plugins?comp=nmagent&type=GetSupportedApis",
			"<SupportedAPIsResponseXML><type>foo</type></SupportedAPIsResponseXML>",
			false,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var gotPath string
			client := nmagent.NewTestClient(&TestTripper{
				RoundTripF: func(req *http.Request) (*http.Response, error) {
					gotPath = req.URL.RequestURI()
					rr := httptest.NewRecorder()
					_, _ = rr.WriteString(test.resp)
					return rr.Result(), nil
				},
			})

			got, err := client.SupportedAPIs(context.Background())
			if err != nil && !test.shouldErr {
				t.Fatal("unexpected error: err:", err)
			}

			if err == nil && test.shouldErr {
				t.Fatal("expected error but received none")
			}

			if gotPath != test.expPath {
				t.Error("paths differ: got:", gotPath, "exp:", test.expPath)
			}

			if !cmp.Equal(got, test.exp) {
				t.Error("response differs from expectation: diff:", cmp.Diff(got, test.exp))
			}
		})
	}
}

func TestGetNCVersion(t *testing.T) {
	tests := []struct {
		name      string
		req       nmagent.NCVersionRequest
		expURL    string
		resp      map[string]interface{}
		shouldErr bool
	}{
		{
			"empty",
			nmagent.NCVersionRequest{},
			"",
			map[string]interface{}{},
			true,
		},
		{
			"happy path",
			nmagent.NCVersionRequest{
				AuthToken:          "foo",
				NetworkContainerID: "bar",
				PrimaryAddress:     "baz",
			},
			"/machine/plugins?comp=nmagent&type=NetworkManagement%2Finterfaces%2Fbaz%2FnetworkContainers%2Fbar%2Fversion%2FauthenticationToken%2Ffoo%2Fapi-version%2F1",
			map[string]interface{}{
				"httpStatusCode":     "200",
				"networkContainerId": "bar",
				"version":            "4815162342",
			},
			false,
		},
		{
			"non-200",
			nmagent.NCVersionRequest{
				AuthToken:          "foo",
				NetworkContainerID: "bar",
				PrimaryAddress:     "baz",
			},
			"/machine/plugins?comp=nmagent&type=NetworkManagement%2Finterfaces%2Fbaz%2FnetworkContainers%2Fbar%2Fversion%2FauthenticationToken%2Ffoo%2Fapi-version%2F1",
			map[string]interface{}{
				"httpStatusCode": "500",
			},
			true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var gotURL string
			client := nmagent.NewTestClient(&TestTripper{
				RoundTripF: func(req *http.Request) (*http.Response, error) {
					gotURL = req.URL.RequestURI()
					rr := httptest.NewRecorder()
					err := json.NewEncoder(rr).Encode(test.resp)
					if err != nil {
						t.Fatal("unexpected error encoding test response: err:", err)
					}
					rr.WriteHeader(http.StatusOK)
					return rr.Result(), nil
				},
			})

			ctx, cancel := testContext(t)
			defer cancel()

			got, err := client.GetNCVersion(ctx, test.req)
			checkErr(t, err, test.shouldErr)

			if gotURL != test.expURL {
				t.Error("received URL differs from expected: got:", gotURL, "exp:", test.expURL)
			}

			exp := nmagent.NCVersion{}
			if ncid, ok := test.resp["networkContainerId"]; ok {
				exp.NetworkContainerID = ncid.(string)
			}

			if version, ok := test.resp["version"]; ok {
				exp.Version = version.(string)
			}

			if !cmp.Equal(got, exp) {
				t.Error("response differs from expectation: diff:", cmp.Diff(got, exp))
			}
		})
	}
}

func TestGetNCVersionList(t *testing.T) {
	tests := []struct {
		name      string
		resp      map[string]interface{}
		expURL    string
		exp       nmagent.NCVersionList
		shouldErr bool
	}{
		{
			"happy path",
			map[string]interface{}{
				"httpStatusCode": "200",
				"networkContainers": []map[string]interface{}{
					{
						"networkContainerId": "foo",
						"version":            "42",
					},
				},
			},
			"/machine/plugins?comp=nmagent&type=NetworkManagement%2Finterfaces%2Fapi-version%2F2",
			nmagent.NCVersionList{
				Containers: []nmagent.NCVersion{
					{
						NetworkContainerID: "foo",
						Version:            "42",
					},
				},
			},
			false,
		},
		{
			"nma fail",
			map[string]interface{}{
				"httpStatusCode": "500",
			},
			"/machine/plugins?comp=nmagent&type=NetworkManagement%2Finterfaces%2Fapi-version%2F2",
			nmagent.NCVersionList{},
			true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var gotURL string
			client := nmagent.NewTestClient(&TestTripper{
				RoundTripF: func(req *http.Request) (*http.Response, error) {
					gotURL = req.URL.RequestURI()
					rr := httptest.NewRecorder()
					rr.WriteHeader(http.StatusOK)
					err := json.NewEncoder(rr).Encode(test.resp)
					if err != nil {
						t.Fatal("unexpected error encoding response: err:", err)
					}
					return rr.Result(), nil
				},
			})

			ctx, cancel := testContext(t)
			defer cancel()

			resp, err := client.GetNCVersionList(ctx)
			checkErr(t, err, test.shouldErr)

			if gotURL != test.expURL {
				t.Error("received URL differs from expected: got:", gotURL, "exp:", test.expURL)
			}

			if got := resp; !cmp.Equal(got, test.exp) {
				t.Error("response differs from expectation: diff:", cmp.Diff(got, test.exp))
			}
		})
	}
}

func TestGetHomeAz(t *testing.T) {
	tests := []struct {
		name      string
		exp       nmagent.AzResponse
		expPath   string
		resp      map[string]interface{}
		shouldErr bool
	}{
		{
			"happy path",
			nmagent.AzResponse{HomeAz: uint(1)},
			"/machine/plugins?comp=nmagent&type=GetHomeAz%2Fapi-version%2F1",
			map[string]interface{}{
				"httpStatusCode": "200",
				"HomeAz":         1,
			},
			false,
		},
		{
			"empty response",
			nmagent.AzResponse{},
			"/machine/plugins?comp=nmagent&type=GetHomeAz%2Fapi-version%2F1",
			map[string]interface{}{
				"httpStatusCode": "500",
			},
			true,
		},
		{
			"404 from NMA",
			nmagent.AzResponse{},
			"/machine/plugins?comp=nmagent&type=GetHomeAz%2Fapi-version%2F1",
			map[string]interface{}{
				"httpStatusCode": "404",
			},
			true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			client := nmagent.NewTestClient(&TestTripper{
				RoundTripF: func(req *http.Request) (*http.Response, error) {
					rr := httptest.NewRecorder()
					err := json.NewEncoder(rr).Encode(test.resp)
					if err != nil {
						t.Fatal("unexpected error encoding response: err:", err)
					}
					rr.WriteHeader(http.StatusOK)
					return rr.Result(), nil
				},
			})

			got, err := client.GetHomeAz(context.TODO())
			if err != nil && !test.shouldErr {
				t.Fatal("unexpected error: err:", err)
			}

			if err == nil && test.shouldErr {
				t.Fatal("expected error but received none")
			}

			if !cmp.Equal(got, test.exp) {
				t.Error("response differs from expectation: diff:", cmp.Diff(got, test.exp))
			}
		})
	}
}
