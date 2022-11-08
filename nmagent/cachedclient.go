package nmagent

import (
	"context"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/log"
	"github.com/pkg/errors"
)

const (
	GetHomeAzAPIName = "GetHomeAz"
	ContextTimeOut   = 2 * time.Second
)

type CachedClient struct {
	ClientIF
	// The purpose of nmaSupportedApisCache here is only for reducing the call to nmagent when the getHomeAz api is supported,
	// but having other unexpected errors while querying homeAz During the retry
	nmaSupportedApisCache    []string
	homeAzResponseCache      HomeAzResponse
	homeAzResponseErrorCache error
	supportedApisMu          sync.RWMutex
	homeAzMu                 sync.RWMutex
	closing                  chan struct{}
}

// NewCachedClient creates a new CachedClient object
func NewCachedClient(nmagentClient ClientIF) *CachedClient {
	return &CachedClient{
		ClientIF: nmagentClient,
		closing:  make(chan struct{}),
		// the purpose of the default error here is for handling the case when cns started and already taken request to get HomeAz,
		// but the initial request to nmagent that sent from the refresh thread is still in process. In this case, the cache value
		// not get updated in time, so it will return empty homeAzResponse and nil err back, which is misleading. Adding a default
		// error cache here to resolve that
		homeAzResponseErrorCache: errors.New("default error cache"),
	}
}

// SupportedAPIs retrieves the capabilities of the nmagent running on
// the node. Cache the response if success
func (c *CachedClient) SupportedAPIs(ctx context.Context) ([]string, error) {
	supportedAPIs, err := c.ClientIF.SupportedAPIs(ctx)
	if err != nil {
		return supportedAPIs, errors.Wrap(err, "failed to retrieve supportedApis from nmagent")
	}
	c.updateNMASupportedApisCache(supportedAPIs)
	return supportedAPIs, nil
}

// GetHomeAz returns homeaz cache directly
func (c *CachedClient) GetHomeAz(_ context.Context) (HomeAzResponse, error) {
	return c.readHomeAzAndErrorCache()
}

// isAPISupportedByNMAgent checks cache to see if the given api supported by nmagent client.
func (c *CachedClient) isAPISupportedByNMAgent(api string) bool {
	supportedAPIs := c.readNMASupportedApisCache()
	for _, supportedAPI := range supportedAPIs {
		if supportedAPI == api {
			return true
		}
	}
	return false
}

// readNMASupportedApisCache gets the nmaSupportedApisCache value
func (c *CachedClient) readNMASupportedApisCache() []string {
	c.supportedApisMu.RLock()
	defer c.supportedApisMu.RUnlock()
	supportedApis := c.nmaSupportedApisCache
	supportedApisCopy := make([]string, len(supportedApis))
	copy(supportedApisCopy, supportedApis)
	return supportedApisCopy
}

// updateNMASupportedApisCache updates the nmaSupportedApisCache value
func (c *CachedClient) updateNMASupportedApisCache(supportedApis []string) {
	c.supportedApisMu.Lock()
	defer c.supportedApisMu.Unlock()
	c.nmaSupportedApisCache = supportedApis
}

// readHomeAzAndErrorCache get the homeAzResponseCache and homeAzResponseErrorCache value
func (c *CachedClient) readHomeAzAndErrorCache() (HomeAzResponse, error) {
	c.homeAzMu.RLock()
	defer c.homeAzMu.RUnlock()
	homeAzResponse := c.homeAzResponseCache
	err := c.homeAzResponseErrorCache
	return homeAzResponse, err
}

// updateHomeAzAndErrorCache update the homeAzResponseCache and homeAzResponseErrorCache value
func (c *CachedClient) updateHomeAzAndErrorCache(homeAzResponse HomeAzResponse, err error) {
	c.homeAzMu.Lock()
	defer c.homeAzMu.Unlock()
	c.homeAzResponseCache = homeAzResponse
	c.homeAzResponseErrorCache = err
}

// Start starts a new thread to refresh home az cache
func (c *CachedClient) Start(retryIntervalInSecs int) {
	go c.refresh(retryIntervalInSecs)
}

// Stop ends the refresh thread
func (c *CachedClient) Stop() {
	close(c.closing)
}

// refresh keeps retrying until successfully getting home az from nmagent
func (c *CachedClient) refresh(retryIntervalInSecs int) {
	for {
		select {
		case <-c.closing:
			return
		default:
			ctx, cancel := context.WithTimeout(context.Background(), ContextTimeOut)
			homeAzResponse, populateErr := c.populateHomeAzCache(ctx)
			// keep retrying when there is an error or getHomeAz api is not supported by nmagent
			if populateErr != nil || !homeAzResponse.IsSupported {
				log.Debugf("Failed to retrieve home az from nmagent, will retry. %v", populateErr)
				time.Sleep(time.Duration(retryIntervalInSecs) * time.Second)
			}
			log.Debugf("Successfully populated home az cache!")
			cancel()
			return
		}
	}
}

// PopulateHomeAzCache makes call to nmagent to retrieve home az if getHomeAz api is supported by nmagent
func (c *CachedClient) populateHomeAzCache(ctx context.Context) (HomeAzResponse, error) {
	// if GetHomeAz api name is not included in the supportedApis cache, makes the call to nmagent again
	// to update the cache in case nmagent is updated underneath
	if !c.isAPISupportedByNMAgent(GetHomeAzAPIName) {
		_, err := c.SupportedAPIs(ctx)
		if err != nil {
			returnErr := errors.Wrap(err, "failed to retrieves nmagent supported api")
			resp := HomeAzResponse{}
			c.updateHomeAzAndErrorCache(resp, returnErr)
			return resp, returnErr
		}
		// getHomeAz api is not supported by nmagent
		if !c.isAPISupportedByNMAgent(GetHomeAzAPIName) {
			resp := HomeAzResponse{}
			c.updateHomeAzAndErrorCache(resp, nil)
			return resp, nil
		}
	}

	// calling NMAgent to get home AZ
	homeAzResponse, err := c.ClientIF.GetHomeAz(ctx)
	if err != nil {
		wrapedErr := errors.Wrap(err, "failed to get HomeAz from nmagent")
		c.updateHomeAzAndErrorCache(homeAzResponse, wrapedErr)
		return homeAzResponse, wrapedErr
	}
	homeAzResponse.IsSupported = true
	c.updateHomeAzAndErrorCache(homeAzResponse, nil)
	return homeAzResponse, nil
}
