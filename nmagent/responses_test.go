package nmagent_test

import (
	"encoding/json"
	"testing"

	"github.com/Azure/azure-container-networking/nmagent"
	"github.com/google/go-cmp/cmp"
)

func TestContainsFixes(t *testing.T) {
	tests := []struct {
		name  string
		resp  nmagent.AzResponse
		fixes []nmagent.HomeAZFix
		exp   bool
	}{
		{
			"empty",
			nmagent.AzResponse{},
			[]nmagent.HomeAZFix{},
			true,
		},
		{
			"one present",
			nmagent.AzResponse{
				AppliedFixes: []nmagent.HomeAZFix{
					nmagent.HomeAZFixIPv6,
				},
			},
			[]nmagent.HomeAZFix{nmagent.HomeAZFixIPv6},
			true,
		},
		{
			"one absent",
			nmagent.AzResponse{
				AppliedFixes: []nmagent.HomeAZFix{},
			},
			[]nmagent.HomeAZFix{nmagent.HomeAZFixIPv6},
			false,
		},
		{
			"one with empty request",
			nmagent.AzResponse{
				AppliedFixes: []nmagent.HomeAZFix{
					nmagent.HomeAZFixIPv6,
				},
			},
			[]nmagent.HomeAZFix{},
			true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := test.resp.ContainsFixes(test.fixes...)

			exp := test.exp
			if got != exp {
				t.Error("unexpected response from ContainsFixes: exp:", exp, "got:", got)
			}
		})
	}
}

func TestUnmarshalAzResponse(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		exp       nmagent.AzResponse
		shouldErr bool
	}{
		{
			"empty",
			"{}",
			nmagent.AzResponse{},
			false,
		},
		{
			"only homeaz",
			`{"homeAz": 42}`,
			nmagent.AzResponse{
				HomeAz: 42,
			},
			false,
		},
		{
			"valid apiversion",
			`{"homeAz": 42, "apiVersion": 0}`,
			nmagent.AzResponse{
				HomeAz: 42,
			},
			false,
		},
		{
			"valid apiversion ipv6",
			`{"homeAz": 42, "apiVersion": 2}`,
			nmagent.AzResponse{
				HomeAz: 42,
				AppliedFixes: []nmagent.HomeAZFix{
					nmagent.HomeAZFixIPv6,
				},
			},
			false,
		},
		{
			"invalid apiversion",
			`{"homeAz": 42, "apiVersion": 42}`,
			nmagent.AzResponse{},
			true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var got nmagent.AzResponse
			err := json.Unmarshal([]byte(test.in), &got)
			if err != nil && !test.shouldErr {
				t.Fatal("unexpected error unmarshaling JSON: err:", err)
			}

			if err == nil && test.shouldErr {
				t.Fatal("expected error but received none")
			}

			exp := test.exp
			if !cmp.Equal(got, exp) {
				t.Error("received response differs from expected: diff:", cmp.Diff(got, exp))
			}
		})
	}
}
