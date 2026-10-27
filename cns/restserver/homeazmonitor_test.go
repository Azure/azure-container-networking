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

// TestHomeAzMonitor makes sure the HomeAzMonitor works properly in caching home az
func TestHomeAzMonitor(t *testing.T) {
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
			homeAzMonitor := NewHomeAzMonitor(test.client, time.Second)
			homeAzMonitor.Populate(context.TODO())

			getHomeAzResponse := homeAzMonitor.GetHomeAz(context.TODO())
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
				homeAzMonitor.Stop()
			})
		})
	}
}

func TestHomeAzMonitorRefreshErrors(t *testing.T) {
	tests := []struct {
		name            string
		err             error
		preserveCache   bool
		homeAzCode      types.ResponseCode
		homeAzSupported bool
	}{
		{
			name:          "wireserver throttling",
			err:           nmagent.Error{Code: http.StatusTooManyRequests, Source: "wireserver"},
			preserveCache: true,
		},
		{
			name:          "wrapped nmagent throttling",
			err:           errors.Wrap(nmagent.Error{Code: http.StatusTooManyRequests, Source: "nmagent"}, "submitting request"),
			preserveCache: true,
		},
		{
			name:            "internal server error",
			err:             nmagent.Error{Code: http.StatusInternalServerError},
			homeAzCode:      types.NmAgentInternalServerError,
			homeAzSupported: true,
		},
		{
			name:            "unauthorized",
			err:             nmagent.Error{Code: http.StatusUnauthorized},
			homeAzCode:      types.StatusUnauthorized,
			homeAzSupported: true,
		},
		{
			name:       "not found",
			err:        nmagent.Error{Code: http.StatusNotFound},
			homeAzCode: types.Success,
		},
		{
			name:            "other http error",
			err:             nmagent.Error{Code: http.StatusBadGateway},
			homeAzCode:      types.UnexpectedError,
			homeAzSupported: true,
		},
		{
			name:            "other error",
			err:             errors.New("request failed"),
			homeAzCode:      types.UnexpectedError,
			homeAzSupported: true,
		},
	}
	for _, test := range tests {
		for _, api := range []string{"SupportedAPIs", "GetHomeAz"} {
			for _, hasCache := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/cached=%t", test.name, api, hasCache), func(t *testing.T) {
					t.Parallel()
					ctx := t.Context()
					var refreshErr error
					azResponse := nmagent.AzResponse{HomeAz: 1, AppliedFixes: []nmagent.HomeAZFix{nmagent.HomeAZFixIPv6}}
					client := &fakes.NMAgentClientFake{
						SupportedAPIsF: func(context.Context) ([]string, error) {
							if api == "SupportedAPIs" && refreshErr != nil {
								return nil, refreshErr
							}
							return []string{GetHomeAzAPIName}, nil
						},
						GetHomeAzF: func(context.Context) (nmagent.AzResponse, error) {
							if refreshErr != nil {
								if api == "SupportedAPIs" {
									t.Fatal("GetHomeAz called after SupportedAPIs failed")
								}
								return nmagent.AzResponse{}, refreshErr
							}
							return azResponse, nil
						},
					}
					monitor := NewHomeAzMonitor(client, time.Second)
					t.Cleanup(monitor.Stop)
					if hasCache {
						monitor.Populate(ctx)
					}
					before := monitor.GetHomeAz(ctx)

					refreshErr = test.err
					for attempt := 0; attempt < 2; attempt++ {
						monitor.Populate(ctx)
						got := monitor.GetHomeAz(ctx)
						if test.preserveCache {
							if diff := cmp.Diff(before, got); diff != "" {
								t.Fatalf("throttling changed the cached response (-want +got):\n%s", diff)
							}
						} else {
							wantCode := test.homeAzCode
							wantHomeAz := cns.HomeAzResponse{IsSupported: test.homeAzSupported}
							if api == "SupportedAPIs" {
								wantCode = types.NmAgentSupportedApisError
								wantHomeAz = cns.HomeAzResponse{}
							}
							if got.Response.ReturnCode != wantCode {
								t.Fatalf("unexpected return code: got %v, want %v", got.Response.ReturnCode, wantCode)
							}
							if diff := cmp.Diff(wantHomeAz, got.HomeAzResponse); diff != "" {
								t.Fatalf("unexpected home az response (-want +got):\n%s", diff)
							}
						}
					}

					refreshErr = nil
					azResponse = nmagent.AzResponse{HomeAz: 2}
					monitor.Populate(ctx)
					want := cns.GetHomeAzResponse{
						Response:       cns.Response{ReturnCode: types.Success, Message: "Get Home Az succeeded"},
						HomeAzResponse: cns.HomeAzResponse{IsSupported: true, HomeAz: 2},
					}
					if diff := cmp.Diff(want, monitor.GetHomeAz(ctx)); diff != "" {
						t.Fatalf("successful refresh did not replace the cache (-want +got):\n%s", diff)
					}
				})
			}
		}
	}
}
