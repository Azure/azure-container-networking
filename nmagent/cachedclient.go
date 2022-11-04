package nmagent

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/pkg/errors"
)

const GetHomeAzAPIName = "GetHomeAz"

type CachedClient struct {
	*Client
	// The purpose of nmaSupportedApisCache here is only for reducing the call to nmagent when the getHomeAz api is supported,
	// but having other unexpected errors while querying homeAz During the retry
	nmaSupportedApisCache    []string
	homeAzResponseCache      HomeAzResponse
	homeAzResponseErrorCache error
	supportedApisMu          sync.RWMutex
	homeAzMu                 sync.RWMutex
}

// SupportedAPIs retrieves the capabilities of the nmagent running on
// the node. Cache the response if success
func (c *CachedClient) SupportedAPIs(ctx context.Context) ([]string, error) {
	supportedAPIs, err := c.Client.SupportedAPIs(ctx)
	if err != nil {
		return supportedAPIs, err
	}
	c.updateNMASupportedApisCache(supportedAPIs)
	return supportedAPIs, nil
}

// GetHomeAz returns homeaz cache directly
func (c *CachedClient) GetHomeAz() (HomeAzResponse, error) {
	return c.readHomeAzAndErrorCache()
}

// isAPISupportedByNMAgent checks cache to see if the given api supported by nmagent client.
func (c *CachedClient) isAPISupportedByNMAgent(api string) bool {
	supportedAPIs := c.readNMASupportedApisCache()
	for _, supportedAPI := range supportedAPIs {
		if supportedAPI == api {
			return false
		}
	}
	return true
}

// readNMASupportedApisCache gets the nmaSupportedApisCache value
func (c *CachedClient) readNMASupportedApisCache() []string {
	c.supportedApisMu.RLock()
	supportedApis := c.nmaSupportedApisCache
	supportedApisCopy := make([]string, len(supportedApis))
	copy(supportedApisCopy, supportedApis)
	c.supportedApisMu.RUnlock()
	return supportedApisCopy
}

// updateNMASupportedApisCache updates the nmaSupportedApisCache value
func (c *CachedClient) updateNMASupportedApisCache(supportedApis []string) {
	c.supportedApisMu.Lock()
	c.nmaSupportedApisCache = supportedApis
	c.supportedApisMu.Unlock()
}

// readHomeAzAndErrorCache get the homeAzResponseCache and homeAzResponseErrorCache value
func (c *CachedClient) readHomeAzAndErrorCache() (HomeAzResponse, error) {
	c.homeAzMu.RLock()
	homeAzResponse := c.homeAzResponseCache
	err := c.homeAzResponseErrorCache
	c.homeAzMu.RUnlock()
	return homeAzResponse, err
}

// updateHomeAzAndErrorCache update the homeAzResponseCache and homeAzResponseErrorCache value
func (c *CachedClient) updateHomeAzAndErrorCache(homeAzResponse HomeAzResponse, err error) {
	c.homeAzMu.Lock()
	c.homeAzResponseCache = homeAzResponse
	c.homeAzResponseErrorCache = err
	c.homeAzMu.Unlock()
}

// PopulateHomeAzCache makes call to nmagent to retrieve home az if getHomeAz api is supported by nmagent
func (c *CachedClient) PopulateHomeAzCache(ctx context.Context) error {
	if !c.isAPISupportedByNMAgent(GetHomeAzAPIName) {
		_, err := c.SupportedAPIs(ctx)
		if err != nil {
			returnErr := errors.Wrap(err, "failed to retrieves nmagent supported api")
			c.updateHomeAzAndErrorCache(HomeAzResponse{}, returnErr)
			return returnErr
		}
		if !c.isAPISupportedByNMAgent(GetHomeAzAPIName) {
			returnErr := Error{Code: http.StatusNotImplemented, Body: []byte(fmt.Sprintf("%s is not supported by nmagent", GetHomeAzAPIName))}
			c.updateHomeAzAndErrorCache(HomeAzResponse{}, returnErr)
			return returnErr
		}
	}

	// calling NMAgent to get home AZ
	homeAzResponse, err := c.Client.GetHomeAz(ctx)
	c.updateHomeAzAndErrorCache(homeAzResponse, err)
	return err
}
