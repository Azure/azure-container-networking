package activedirectory

import (
	"github.com/Azure/go-autorest/autorest/adal"
)

// MSITokenFetcher is used to build a service principal token for accessing different resources.
// By default, if none of the fields are assigned, it will use the system assigned identity.
// Otherwise, it will return a token for either the ClientID or ResourceID provided.
type MSITokenFetcher struct {
	ClientID   string
	ResourceID string
}

// GetServicePrincipalToken returns a token for the specified resource.
// The token returned may need to be refreshed before use.
func (m *MSITokenFetcher) GetServicePrincipalToken(resource string) (*adal.ServicePrincipalToken, error) {
	msiEndpoint, _ := adal.GetMSIVMEndpoint() // error is always nil

	switch {
	case m.ClientID != "":
		return adal.NewServicePrincipalTokenFromMSIWithUserAssignedID(msiEndpoint, resource, m.ClientID)
	default:
		return adal.NewServicePrincipalTokenFromMSI(msiEndpoint, resource)
	}
}
