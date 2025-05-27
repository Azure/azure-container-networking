package main

import (
	"context"
	"time"

	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/restserver"
)

// initSyncHostNncVersion starts a goroutine to periodically poll and sync NC versions for NNC
func initSyncHostNncVersion(ctx context.Context, httpRestServiceImplementation *restserver.HTTPRestService, cnsconfig *configuration.CNSConfig) {
	logger.Printf("Starting SyncHostNCVersion loop.")
	go func() {
		// Periodically poll vfp programmed NC version from NMAgent
		tickerChannel := time.Tick(time.Duration(cnsconfig.SyncHostNCVersionIntervalMs) * time.Millisecond)
		for {
			select {
			case <-tickerChannel:
				timedCtx, cancel := context.WithTimeout(ctx, time.Duration(cnsconfig.SyncHostNCVersionIntervalMs)*time.Millisecond)
				httpRestServiceImplementation.SyncHostNCVersion(timedCtx, cnsconfig.ChannelMode)
				cancel()
			case <-ctx.Done():
				logger.Printf("Stopping SyncHostNCVersion loop.")
				return
			}
		}
	}()
	logger.Printf("Initialized SyncHostNCVersion loop.")
}