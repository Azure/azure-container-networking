package restserver

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"testing/synctest"
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
			wantInitial := cns.GetHomeAzResponse{Response: cns.Response{
				ReturnCode: types.NotFound,
				Message:    "HomeAz Cache is unavailable",
			}}
			if got := monitor.GetHomeAz(t.Context()); got != wantInitial {
				t.Fatalf("initial HomeAZ response = %+v, want %+v", got, wantInitial)
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
			name:        "wrapped internal server error",
			err:         errors.Wrap(nmagent.Error{Code: http.StatusInternalServerError}, "request failed"),
			wantCode:    types.NmAgentInternalServerError,
			wantMessage: fmt.Sprintf("[HomeAzMonitor] nmagent internal server error, %v", errors.Wrap(nmagent.Error{Code: http.StatusInternalServerError}, "request failed")),
			supported:   true,
		},
		{
			name:        "request deadline exceeded",
			err:         context.DeadlineExceeded,
			wantCode:    types.UnexpectedError,
			wantMessage: "[HomeAzMonitor] failed with Error. context deadline exceeded",
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
			if got := monitor.GetRNCSupport(t.Context()); got.Response.ReturnCode != types.Success || !got.RNCSupportResponse.IsSupported {
				t.Fatalf("RNC support response = %+v, want success with support", got)
			}
		})
	}
}

func TestNMAgentMonitorHomeAzRefresh(t *testing.T) {
	success := cns.GetHomeAzResponse{
		Response:       cns.Response{ReturnCode: types.Success, Message: "Get Home Az succeeded"},
		HomeAzResponse: cns.HomeAzResponse{IsSupported: true, HomeAz: 1, NmaAppliedTheIPV6Fix: true},
	}
	tests := []struct {
		name     string
		apis     []string
		queryErr error
		az       nmagent.AzResponse
		azErr    error
		want     cns.GetHomeAzResponse
	}{
		{
			name: "initial success", apis: []string{GetHomeAzAPIName, RNCSupportAPIName},
			az: nmagent.AzResponse{HomeAz: 1, AppliedFixes: []nmagent.HomeAZFix{nmagent.HomeAZFixIPv6}}, want: success,
		},
		{
			name: "RNC support removed without changing HomeAZ", apis: []string{GetHomeAzAPIName},
			az: nmagent.AzResponse{HomeAz: 1, AppliedFixes: []nmagent.HomeAZFix{nmagent.HomeAZFixIPv6}}, want: success,
		},
		{
			name: "discovery failure replaces successful HomeAZ", queryErr: errors.New("discovery failed"),
			want: cns.GetHomeAzResponse{Response: cns.Response{
				ReturnCode: types.NmAgentSupportedApisError,
				Message:    "[HomeAzMonitor] failed to query nmagent's supported apis, discovery failed",
			}},
		},
		{
			name: "HomeAZ and fixes updated after recovery", apis: []string{GetHomeAzAPIName, RNCSupportAPIName},
			az: nmagent.AzResponse{HomeAz: 2},
			want: cns.GetHomeAzResponse{
				Response:       success.Response,
				HomeAzResponse: cns.HomeAzResponse{IsSupported: true, HomeAz: 2},
			},
		},
		{
			name: "region without AZs replaces successful HomeAZ", apis: []string{GetHomeAzAPIName, RNCSupportAPIName},
			azErr: nmagent.Error{Code: http.StatusNotFound},
			want: cns.GetHomeAzResponse{Response: cns.Response{
				ReturnCode: types.Success,
				Message:    fmt.Sprintf("[HomeAzMonitor] region does not support AZs, NMAgent returned StatusCode: 404, error: %v", nmagent.Error{Code: http.StatusNotFound}),
			}},
		},
		{
			name: "HomeAZ timeout preserves API support", apis: []string{GetHomeAzAPIName, RNCSupportAPIName},
			azErr: context.DeadlineExceeded,
			want: cns.GetHomeAzResponse{
				Response:       cns.Response{ReturnCode: types.UnexpectedError, Message: "[HomeAzMonitor] failed with Error. context deadline exceeded"},
				HomeAzResponse: cns.HomeAzResponse{IsSupported: true},
			},
		},
		{
			name: "HomeAZ API support removed", apis: []string{RNCSupportAPIName},
			want: cns.GetHomeAzResponse{Response: cns.Response{
				ReturnCode: types.Success,
				Message:    "[HomeAzMonitor] nmagent does not support GetHomeAz api.",
			}},
		},
		{
			name: "original HomeAZ and fixes restored", apis: []string{GetHomeAzAPIName},
			az: nmagent.AzResponse{HomeAz: 1, AppliedFixes: []nmagent.HomeAZFix{nmagent.HomeAZFixIPv6}}, want: success,
		},
	}
	var current int
	var homeAzCalls int
	monitor := NewNMAgentMonitor(&fakes.NMAgentClientFake{
		SupportedAPIsF: func(context.Context) ([]string, error) {
			return tests[current].apis, tests[current].queryErr
		},
		GetHomeAzF: func(context.Context) (nmagent.AzResponse, error) {
			homeAzCalls++
			return tests[current].az, tests[current].azErr
		},
	}, time.Second)
	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current, homeAzCalls = i, 0
			monitor.Populate(t.Context())
			if got := monitor.GetHomeAz(t.Context()); got != test.want {
				t.Fatalf("HomeAZ response = %+v, want %+v", got, test.want)
			}
			wantCalls := 0
			if test.queryErr == nil && isAPISupportedByNMAgent(test.apis, GetHomeAzAPIName) {
				wantCalls = 1
			}
			if homeAzCalls != wantCalls {
				t.Fatalf("GetHomeAz calls = %d, want %d", homeAzCalls, wantCalls)
			}
			item := monitor.values.Items()[homeAzCacheKey]
			if item.Object != test.want || item.Expiration != 0 {
				t.Fatalf("HomeAZ cache item = %+v, want non-expiring typed response %+v", item, test.want)
			}
		})
	}
}

func TestNMAgentMonitorRefreshLifecycle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var queryContexts, homeAzContexts []context.Context
		interval := time.Minute
		monitor := NewNMAgentMonitor(&fakes.NMAgentClientFake{
			SupportedAPIsF: func(ctx context.Context) ([]string, error) {
				queryContexts = append(queryContexts, ctx)
				apis := []string{GetHomeAzAPIName}
				if len(queryContexts) == 1 {
					apis = append(apis, RNCSupportAPIName)
				}
				return apis, nil
			},
			GetHomeAzF: func(ctx context.Context) (nmagent.AzResponse, error) {
				homeAzContexts = append(homeAzContexts, ctx)
				return nmagent.AzResponse{HomeAz: uint(len(homeAzContexts))}, nil
			},
		}, interval)
		stop := sync.OnceFunc(monitor.Stop)
		defer stop()
		monitor.Start()
		synctest.Wait()

		for refresh := 1; refresh <= 2; refresh++ {
			if len(queryContexts) != refresh || len(homeAzContexts) != refresh {
				t.Fatalf("refresh %d: discovery calls = %d, HomeAZ calls = %d", refresh, len(queryContexts), len(homeAzContexts))
			}
			ctx := queryContexts[refresh-1]
			if ctx != homeAzContexts[refresh-1] {
				t.Fatal("discovery and HomeAZ requests did not share a context")
			}
			if deadline, ok := ctx.Deadline(); !ok || !deadline.Equal(time.Now().Add(ContextTimeOut)) {
				t.Fatalf("refresh deadline = %v (present=%t), want now + %v", deadline, ok, ContextTimeOut)
			}
			if ctx.Err() != context.Canceled {
				t.Fatalf("completed refresh context error = %v, want cancellation", ctx.Err())
			}
			if got := monitor.GetHomeAz(t.Context()); got.Response.ReturnCode != types.Success || got.HomeAzResponse.HomeAz != uint(refresh) {
				t.Fatalf("refresh %d: unexpected HomeAZ response: %+v", refresh, got)
			}
			if got := monitor.GetRNCSupport(t.Context()); got.Response.ReturnCode != types.Success || got.RNCSupportResponse.IsSupported != (refresh == 1) {
				t.Fatalf("refresh %d: unexpected RNC response: %+v", refresh, got)
			}
			if refresh == 1 {
				time.Sleep(interval)
				synctest.Wait()
			}
		}

		stop()
		synctest.Wait()
		time.Sleep(2 * interval)
		synctest.Wait()
		if len(queryContexts) != 2 || len(homeAzContexts) != 2 {
			t.Fatal("monitor continued refreshing after Stop")
		}
	})
}
