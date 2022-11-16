package restserver

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/nmagent"
	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
)

const (
	GetHomeAzAPIName = "GetHomeAz"
	ContextTimeOut   = 2 * time.Second
	homeAzCacheKey   = "HomeAz"
)

type HomeAzCache struct {
	nmagentClient
	values *cache.Cache
	// channel used as signal to end of the goroutine for populating home az cache
	closing chan struct{}
	// channel used as signal to block the Start() of HomeAzCache until the first request to retrieve home az completes
	block chan struct{}
}

// NewHomeAzCache creates a new HomeAzCache object
func NewHomeAzCache(client nmagentClient) *HomeAzCache {
	return &HomeAzCache{
		nmagentClient: client,
		values:        cache.New(cache.NoExpiration, cache.NoExpiration),
		closing:       make(chan struct{}),
		block:         make(chan struct{}),
	}
}

// GetHomeAz returns home az cache value directly
func (h *HomeAzCache) GetHomeAz(_ context.Context) cns.GetHomeAzResponse {
	return h.readCacheValue()
}

// updateCacheValue updates home az cache value
func (h *HomeAzCache) updateCacheValue(resp cns.GetHomeAzResponse) {
	h.values.Set(homeAzCacheKey, resp, cache.NoExpiration)
}

// readCacheValue reads home az cache value
func (h *HomeAzCache) readCacheValue() cns.GetHomeAzResponse {
	cachedResp, _ := h.values.Get(homeAzCacheKey)
	return cachedResp.(cns.GetHomeAzResponse)
}

// Start starts a new thread to refresh home az cache
func (h *HomeAzCache) Start(retryIntervalInSecs time.Duration) {
	go h.refresh(retryIntervalInSecs)
	// block until the first request to nmagent for retrieving home az completes
	<-h.block
}

// Stop ends the refresh thread
func (h *HomeAzCache) Stop() {
	close(h.closing)
}

// refresh periodically pulls home az from nmagent
func (h *HomeAzCache) refresh(retryIntervalInSecs time.Duration) {
	// Ticker will not tick right away, so proactively make a call here to achieve that
	ctx, cancel := context.WithTimeout(context.Background(), ContextTimeOut)
	h.populate(ctx)
	cancel()

	// unblock Start()
	h.block <- struct{}{}

	ticker := time.NewTicker(retryIntervalInSecs)
	defer ticker.Stop()
	for {
		select {
		case <-h.closing:
			return
		case <-ticker.C:
			ctx, cancel = context.WithTimeout(context.Background(), ContextTimeOut)
			h.populate(ctx)
			cancel()
		}
	}
}

// populate makes call to nmagent to retrieve home az if getHomeAz api is supported by nmagent
func (h *HomeAzCache) populate(ctx context.Context) {
	supportedApis, err := h.SupportedAPIs(ctx)
	if err != nil {
		returnMessage := fmt.Sprintf("[HomeAzCache] failed to query nmagent's supported apis, %v", err)
		returnCode := types.NmAgentSupportedApisError
		h.update(returnCode, returnMessage, cns.HomeAzResponse{})
		return
	}
	// check if getHomeAz api is supported by nmagent
	if !isAPISupportedByNMAgent(supportedApis, GetHomeAzAPIName) {
		returnMessage := fmt.Sprintf("[HomeAzCache] nmagent does not support %s api.", GetHomeAzAPIName)
		returnCode := types.Success
		h.update(returnCode, returnMessage, cns.HomeAzResponse{})
		return
	}

	// calling NMAgent to get home AZ
	azResponse, err := h.nmagentClient.GetHomeAz(ctx)
	if err != nil {
		apiError := nmagent.Error{}
		if ok := errors.As(err, &apiError); ok {
			switch apiError.StatusCode() {
			case http.StatusInternalServerError:
				returnMessage := fmt.Sprintf("[HomeAzCache] nmagent server internal error, %v", err)
				returnCode := types.NmAgentInternalServerError
				h.update(returnCode, returnMessage, cns.HomeAzResponse{IsSupported: true})
				return

			case http.StatusUnauthorized:
				returnMessage := fmt.Sprintf("[HomeAzCache] failed to authenticate with OwningServiceInstanceId, %v", err)
				returnCode := types.StatusUnauthorized
				h.update(returnCode, returnMessage, cns.HomeAzResponse{IsSupported: true})
				return

			default:
				returnMessage := fmt.Sprintf("[HomeAzCache] failed with StatusCode: %d", apiError.StatusCode())
				returnCode := types.UnexpectedError
				h.update(returnCode, returnMessage, cns.HomeAzResponse{IsSupported: true})
				return
			}
		}
		returnMessage := fmt.Sprintf("[HomeAzCache] failed with Error. %v", err)
		returnCode := types.UnexpectedError
		h.update(returnCode, returnMessage, cns.HomeAzResponse{IsSupported: true})
		return
	}

	h.update(types.Success, "Get Home Az successfully", cns.HomeAzResponse{IsSupported: true, HomeAz: azResponse.HomeAz})
}

// update constructs a GetHomeAzResponse entity and update its cache
func (h *HomeAzCache) update(code types.ResponseCode, msg string, homeAzResponse cns.HomeAzResponse) {
	log.Debugf(msg)
	resp := cns.GetHomeAzResponse{
		Response: cns.Response{
			ReturnCode: code,
			Message:    msg,
		},
		HomeAzResponse: homeAzResponse,
	}
	h.updateCacheValue(resp)
}

// isAPISupportedByNMAgent checks if a nmagent client api slice contains a given api
func isAPISupportedByNMAgent(apis []string, api string) bool {
	for _, supportedAPI := range apis {
		if supportedAPI == api {
			return true
		}
	}
	return false
}
