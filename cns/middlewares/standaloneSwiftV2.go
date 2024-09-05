package middlewares

import (
	"context"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/pkg/errors"
)

type StandaloneSWIFTv2Middleware struct{}

// IPConfigsRequestHandlerWrapper is the middleware function for handling SWIFT v2 IP config requests for SF standalone scenario. This function wraps the default SWIFT request
// and release IP configs handlers.
func (m *StandaloneSWIFTv2Middleware) IPConfigsRequestHandlerWrapper(ipRequestHandler, _ cns.IPConfigsHandlerFunc) cns.IPConfigsHandlerFunc {
	return func(ctx context.Context, req cns.IPConfigsRequest) (*cns.IPConfigsResponse, error) {
		ipConfigsResp, err := ipRequestHandler(ctx, req)
		if err != nil {
			return ipConfigsResp, errors.Wrapf(err, "Failed to requestIPConfigs for Standalone SwiftV2 from IPConfigsRequest %+v", req)
		}

		return ipConfigsResp, nil
	}
}

func (m *StandaloneSWIFTv2Middleware) Type() cns.SWIFTV2Mode {
	return cns.StandaloneSWIFTV2
}
