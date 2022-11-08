package nmagent_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/nmagent"
	"github.com/Azure/azure-container-networking/nmagent/fakes"
	"github.com/google/go-cmp/cmp"
)

// TestHomeAzCache makes sure the CachedClient works properly in caching home ez response and error
func TestHomeAzCache(t *testing.T) {
	tests := []struct {
		name         string
		client       *fakes.NMAgentClientFake
		getHomeAzExp nmagent.HomeAzResponse
		shouldErr    bool
	}{
		{
			"happy path",
			&fakes.NMAgentClientFake{
				SupportedAPIsF: func(ctx context.Context) ([]string, error) {
					return []string{"GetHomeAz"}, nil
				},
				GetHomeAzF: func(ctx context.Context) (nmagent.HomeAzResponse, error) {
					return nmagent.HomeAzResponse{IsSupported: true, HomeAz: uint(1)}, nil
				},
			},
			nmagent.HomeAzResponse{IsSupported: true, HomeAz: uint(1)},
			false,
		},
		{
			"getHomeAz is not supported in nmagent",
			&fakes.NMAgentClientFake{
				SupportedAPIsF: func(ctx context.Context) ([]string, error) {
					return []string{"dummy"}, nil
				},
				GetHomeAzF: func(ctx context.Context) (nmagent.HomeAzResponse, error) {
					return nmagent.HomeAzResponse{}, nil
				},
			},
			nmagent.HomeAzResponse{},
			false,
		},
		{
			"unexpected errors",
			&fakes.NMAgentClientFake{
				SupportedAPIsF: func(ctx context.Context) ([]string, error) {
					return []string{"dummy"}, nil
				},
				GetHomeAzF: func(ctx context.Context) (nmagent.HomeAzResponse, error) {
					return nmagent.HomeAzResponse{}, fmt.Errorf("unexpected errors")
				},
			},
			nmagent.HomeAzResponse{},
			false,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := nmagent.NewCachedClient(test.client)
			client.Start(1)

			// give some time for the thread to complete retrieving home az and update the cache
			time.Sleep(2 * time.Second)
			homeAzResponseCache, errCache := client.GetHomeAz(context.TODO())
			// check the homeAz cache value
			if !cmp.Equal(homeAzResponseCache, test.getHomeAzExp) {
				t.Error("homeAz cache differs from expectation: diff:", cmp.Diff(homeAzResponseCache, test.getHomeAzExp))
			}

			// check the error Cache
			if errCache != nil && !test.shouldErr {
				t.Fatal("unexpected error: err:", errCache)
			}
			if errCache == nil && test.shouldErr {
				t.Fatal("expected error but received none")
			}
			client.Stop()
		})
	}
}
