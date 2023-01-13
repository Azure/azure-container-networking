package nmagent_test

import (
	"encoding/json"
	"testing"

	"github.com/Azure/azure-container-networking/nmagent"
	"github.com/google/go-cmp/cmp"
)

func TestPolicyMarshal(t *testing.T) {
	policyTests := []struct {
		name   string
		policy nmagent.Policy
		exp    string
	}{
		{
			"basic",
			nmagent.Policy{
				ID:   "policyID1",
				Type: "type1",
			},
			"\"policyID1, type1\"",
		},
	}

	for _, test := range policyTests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := json.Marshal(test.policy)
			if err != nil {
				t.Fatal("unexpected err marshaling policy: err", err)
			}

			if string(got) != test.exp {
				t.Errorf("marshaled policy does not match expectation: got: %q: exp: %q", string(got), test.exp)
			}

			var enc nmagent.Policy
			err = json.Unmarshal(got, &enc)
			if err != nil {
				t.Fatal("unexpected error unmarshaling: err:", err)
			}

			if !cmp.Equal(enc, test.policy) {
				t.Error("re-encoded policy differs from expectation: diff:", cmp.Diff(enc, test.policy))
			}
		})
	}
}

func TestDeleteContainerRequestValidation(t *testing.T) {
	dcrTests := []struct {
		name          string
		req           nmagent.DeleteContainerRequest
		shouldBeValid bool
	}{
		{
			"empty",
			nmagent.DeleteContainerRequest{},
			false,
		},
		{
			"missing ncid",
			nmagent.DeleteContainerRequest{
				PrimaryAddress:      "10.0.0.1",
				AuthenticationToken: "swordfish",
			},
			false,
		},
		{
			"missing primary address",
			nmagent.DeleteContainerRequest{
				NCID:                "00000000-0000-0000-0000-000000000000",
				AuthenticationToken: "swordfish",
			},
			false,
		},
		{
			"missing auth token",
			nmagent.DeleteContainerRequest{
				NCID:           "00000000-0000-0000-0000-000000000000",
				PrimaryAddress: "10.0.0.1",
			},
			false,
		},
	}

	for _, test := range dcrTests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := test.req.Validate()
			if err != nil && test.shouldBeValid {
				t.Fatal("unexpected validation errors: err:", err)
			}

			if err == nil && !test.shouldBeValid {
				t.Fatal("expected request to be invalid but wasn't")
			}
		})
	}
}

func TestJoinNetworkRequestPath(t *testing.T) {
	jnr := nmagent.JoinNetworkRequest{
		NetworkID: "00000000-0000-0000-0000-000000000000",
	}

	exp := "/NetworkManagement/joinedVirtualNetworks/00000000-0000-0000-0000-000000000000/api-version/1"
	if jnr.Path() != exp {
		t.Error("unexpected path: exp:", exp, "got:", jnr.Path())
	}
}

func TestJoinNetworkRequestValidate(t *testing.T) {
	validateRequest := []struct {
		name          string
		req           nmagent.JoinNetworkRequest
		shouldBeValid bool
	}{
		{
			"invalid",
			nmagent.JoinNetworkRequest{
				NetworkID: "",
			},
			false,
		},
		{
			"valid",
			nmagent.JoinNetworkRequest{
				NetworkID: "00000000-0000-0000-0000-000000000000",
			},
			true,
		},
	}

	for _, test := range validateRequest {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := test.req.Validate()
			if err != nil && test.shouldBeValid {
				t.Fatal("unexpected error validating: err:", err)
			}

			if err == nil && !test.shouldBeValid {
				t.Fatal("expected request to be invalid but wasn't")
			}
		})
	}
}

func TestGetNetworkConfigRequestPath(t *testing.T) {
	pathTests := []struct {
		name string
		req  nmagent.GetNetworkConfigRequest
		exp  string
	}{
		{
			"happy path",
			nmagent.GetNetworkConfigRequest{
				VNetID: "00000000-0000-0000-0000-000000000000",
			},
			"/NetworkManagement/joinedVirtualNetworks/00000000-0000-0000-0000-000000000000/api-version/1",
		},
	}

	for _, test := range pathTests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := test.req.Path(); got != test.exp {
				t.Error("unexpected path: exp:", test.exp, "got:", got)
			}
		})
	}
}

func TestGetNetworkConfigRequestValidate(t *testing.T) {
	validateTests := []struct {
		name          string
		req           nmagent.GetNetworkConfigRequest
		shouldBeValid bool
	}{
		{
			"happy path",
			nmagent.GetNetworkConfigRequest{
				VNetID: "00000000-0000-0000-0000-000000000000",
			},
			true,
		},
		{
			"empty",
			nmagent.GetNetworkConfigRequest{},
			false,
		},
	}

	for _, test := range validateTests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := test.req.Validate()
			if err != nil && test.shouldBeValid {
				t.Fatal("expected request to be valid but wasn't: err:", err)
			}

			if err == nil && !test.shouldBeValid {
				t.Fatal("expected error to be invalid but wasn't")
			}
		})
	}
}

func TestPutNetworkContainerRequestPath(t *testing.T) {
	pathTests := []struct {
		name string
		req  nmagent.PutNetworkContainerRequest
		exp  string
	}{
		{
			"happy path",
			nmagent.PutNetworkContainerRequest{
				ID:         "00000000-0000-0000-0000-000000000000",
				VNetID:     "11111111-1111-1111-1111-111111111111",
				Version:    uint64(12345),
				SubnetName: "foo",
				IPv4Addrs: []string{
					"10.0.0.2",
					"10.0.0.3",
				},
				Policies: []nmagent.Policy{
					{
						ID:   "Foo",
						Type: "Bar",
					},
				},
				VlanID:              0,
				AuthenticationToken: "swordfish",
				PrimaryAddress:      "10.0.0.1",
			},
			"/NetworkManagement/interfaces/10.0.0.1/networkContainers/00000000-0000-0000-0000-000000000000/authenticationToken/swordfish/api-version/1",
		},
	}

	for _, test := range pathTests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := test.req.Path(); got != test.exp {
				t.Error("path differs from expectation: exp:", test.exp, "got:", got)
			}
		})
	}
}

func TestPutNetworkContainerRequestValidate(t *testing.T) {
	validationTests := []struct {
		name          string
		req           nmagent.PutNetworkContainerRequest
		shouldBeValid bool
	}{
		{
			"empty",
			nmagent.PutNetworkContainerRequest{},
			false,
		},
		{
			"happy",
			nmagent.PutNetworkContainerRequest{
				ID:         "00000000-0000-0000-0000-000000000000",
				VNetID:     "11111111-1111-1111-1111-111111111111",
				Version:    uint64(12345),
				SubnetName: "foo",
				IPv4Addrs: []string{
					"10.0.0.2",
					"10.0.0.3",
				},
				Policies: []nmagent.Policy{
					{
						ID:   "Foo",
						Type: "Bar",
					},
				},
				VlanID:              0,
				AuthenticationToken: "swordfish",
				PrimaryAddress:      "10.0.0.1",
			},
			true,
		},
		{
			"missing IPv4Addrs",
			nmagent.PutNetworkContainerRequest{
				ID:         "00000000-0000-0000-0000-000000000000",
				VNetID:     "11111111-1111-1111-1111-111111111111",
				Version:    uint64(12345),
				SubnetName: "foo",
				IPv4Addrs:  []string{}, // the important part
				Policies: []nmagent.Policy{
					{
						ID:   "Foo",
						Type: "Bar",
					},
				},
				VlanID:              0,
				AuthenticationToken: "swordfish",
				PrimaryAddress:      "10.0.0.1",
			},
			false,
		},
		{
			"missing subnet name",
			nmagent.PutNetworkContainerRequest{
				ID:         "00000000-0000-0000-0000-000000000000",
				VNetID:     "11111111-1111-1111-1111-111111111111",
				Version:    uint64(12345),
				SubnetName: "", // the important part of the test
				IPv4Addrs: []string{
					"10.0.0.2",
				},
				Policies: []nmagent.Policy{
					{
						ID:   "Foo",
						Type: "Bar",
					},
				},
				VlanID:              0,
				AuthenticationToken: "swordfish",
				PrimaryAddress:      "10.0.0.1",
			},
			false,
		},
		{
			"version 0 OK",
			nmagent.PutNetworkContainerRequest{
				ID:         "00000000-0000-0000-0000-000000000000",
				VNetID:     "11111111-1111-1111-1111-111111111111",
				Version:    uint64(0), // the important part of the test
				SubnetName: "foo",
				IPv4Addrs: []string{
					"10.0.0.2",
				},
				Policies: []nmagent.Policy{
					{
						ID:   "Foo",
						Type: "Bar",
					},
				},
				VlanID:              0,
				AuthenticationToken: "swordfish",
				PrimaryAddress:      "10.0.0.1",
			},
			true,
		},
		{
			"no version field",
			nmagent.PutNetworkContainerRequest{
				ID:         "00000000-0000-0000-0000-000000000000",
				VNetID:     "11111111-1111-1111-1111-111111111111",
				SubnetName: "foo",
				IPv4Addrs: []string{
					"10.0.0.2",
				},
				Policies: []nmagent.Policy{
					{
						ID:   "Foo",
						Type: "Bar",
					},
				},
				VlanID:              0,
				AuthenticationToken: "swordfish",
				PrimaryAddress:      "10.0.0.1",
			},
			true,
		},
		{
			"missing vnet id",
			nmagent.PutNetworkContainerRequest{
				ID:         "00000000-0000-0000-0000-000000000000",
				VNetID:     "", // the important part
				Version:    uint64(12345),
				SubnetName: "foo",
				IPv4Addrs: []string{
					"10.0.0.2",
				},
				Policies: []nmagent.Policy{
					{
						ID:   "Foo",
						Type: "Bar",
					},
				},
				VlanID:              0,
				AuthenticationToken: "swordfish",
				PrimaryAddress:      "10.0.0.1",
			},
			false,
		},
		{
			"missing PrimaryAddress",
			nmagent.PutNetworkContainerRequest{
				ID:         "00000000-0000-0000-0000-000000000000",
				VNetID:     "11111111-1111-1111-1111-111111111111",
				Version:    uint64(12345),
				SubnetName: "foo",
				IPv4Addrs: []string{
					"10.0.0.2",
				},
				Policies: []nmagent.Policy{
					{
						ID:   "Foo",
						Type: "Bar",
					},
				},
				VlanID:              0,
				AuthenticationToken: "swordfish",
			},
			false,
		},
		{
			"missing ID",
			nmagent.PutNetworkContainerRequest{
				VNetID:     "11111111-1111-1111-1111-111111111111",
				Version:    uint64(12345),
				SubnetName: "foo",
				IPv4Addrs: []string{
					"10.0.0.2",
				},
				Policies: []nmagent.Policy{
					{
						ID:   "Foo",
						Type: "Bar",
					},
				},
				VlanID:              0,
				AuthenticationToken: "swordfish",
				PrimaryAddress:      "10.0.0.1",
			},
			false,
		},
		{
			"missing AuthenticationToken",
			nmagent.PutNetworkContainerRequest{
				ID:         "00000000-0000-0000-0000-000000000000",
				VNetID:     "11111111-1111-1111-1111-111111111111",
				Version:    uint64(12345),
				SubnetName: "foo",
				IPv4Addrs: []string{
					"10.0.0.2",
				},
				Policies: []nmagent.Policy{
					{
						ID:   "Foo",
						Type: "Bar",
					},
				},
				VlanID:         0,
				PrimaryAddress: "10.0.0.1",
			},
			false,
		},
	}

	for _, test := range validationTests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := test.req.Validate()
			if err != nil && test.shouldBeValid {
				t.Fatal("unexpected error validating: err:", err)
			}

			if err == nil && !test.shouldBeValid {
				t.Fatal("expected validation error but received none")
			}
		})
	}
}

func TestNCVersionRequestValidate(t *testing.T) {
	tests := []struct {
		name          string
		req           nmagent.NCVersionRequest
		shouldBeValid bool
	}{
		{
			"empty",
			nmagent.NCVersionRequest{},
			false,
		},
		{
			"complete",
			nmagent.NCVersionRequest{
				AuthToken:          "blah",
				NetworkContainerID: "12345",
				PrimaryAddress:     "4815162342",
			},
			true,
		},
		{
			"missing ncid",
			nmagent.NCVersionRequest{
				AuthToken:      "blah",
				PrimaryAddress: "4815162342",
			},
			false,
		},
		{
			"missing auth token",
			nmagent.NCVersionRequest{
				NetworkContainerID: "12345",
				PrimaryAddress:     "4815162342",
			},
			false,
		},
		{
			"missing primary address",
			nmagent.NCVersionRequest{
				AuthToken:          "blah",
				NetworkContainerID: "12345",
			},
			false,
		},
		{
			"only auth token",
			nmagent.NCVersionRequest{
				AuthToken: "blah",
			},
			false,
		},
		{
			"only ncid",
			nmagent.NCVersionRequest{
				NetworkContainerID: "12345",
			},
			false,
		},
		{
			"only primary address",
			nmagent.NCVersionRequest{
				PrimaryAddress: "4815162342",
			},
			false,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := test.req.Validate()
			if err != nil && test.shouldBeValid {
				t.Fatal("request was not valid when it should have been: err:", err)
			}

			if err == nil && !test.shouldBeValid {
				t.Fatal("expected request to be invalid when it was valid")
			}
		})
	}
}
