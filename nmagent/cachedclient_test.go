package nmagent_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Azure/azure-container-networking/nmagent"
	"github.com/google/go-cmp/cmp"
)

// TestHomeAzCache makes sure the CachedClient works properly in caching home ez response and error
func TestHomeAzCache(t *testing.T) {
	tests := []struct {
		name      string
		exp       nmagent.HomeAzResponse
		expPath   string
		resp      map[string]interface{}
		shouldErr bool
	}{
		{
			"happy path",
			nmagent.HomeAzResponse{HomeAz: uint(1)},
			"/machine/plugins/?comp=nmagent&type=GetHomeAz",
			map[string]interface{}{
				"httpStatusCode": "200",
				"HomeAz":         1,
			},
			false,
		},
		{
			"empty response",
			nmagent.HomeAzResponse{},
			"/machine/plugins/?comp=nmagent&type=GetHomeAz",
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
			client := nmagent.CachedClient{Client: nmagent.NewTestClient(&TestTripper{
				RoundTripF: func(req *http.Request) (*http.Response, error) {
					rr := httptest.NewRecorder()
					err := json.NewEncoder(rr).Encode(test.resp)
					if err != nil {
						t.Fatal("unexpected error encoding response: err:", err)
					}
					rr.WriteHeader(http.StatusOK)
					return rr.Result(), nil
				},
			})}

			// only testing the cache value, Other scenarios were covered in client_test.go
			populateErr := client.PopulateHomeAzCache(context.TODO())
			homeAzResponseCache, errCache := client.GetHomeAz(context.TODO())

			if errCache != nil && !test.shouldErr {
				t.Fatal("unexpected error: err:", errCache)
			}
			if errCache != nil && !cmp.Equal(populateErr, errCache) {
				t.Fatal("got discrepant errors, diff: ", cmp.Diff(errCache, populateErr))
			}

			if errCache == nil && test.shouldErr {
				t.Fatal("expected error but received none")
			}

			if !cmp.Equal(homeAzResponseCache, test.exp) {
				t.Error("response differs from expectation: diff:", cmp.Diff(homeAzResponseCache, test.exp))
			}
		})
	}
}
