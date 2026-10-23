package restserver

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/fakes"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/nmagent"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
)

func TestNMAgentMonitorHomeAz(t *testing.T) {
	tests := []struct {
		name      string
		client    *fakes.NMAgentClientFake
		homeAzExp cns.HomeAzResponse
		shouldErr bool
	}{
		{
			"happy path",
			&fakes.NMAgentClientFake{
				SupportedAPIsF: func(_ context.Context) ([]string, error) {
					return []string{"GetHomeAz"}, nil
				},
				GetHomeAzF: func(_ context.Context) (nmagent.AzResponse, error) {
					return nmagent.AzResponse{HomeAz: uint(1), AppliedFixes: []nmagent.HomeAZFix{nmagent.HomeAZFixIPv6}}, nil
				},
			},
			cns.HomeAzResponse{IsSupported: true, HomeAz: uint(1), NmaAppliedTheIPV6Fix: true},
			false,
		},
		{
			"getHomeAz is not supported in nmagent",
			&fakes.NMAgentClientFake{
				SupportedAPIsF: func(_ context.Context) ([]string, error) {
					return []string{"dummy"}, nil
				},
				GetHomeAzF: func(_ context.Context) (nmagent.AzResponse, error) {
					return nmagent.AzResponse{}, nil
				},
			},
			cns.HomeAzResponse{},
			false,
		},
		{
			"api supported but home az value is not valid",
			&fakes.NMAgentClientFake{
				SupportedAPIsF: func(_ context.Context) ([]string, error) {
					return []string{GetHomeAzAPIName}, nil
				},
				GetHomeAzF: func(_ context.Context) (nmagent.AzResponse, error) {
					return nmagent.AzResponse{HomeAz: 0}, nil
				},
			},
			cns.HomeAzResponse{IsSupported: true},
			true,
		},
		{
			"api supported but apiVersion value is not valid",
			&fakes.NMAgentClientFake{
				SupportedAPIsF: func(_ context.Context) ([]string, error) {
					return []string{GetHomeAzAPIName}, nil
				},
				GetHomeAzF: func(_ context.Context) (nmagent.AzResponse, error) {
					return nmagent.AzResponse{HomeAz: uint(1), AppliedFixes: []nmagent.HomeAZFix{nmagent.HomeAZFixInvalid}}, nil
				},
			},
			cns.HomeAzResponse{IsSupported: true, HomeAz: uint(1), NmaAppliedTheIPV6Fix: false},
			false,
		},
		{
			"api supported but got unexpected errors",
			&fakes.NMAgentClientFake{
				SupportedAPIsF: func(_ context.Context) ([]string, error) {
					return []string{GetHomeAzAPIName}, nil
				},
				GetHomeAzF: func(_ context.Context) (nmagent.AzResponse, error) {
					return nmagent.AzResponse{}, errors.New("unexpected error")
				},
			},
			cns.HomeAzResponse{IsSupported: true},
			true,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			monitor := NewNMAgentMonitor(test.client, time.Second)
			monitor.Populate(context.TODO())

			getHomeAzResponse := monitor.GetHomeAz(context.TODO())
			// check the homeAz cache value
			if !cmp.Equal(getHomeAzResponse.HomeAzResponse, test.homeAzExp) {
				t.Error("homeAz cache differs from expectation: diff:", cmp.Diff(getHomeAzResponse.HomeAzResponse, test.homeAzExp))
			}

			// check returnCode for error
			if getHomeAzResponse.Response.ReturnCode != types.Success && !test.shouldErr {
				t.Fatal("unexpected error: ", getHomeAzResponse.Response.Message)
			}
			if getHomeAzResponse.Response.ReturnCode == types.Success && test.shouldErr {
				t.Fatal("expected error but received none")
			}
			t.Cleanup(func() {
				monitor.Stop()
			})
		})
	}
}

func TestNMAgentMonitorRNCSupport(t *testing.T) {
	tests := []struct {
		name          string
		apis          []string
		homeAzErr     error
		wantSupported bool
		wantHomeAz    bool
	}{
		{
			name:          "both APIs supported",
			apis:          []string{GetHomeAzAPIName, "NetworkManagementRNCSupport"},
			wantSupported: true,
			wantHomeAz:    true,
		},
		{
			name:       "only HomeAZ supported",
			apis:       []string{GetHomeAzAPIName},
			wantHomeAz: true,
		},
		{
			name:          "only RNC supported",
			apis:          []string{"NetworkManagementRNCSupport"},
			wantSupported: true,
		},
		{
			name: "neither API supported",
			apis: []string{"OtherAPI"},
		},
		{
			name: "empty API list",
			apis: []string{},
		},
		{
			name: "nil API list",
		},
		{
			name: "RNC API name must match exactly",
			apis: []string{"networkmanagementrncsupport", "NetworkManagementRNCSupportV2"},
		},
		{
			name:          "HomeAZ failure does not affect RNC support",
			apis:          []string{GetHomeAzAPIName, "NetworkManagementRNCSupport"},
			homeAzErr:     errors.New("home az unavailable"),
			wantSupported: true,
			wantHomeAz:    true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			apiCalls, homeAzCalls := 0, 0
			monitor := NewNMAgentMonitor(&fakes.NMAgentClientFake{
				SupportedAPIsF: func(context.Context) ([]string, error) {
					apiCalls++
					return test.apis, nil
				},
				GetHomeAzF: func(context.Context) (nmagent.AzResponse, error) {
					homeAzCalls++
					return nmagent.AzResponse{HomeAz: 1}, test.homeAzErr
				},
			}, time.Second)

			if got := monitor.GetRNCSupport(t.Context()); got.Response.ReturnCode != types.NotFound || got.RNCSupportResponse.IsSupported {
				t.Fatalf("unexpected initial RNC support response: %+v", got)
			}
			monitor.Populate(t.Context())
			for range 2 {
				if got := monitor.GetRNCSupport(t.Context()); got.Response.ReturnCode != types.Success || got.RNCSupportResponse.IsSupported != test.wantSupported {
					t.Fatalf("RNC support response = %+v, want success with supported=%t", got, test.wantSupported)
				}
			}
			if apiCalls != 1 {
				t.Fatalf("SupportedAPIs calls = %d, want 1", apiCalls)
			}
			wantHomeAzCalls := 0
			if test.wantHomeAz {
				wantHomeAzCalls = 1
			}
			if homeAzCalls != wantHomeAzCalls {
				t.Fatalf("GetHomeAz calls = %d, want %d", homeAzCalls, wantHomeAzCalls)
			}
		})
	}
}

func TestNMAgentMonitorRNCSupportRefresh(t *testing.T) {
	tests := []struct {
		name          string
		apis          []string
		err           error
		wantSupported bool
		wantCode      types.ResponseCode
	}{
		{name: "initial query failure", err: errors.New("query failed"), wantCode: types.NmAgentSupportedApisError},
		{name: "repeated failure", err: errors.New("query failed"), wantCode: types.NmAgentSupportedApisError},
		{name: "support detected", apis: []string{RNCSupportAPIName}, wantSupported: true},
		{name: "support unchanged", apis: []string{RNCSupportAPIName}, wantSupported: true},
		{name: "failure replaces support", err: errors.New("query failed"), wantCode: types.NmAgentSupportedApisError},
		{name: "support restored", apis: []string{RNCSupportAPIName}, wantSupported: true},
		{name: "support removed"},
		{name: "lack of support unchanged"},
		{name: "failure replaces lack of support", err: errors.New("query failed"), wantCode: types.NmAgentSupportedApisError},
		{name: "recovery without support"},
	}
	var apis []string
	var queryErr error
	monitor := NewNMAgentMonitor(&fakes.NMAgentClientFake{
		SupportedAPIsF: func(context.Context) ([]string, error) {
			return apis, queryErr
		},
		GetHomeAzF: func(context.Context) (nmagent.AzResponse, error) {
			t.Fatal("GetHomeAz called when not supported")
			return nmagent.AzResponse{}, nil
		},
	}, time.Second)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			apis, queryErr = test.apis, test.err
			monitor.Populate(t.Context())
			got := monitor.GetRNCSupport(t.Context())
			if got.Response.ReturnCode != test.wantCode || got.RNCSupportResponse.IsSupported != test.wantSupported {
				t.Fatalf("RNC support response = %+v, want code=%v and supported=%t", got, test.wantCode, test.wantSupported)
			}
			if queryErr != nil {
				if want := fmt.Sprintf("failed to query nmagent supported apis: %v", queryErr); got.Response.Message != want {
					t.Fatalf("RNC support message = %q, want %q", got.Response.Message, want)
				}
			}
		})
	}
}
