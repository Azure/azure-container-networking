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

type HomeAzMonitor struct {
	nmagentClient
	values *cache.Cache
	// channel used as signal to end of the goroutine for populating home az cache
	closing                  chan struct{}
	cacheRefreshIntervalSecs time.Duration
}

// NewHomeAzMonitor creates a new HomeAzMonitor object
func NewHomeAzMonitor(client nmagentClient, cacheRefreshIntervalSecs time.Duration) *HomeAzMonitor {
	return &HomeAzMonitor{
		nmagentClient:            client,
		cacheRefreshIntervalSecs: cacheRefreshIntervalSecs,
		values:                   cache.New(cache.NoExpiration, cache.NoExpiration),
		closing:                  make(chan struct{}),
	}
}

// GetHomeAz returns home az cache value directly
func (h *HomeAzMonitor) GetHomeAz(_ context.Context) cns.GetHomeAzResponse {
	return h.readCacheValue()
}

// updateCacheValue updates home az cache value
func (h *HomeAzMonitor) updateCacheValue(resp cns.GetHomeAzResponse) {
	h.values.Set(homeAzCacheKey, resp, cache.NoExpiration)
}

// readCacheValue reads home az cache value
func (h *HomeAzMonitor) readCacheValue() cns.GetHomeAzResponse {
	cachedResp, found := h.values.Get(homeAzCacheKey)
	if !found {
		return cns.GetHomeAzResponse{Response: cns.Response{
			ReturnCode: types.UnexpectedError,
			Message:    "HomeAz Cache is unavailable",
		}, HomeAzResponse: cns.HomeAzResponse{IsSupported: false}}
	}
	return cachedResp.(cns.GetHomeAzResponse)
}

// Start starts a new thread to refresh home az cache
func (h *HomeAzMonitor) Start() {
	go h.refresh()
}

// Stop ends the refresh thread
func (h *HomeAzMonitor) Stop() {
	close(h.closing)
}

// refresh periodically pulls home az from nmagent
func (h *HomeAzMonitor) refresh() {
	// Ticker will not tick right away, so proactively make a call here to achieve that
	ctx, cancel := context.WithTimeout(context.Background(), ContextTimeOut)
	h.Populate(ctx)
	cancel()

	ticker := time.NewTicker(h.cacheRefreshIntervalSecs)
	defer ticker.Stop()
	for {
		select {
		case <-h.closing:
			return
		case <-ticker.C:
			ctx, cancel = context.WithTimeout(context.Background(), ContextTimeOut)
			h.Populate(ctx)
			cancel()
		}
	}
}

// Populate makes call to nmagent to retrieve home az if getHomeAz api is supported by nmagent
func (h *HomeAzMonitor) Populate(ctx context.Context) {
	supportedApis, err := h.SupportedAPIs(ctx)
	if err != nil {
		returnMessage := fmt.Sprintf("[HomeAzMonitor] failed to query nmagent's supported apis, %v", err)
		returnCode := types.NmAgentSupportedApisError
		h.update(returnCode, returnMessage, cns.HomeAzResponse{IsSupported: false})
		return
	}
	// check if getHomeAz api is supported by nmagent
	if !isAPISupportedByNMAgent(supportedApis, GetHomeAzAPIName) {
		returnMessage := fmt.Sprintf("[HomeAzMonitor] nmagent does not support %s api.", GetHomeAzAPIName)
		returnCode := types.Success
		h.update(returnCode, returnMessage, cns.HomeAzResponse{IsSupported: false})
		return
	}

	// calling NMAgent to get home AZ
	azResponse, err := h.nmagentClient.GetHomeAz(ctx)
	if err != nil {
		apiError := nmagent.Error{}
		if ok := errors.As(err, &apiError); ok {
			switch apiError.StatusCode() {
			case http.StatusInternalServerError:
				returnMessage := fmt.Sprintf("[HomeAzMonitor] nmagent internal server error, %v", err)
				returnCode := types.NmAgentInternalServerError
				h.update(returnCode, returnMessage, cns.HomeAzResponse{IsSupported: true})
				return

			case http.StatusUnauthorized:
				returnMessage := fmt.Sprintf("[HomeAzMonitor] failed to authenticate with OwningServiceInstanceId, %v", err)
				returnCode := types.StatusUnauthorized
				h.update(returnCode, returnMessage, cns.HomeAzResponse{IsSupported: true})
				return

			default:
				returnMessage := fmt.Sprintf("[HomeAzMonitor] failed with StatusCode: %d", apiError.StatusCode())
				returnCode := types.UnexpectedError
				h.update(returnCode, returnMessage, cns.HomeAzResponse{IsSupported: true})
				return
			}
		}
		returnMessage := fmt.Sprintf("[HomeAzMonitor] failed with Error. %v", err)
		returnCode := types.UnexpectedError
		h.update(returnCode, returnMessage, cns.HomeAzResponse{IsSupported: true})
		return
	}

	h.update(types.Success, "Get Home Az succeeded", cns.HomeAzResponse{IsSupported: true, HomeAz: azResponse.HomeAz})
}

// update constructs a GetHomeAzResponse entity and update its cache
func (h *HomeAzMonitor) update(code types.ResponseCode, msg string, homeAzResponse cns.HomeAzResponse) {
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
