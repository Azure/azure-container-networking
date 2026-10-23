package restserver

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/nmagent"
	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
)

const (
	GetHomeAzAPIName   = "GetHomeAz"
	RNCSupportAPIName  = "NetworkManagementRNCSupport"
	ContextTimeOut     = 5 * time.Second
	homeAzCacheKey     = "HomeAz"
	rncSupportCacheKey = "RNCSupport"
)

// NMAgentMonitor caches HomeAZ and RNC capability observations from NMAgent.
type NMAgentMonitor struct {
	nmagentClient
	values *cache.Cache
	// channel used as signal to end the cache refresh goroutine
	closing                  chan struct{}
	cacheRefreshIntervalSecs time.Duration
}

// NewNMAgentMonitor creates a monitor with an empty, non-expiring cache.
func NewNMAgentMonitor(client nmagentClient, cacheRefreshIntervalSecs time.Duration) *NMAgentMonitor {
	return &NMAgentMonitor{
		nmagentClient:            client,
		cacheRefreshIntervalSecs: cacheRefreshIntervalSecs,
		values:                   cache.New(cache.NoExpiration, cache.NoExpiration),
		closing:                  make(chan struct{}),
	}
}

// GetHomeAz returns home az cache value directly
func (h *NMAgentMonitor) GetHomeAz(_ context.Context) cns.GetHomeAzResponse {
	return h.readCacheValue()
}

// GetRNCSupport returns the cached result of the latest capability discovery.
func (h *NMAgentMonitor) GetRNCSupport(_ context.Context) cns.GetRNCSupportResponse {
	cachedResponse, found := h.values.Get(rncSupportCacheKey)
	if !found {
		return cns.GetRNCSupportResponse{Response: cns.Response{
			ReturnCode: types.NotFound,
			Message:    "RNC support cache is unavailable",
		}}
	}
	return cachedResponse.(cns.GetRNCSupportResponse)
}

func (h *NMAgentMonitor) updateRNCSupport(resp cns.GetRNCSupportResponse) {
	if cached, found := h.values.Get(rncSupportCacheKey); found && cached.(cns.GetRNCSupportResponse) == resp {
		return
	}
	h.values.Set(rncSupportCacheKey, resp, cache.NoExpiration)
	logger.Printf("[NMAgentMonitor] RNC support cache updated: %+v", resp)
}

// updateCacheValue updates home az cache value
func (h *NMAgentMonitor) updateCacheValue(resp cns.GetHomeAzResponse) {
	h.values.Set(homeAzCacheKey, resp, cache.NoExpiration)
}

// readCacheValue reads home az cache value
func (h *NMAgentMonitor) readCacheValue() cns.GetHomeAzResponse {
	cachedResp, found := h.values.Get(homeAzCacheKey)
	if !found {
		return cns.GetHomeAzResponse{Response: cns.Response{
			ReturnCode: types.NotFound,
			Message:    "HomeAz Cache is unavailable",
		}, HomeAzResponse: cns.HomeAzResponse{IsSupported: false}}
	}
	return cachedResp.(cns.GetHomeAzResponse)
}

// Start starts a new thread to refresh the NMAgent cache.
func (h *NMAgentMonitor) Start() {
	logger.Printf("[NMAgentMonitor] starting cache refresh")
	go h.refresh()
}

// Stop ends the refresh thread
func (h *NMAgentMonitor) Stop() {
	close(h.closing)
}

// refresh periodically pulls capabilities and home az from nmagent.
func (h *NMAgentMonitor) refresh() {
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

// Populate caches RNC support and retrieves HomeAZ if GetHomeAz is supported.
func (h *NMAgentMonitor) Populate(ctx context.Context) {
	supportedApis, err := h.SupportedAPIs(ctx)
	if err != nil {
		h.updateRNCSupport(cns.GetRNCSupportResponse{Response: cns.Response{
			ReturnCode: types.NmAgentSupportedApisError,
			Message:    fmt.Sprintf("failed to query nmagent supported apis: %v", err),
		}})
		returnMessage := fmt.Sprintf("[HomeAzMonitor] failed to query nmagent's supported apis, %v", err)
		returnCode := types.NmAgentSupportedApisError
		h.update(returnCode, returnMessage, cns.HomeAzResponse{IsSupported: false})
		return
	}

	h.updateRNCSupport(cns.GetRNCSupportResponse{
		Response: cns.Response{
			ReturnCode: types.Success,
			Message:    "RNC support query succeeded",
		},
		RNCSupportResponse: cns.RNCSupportResponse{
			IsSupported: isAPISupportedByNMAgent(supportedApis, RNCSupportAPIName),
		},
	})

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

			case http.StatusNotFound:
				returnMessage := fmt.Sprintf("[HomeAzMonitor] region does not support AZs, NMAgent returned StatusCode: %d, error: %v", apiError.StatusCode(), err)
				// Marking this as success since we don't want to enter the retry loop on DNC side.
				returnCode := types.Success
				h.update(returnCode, returnMessage, cns.HomeAzResponse{IsSupported: false})
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

	// validate home az value, HomeAz is a uint, so it's value >=0
	if azResponse.HomeAz == 0 {
		returnMessage := fmt.Sprintf("[HomeAzMonitor] invalid home az value from nmagent: %d", azResponse.HomeAz)
		returnCode := types.UnexpectedError
		h.update(returnCode, returnMessage, cns.HomeAzResponse{IsSupported: true})
		return
	}
	h.update(types.Success, "Get Home Az succeeded", cns.HomeAzResponse{IsSupported: true, HomeAz: azResponse.HomeAz, NmaAppliedTheIPV6Fix: azResponse.ContainsFixes(nmagent.HomeAZFixIPv6)})
}

// update constructs a GetHomeAzResponse entity and update its cache
func (h *NMAgentMonitor) update(code types.ResponseCode, msg string, homeAzResponse cns.HomeAzResponse) {
	resp := cns.GetHomeAzResponse{
		Response: cns.Response{
			ReturnCode: code,
			Message:    msg,
		},
		HomeAzResponse: homeAzResponse,
	}

	// log the response and update the cache if it doesn't match with the current cached value
	if h.readCacheValue() != resp {
		logger.Printf("[HomeAzMonitor] updating home az cache value: %+v", resp)
		h.updateCacheValue(resp)
	}
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
