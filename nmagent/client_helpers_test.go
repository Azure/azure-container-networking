package nmagent

import (
	"net/http"

	"github.com/Azure/azure-container-networking/nmagent/internal"
	"github.com/Azure/azure-container-networking/retry"
)

// NewTestClient is a factory function available in tests only for creating
// NMAgent clients with a mock transport
func NewTestClient(transport http.RoundTripper) *Client {
	return &Client{
		httpClient: &http.Client{
			Transport: &internal.WireserverTransport{
				Transport: transport,
			},
		},
		host: "localhost",
		port: 12345,
		retrier: retry.Retrier{
			Cooldown: retry.AsFastAsPossible(),
		},
	}
}
