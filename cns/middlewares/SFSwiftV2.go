package middlewares

import (
	"context"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/pkg/errors"
)

type SFSWIFTv2Middleware struct{}

// IPConfigsRequestHandlerWrapper is the middleware function for handling SWIFT v2 IP config requests for SF standalone scenario. This function wraps the default SWIFT request
// and release IP configs handlers.
func (m *SFSWIFTv2Middleware) IPConfigsRequestHandlerWrapper(ipRequestHandler, _ cns.IPConfigsHandlerFunc) cns.IPConfigsHandlerFunc {
	return func(ctx context.Context, req cns.IPConfigsRequest) (*cns.IPConfigsResponse, error) {
		ipConfigsResp, err := ipRequestHandler(ctx, req)
		if err != nil {
			ipConfigsResp.Response.ReturnCode = types.UnexpectedError
			return ipConfigsResp, errors.Wrapf(err, "Failed to requestIPConfigs for SF from IPConfigsRequest %v", req)
		}

		// SwiftV2-SF will always request for secondaryInterfaces for a pod
		req.SecondaryInterfacesExist = true
		return ipConfigsResp, nil
	}
}

func (m *SFSWIFTv2Middleware) Type() cns.SWIFTV2Mode {
	return cns.SFSWIFTV2
}
