package internal

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
)

type TestTripper struct {
	// TODO(timraymond): this entire struct is duplicated
	RoundTripF func(*http.Request) (*http.Response, error)
}

func (t *TestTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.RoundTripF(req)
}

func TestWireserverTransportPathTransform(t *testing.T) {
	// Wireserver introduces specific rules on how requests should be
	// transformed. This test ensures we got those correct.

	pathTests := []struct {
		name   string
		method string
		sub    string
		exp    string
	}{
		{
			"happy path",
			http.MethodGet,
			"/test/path",
			"/machine/plugins?comp=nmagent&type=test%2Fpath",
		},
		{
			"empty",
			http.MethodGet,
			"",
			"/machine/plugins?comp=nmagent&type=",
		},
		{
			"monopath",
			http.MethodGet,
			"/foo",
			"/machine/plugins?comp=nmagent&type=foo",
		},
		{
			"publish",
			http.MethodPost,
			//nolint:lll // not a useful linter in a test
			"/NetworkManagement/interfaces/10.224.0.33/networkContainers/99e8b0e0-3524-490b-b2ca-31928e6d75a3?authenticationToken=6e4f70a6-a379-4f76-8e7a-2ae18392dc07&api-version=1",
			//nolint:lll // not a useful linter in a test
			"/machine/plugins?comp=nmagent&type=NetworkManagement%2Finterfaces%2F10.224.0.33%2FnetworkContainers%2F99e8b0e0-3524-490b-b2ca-31928e6d75a3%2FauthenticationToken%2F6e4f70a6-a379-4f76-8e7a-2ae18392dc07%2Fapi-version%2F1",
		},
		{
			"publish with munged uri",
			http.MethodPost,
			//nolint:lll // not a useful linter in a test
			"/NetworkManagement/interfaces/10.224.0.33/networkContainers/99e8b0e0-3524-490b-b2ca-31928e6d75a3/authenticationToken/6e4f70a6-a379-4f76-8e7a-2ae18392dc07/api-version/1",
			//nolint:lll // not a useful linter in a test
			"/machine/plugins?comp=nmagent&type=NetworkManagement%2Finterfaces%2F10.224.0.33%2FnetworkContainers%2F99e8b0e0-3524-490b-b2ca-31928e6d75a3%2FauthenticationToken%2F6e4f70a6-a379-4f76-8e7a-2ae18392dc07%2Fapi-version%2F1",
		},
	}

	for _, test := range pathTests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var got string
			client := &http.Client{
				Transport: &WireserverTransport{
					Transport: &TestTripper{
						RoundTripF: func(r *http.Request) (*http.Response, error) {
							got = r.URL.RequestURI()
							rr := httptest.NewRecorder()
							rr.WriteHeader(http.StatusOK)
							_, _ = rr.WriteString(`{"httpStatusCode": "200"}`)
							return rr.Result(), nil
						},
					},
				},
			}

			// execute

			//nolint:noctx // just a test
			req, err := http.NewRequest(test.method, test.sub, http.NoBody)
			if err != nil {
				t.Fatal("error creating new request: err:", err)
			}

			resp, err := client.Do(req)
			if err != nil {
				t.Fatal("unexpected error submitting request: err:", err)
			}
			defer resp.Body.Close()

			// assert
			if got != test.exp {
				t.Error("received path differs from expectation: exp:", test.exp, "got:", got)
			}
		})
	}
}

func TestWireserverTransportStatusTransform(t *testing.T) {
	// Wireserver only responds with 200 or 400 and embeds the actual status code
	// in JSON. The Transport should correct this and return the actual status as
	// an actual status

	statusTests := []struct {
		name      string
		response  map[string]interface{}
		expBody   map[string]interface{}
		expStatus int
	}{
		{
			"401",
			map[string]interface{}{
				"httpStatusCode": "401",
			},
			map[string]interface{}{},
			http.StatusUnauthorized,
		},
		{
			"200 with body",
			map[string]interface{}{
				"httpStatusCode": "200",
				"some":           "data",
			},
			map[string]interface{}{
				"some": "data",
			},
			http.StatusOK,
		},
	}

	for _, test := range statusTests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			client := &http.Client{
				Transport: &WireserverTransport{
					Transport: &TestTripper{
						RoundTripF: func(r *http.Request) (*http.Response, error) {
							rr := httptest.NewRecorder()
							// mimic Wireserver handing back a 200 regardless:
							rr.WriteHeader(http.StatusOK)

							err := json.NewEncoder(rr).Encode(&test.response)
							if err != nil {
								return nil, errors.Wrap(err, "encoding json response")
							}

							return rr.Result(), nil
						},
					},
				},
			}

			// execute

			// nolint:noctx // just a test
			req, err := http.NewRequest(http.MethodGet, "/test/path", http.NoBody)
			if err != nil {
				t.Fatal("error creating new request: err:", err)
			}

			resp, err := client.Do(req)
			if err != nil {
				t.Fatal("unexpected error submitting request: err:", err)
			}
			defer resp.Body.Close()

			// assert
			gotStatus := resp.StatusCode
			if gotStatus != test.expStatus {
				t.Errorf("status codes differ: exp: (%d) %s: got (%d): %s", test.expStatus, http.StatusText(test.expStatus), gotStatus, http.StatusText(gotStatus))
			}

			var gotBody map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&gotBody)
			if err != nil {
				t.Fatal("unexpected error decoding json body: err:", err)
			}

			if !cmp.Equal(test.expBody, gotBody) {
				t.Error("received body differs from expected: diff:", cmp.Diff(test.expBody, gotBody))
			}
		})
	}
}

func TestWireserverTransportPutPost(t *testing.T) {
	// wireserver can't tolerate PUT requests, so they must be transformed to POSTs
	t.Parallel()

	var got string
	client := &http.Client{
		Transport: &WireserverTransport{
			Transport: &TestTripper{
				RoundTripF: func(req *http.Request) (*http.Response, error) {
					got = req.Method
					rr := httptest.NewRecorder()
					_, _ = rr.WriteString(`{"httpStatusCode": "200"}`)
					rr.WriteHeader(http.StatusOK)
					return rr.Result(), nil
				},
			},
		},
	}

	req, err := http.NewRequest(http.MethodPut, "/test/path", http.NoBody)
	if err != nil {
		t.Fatal("unexpected error creating http request: err:", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal("error submitting request: err:", err)
	}
	defer resp.Body.Close()

	exp := http.MethodPost
	if got != exp {
		t.Error("unexpected status: exp:", exp, "got:", got)
	}
}

func TestWireserverTransportPostBody(t *testing.T) {
	// all PUT and POST requests must have an empty string body
	t.Parallel()

	var gotBody []byte
	client := &http.Client{
		Transport: &WireserverTransport{
			Transport: &TestTripper{
				RoundTripF: func(req *http.Request) (*http.Response, error) {
					body, err := io.ReadAll(req.Body)
					if err != nil {
						t.Fatal("unexpected error reading request body: err:", err)
					}
					gotBody = body
					rr := httptest.NewRecorder()
					_, _ = rr.WriteString(`{"httpStatusCode": "200"}`)
					rr.WriteHeader(http.StatusOK)
					return rr.Result(), nil
				},
			},
		},
	}

	// PUT request
	req, err := http.NewRequest(http.MethodPut, "/test/path", http.NoBody)
	if err != nil {
		t.Fatal("unexpected error creating http request: err:", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal("error submitting request: err:", err)
	}
	defer resp.Body.Close()

	exp := `""` // an empty JSON string
	if got := string(gotBody); got != exp {
		t.Errorf("unexpected body received: got: %q, exp: %q\n", got, exp)
	}

	// POST request
	// nolint:noctx // just a test
	req, err = http.NewRequest(http.MethodPost, "/test/path", http.NoBody)
	if err != nil {
		t.Fatal("unexpected error creating http request: err:", err)
	}

	resp, err = client.Do(req)
	if err != nil {
		t.Fatal("error submitting request: err:", err)
	}
	defer resp.Body.Close()

	if got := string(gotBody); got != exp {
		t.Errorf("unexpected body received: got: %q, exp: %q\n", got, exp)
	}
}

func TestWireserverTransportQuery(t *testing.T) {
	// the query string must have its constituent parts converted to slashes and
	// appended to the path
	t.Parallel()

	var got string
	client := &http.Client{
		Transport: &WireserverTransport{
			Transport: &TestTripper{
				RoundTripF: func(req *http.Request) (*http.Response, error) {
					got = req.URL.RequestURI()
					rr := httptest.NewRecorder()
					_, _ = rr.WriteString(`{"httpStatusCode": "200"}`)
					rr.WriteHeader(http.StatusOK)
					return rr.Result(), nil
				},
			},
		},
	}

	// nolint:noctx // just a test
	req, err := http.NewRequest(http.MethodPut, "/test/path?api-version=1234&foo=bar", http.NoBody)
	if err != nil {
		t.Fatal("unexpected error creating http request: err:", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal("error submitting request: err:", err)
	}
	defer resp.Body.Close()

	exp := "/machine/plugins?comp=nmagent&type=test%2Fpath%2Fapi-version%2F1234%2Ffoo%2Fbar"
	if got != exp {
		t.Error("received request differs from expectation: got:", got, "want:", exp)
	}
}

func TestWireserverResponse(t *testing.T) {
	wsRespTests := []struct {
		name      string
		resp      string
		exp       int
		shouldErr bool
	}{
		{
			"empty",
			"{}",
			0,
			true,
		},
		{
			"happy path",
			`{
				"httpStatusCode": "401"
			}`,
			401,
			false,
		},
		{
			"missing code",
			`{
				"httpStatusCode": ""
			}`,
			0,
			true,
		},
		{
			"other stuff",
			`{
				"httpStatusCode": "201",
				"other": "stuff"
			}`,
			201,
			false,
		},
		{
			"not a string",
			`{
				"httpStatusCode": 201,
				"other": "stuff"
			}`,
			0,
			true,
		},
		{
			"processing",
			`{
				"httpStatusCode": "102",
				"other": "stuff"
			}`,
			102,
			false,
		},
	}

	for _, test := range wsRespTests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var resp WireserverResponse
			err := json.Unmarshal([]byte(test.resp), &resp)
			if err != nil {
				t.Fatal("unexpected unmarshaling error: err:", err)
			}

			got, err := resp.StatusCode()
			if err != nil && !test.shouldErr {
				t.Fatal("unexpected error retrieving status code: err:", err)
			}

			if err == nil && test.shouldErr {
				t.Fatal("no error received when one was expected")
			}

			if got != test.exp {
				t.Error("received incorrect code: got:", got, "want:", test.exp)
			}
		})
	}
}
