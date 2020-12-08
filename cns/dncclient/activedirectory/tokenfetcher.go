package activedirectory

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/go-autorest/autorest/adal"
)

// A TokenFetcher returns a service principal token for a specific resource.
// The tokens returned may need to be refreshed prior to use.
type TokenFetcher interface {
	GetServicePrincipalToken(resource string) (*adal.ServicePrincipalToken, error)
}

const tokenRefreshTimeout = time.Minute

// GetFreshToken returns a freshly fetched OAuth token for the given service principal
func GetFreshToken(ctx context.Context, spt *adal.ServicePrincipalToken) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, tokenRefreshTimeout)
	defer cancel()
	if err := spt.RefreshWithContext(ctx); err != nil {
		return "", fmt.Errorf("could not refresh token: %v", err)
	}
	return spt.OAuthToken(), nil
}
