package restserver

import (
	"context"
	"fmt"
	"net/http"
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

			if supported, known := monitor.IsRNCSupported(); supported || known {
				t.Fatalf("RNC support before population = (%t, %t), want (false, false)", supported, known)
			}
			wantInitial := cns.GetHomeAzResponse{Response: cns.Response{
				ReturnCode: types.NotFound,
				Message:    "HomeAz Cache is unavailable",
			}}
			if got := monitor.GetHomeAz(t.Context()); got != wantInitial {
				t.Fatalf("initial HomeAZ response = %+v, want %+v", got, wantInitial)
			}

			monitor.Populate(t.Context())
			for range 2 {
				if supported, known := monitor.IsRNCSupported(); supported != test.wantSupported || !known {
					t.Fatalf("RNC support = (%t, %t), want (%t, true)", supported, known, test.wantSupported)
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
		wantKnown     bool
	}{
		{name: "initial query failure", err: errors.New("query failed")},
		{name: "support detected", apis: []string{RNCSupportAPIName}, wantSupported: true, wantKnown: true},
		{name: "failure retains support", err: errors.New("query failed"), wantSupported: true, wantKnown: true},
		{name: "support removed", wantKnown: true},
		{name: "failure retains lack of support", err: errors.New("query failed"), wantKnown: true},
		{name: "support restored", apis: []string{RNCSupportAPIName}, wantSupported: true, wantKnown: true},
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
			if supported, known := monitor.IsRNCSupported(); supported != test.wantSupported || known != test.wantKnown {
				t.Fatalf("RNC support = (%t, %t), want (%t, %t)", supported, known, test.wantSupported, test.wantKnown)
			}
			if queryErr != nil {
				want := cns.GetHomeAzResponse{Response: cns.Response{
					ReturnCode: types.NmAgentSupportedApisError,
					Message:    fmt.Sprintf("[HomeAzMonitor] failed to query nmagent's supported apis, %v", queryErr),
				}}
				if got := monitor.GetHomeAz(t.Context()); got != want {
					t.Fatalf("HomeAZ response = %+v, want %+v", got, want)
				}
			}
		})
	}
}

func TestNMAgentMonitorHomeAzErrors(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantCode    types.ResponseCode
		wantMessage string
		supported   bool
	}{
		{
			name:        "internal server error",
			err:         nmagent.Error{Code: http.StatusInternalServerError},
			wantCode:    types.NmAgentInternalServerError,
			wantMessage: fmt.Sprintf("[HomeAzMonitor] nmagent internal server error, %v", nmagent.Error{Code: http.StatusInternalServerError}),
			supported:   true,
		},
		{
			name:        "unauthorized",
			err:         nmagent.Error{Code: http.StatusUnauthorized},
			wantCode:    types.StatusUnauthorized,
			wantMessage: fmt.Sprintf("[HomeAzMonitor] failed to authenticate with OwningServiceInstanceId, %v", nmagent.Error{Code: http.StatusUnauthorized}),
			supported:   true,
		},
		{
			name:        "region without AZs",
			err:         nmagent.Error{Code: http.StatusNotFound},
			wantCode:    types.Success,
			wantMessage: fmt.Sprintf("[HomeAzMonitor] region does not support AZs, NMAgent returned StatusCode: 404, error: %v", nmagent.Error{Code: http.StatusNotFound}),
		},
		{
			name:        "unexpected HTTP status",
			err:         nmagent.Error{Code: http.StatusBadRequest},
			wantCode:    types.UnexpectedError,
			wantMessage: "[HomeAzMonitor] failed with StatusCode: 400",
			supported:   true,
		},
		{
			name:        "transport error",
			err:         errors.New("connection failed"),
			wantCode:    types.UnexpectedError,
			wantMessage: "[HomeAzMonitor] failed with Error. connection failed",
			supported:   true,
		},
		{
			name:        "invalid home az",
			wantCode:    types.UnexpectedError,
			wantMessage: "[HomeAzMonitor] invalid home az value from nmagent: 0",
			supported:   true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			monitor := NewNMAgentMonitor(&fakes.NMAgentClientFake{
				SupportedAPIsF: func(context.Context) ([]string, error) {
					return []string{GetHomeAzAPIName, RNCSupportAPIName}, nil
				},
				GetHomeAzF: func(context.Context) (nmagent.AzResponse, error) {
					return nmagent.AzResponse{}, test.err
				},
			}, time.Second)
			monitor.Populate(t.Context())
			want := cns.GetHomeAzResponse{
				Response:       cns.Response{ReturnCode: test.wantCode, Message: test.wantMessage},
				HomeAzResponse: cns.HomeAzResponse{IsSupported: test.supported},
			}
			if got := monitor.GetHomeAz(t.Context()); got != want {
				t.Fatalf("HomeAZ response = %+v, want %+v", got, want)
			}
			if supported, known := monitor.IsRNCSupported(); !supported || !known {
				t.Fatalf("RNC support = (%t, %t), want (true, true)", supported, known)
			}
		})
	}
}
