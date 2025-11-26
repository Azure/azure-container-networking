// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/pkg/errors"
)

// TODO: make this file a sub pacakge?

// NetworkContainerSyncState manages waiting on conflist to ge
// Basically a wait group that can only be added once and waits with a context
// meant to be used uninitialized and then started once the sync loop begins.
type networkContainerSyncState struct {
	wg      sync.WaitGroup
	started atomic.Bool
}

// Start is like add except it only allows being called once.
func (n *networkContainerSyncState) Start() error {
	if !n.started.CompareAndSwap(false, true) {
		return errors.New("sync loop already started")
	}
	n.wg.Add(1)
	return nil
}

// NotifyReady is like Done but will ignore if Start was never called.
func (n *networkContainerSyncState) NotifyReady() {
	if !n.started.Load() {
		return //nobody ever set this up just move on.
	}
	n.wg.Done()
}

// Wait waits for the CNI conflist to be ready or for the context to be done.
func (n *networkContainerSyncState) Wait(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		n.wg.Wait() //still fine to wait even if never started will just return immediately
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}
}

// StartSyncHostNCVersionLoop loops until NCs are programmed and conflist is written The node subnet equivalent isStartNodeSubnet
func (service *HTTPRestService) StartSyncHostNCVersionLoop(ctx context.Context, cnsconfig configuration.CNSConfig) error {
	if err := service.ncSyncState.Start(); err != nil {
		return err
	}
	go func() {
		var one sync.Once
		logger.Printf("Starting SyncHostNCVersion loop.")
		// Periodically poll vfp programmed NC version from NMAgent
		ticker := time.NewTicker(time.Duration(cnsconfig.SyncHostNCVersionIntervalMs) * time.Millisecond)
		timeout := time.Duration(cnsconfig.SyncHostNCVersionIntervalMs) * time.Millisecond
		for {
			timedCtx, cancel := context.WithTimeout(ctx, timeout)
			if service.SyncHostNCVersion(timedCtx, cnsconfig.ChannelMode) {
				one.Do(service.ncSyncState.NotifyReady)
			}
			cancel()
			select {
			case <-ticker.C:
				timedCtx, cancel := context.WithTimeout(ctx, timeout)
				if service.SyncHostNCVersion(timedCtx, cnsconfig.ChannelMode) {
					one.Do(service.ncSyncState.NotifyReady)
				}
				cancel()
			case <-ctx.Done():
				logger.Printf("Stopping SyncHostNCVersion loop.")
				return
			}
		}
	}()
	return nil
}

// TODO: lowercase/unexport this function drive everything through StartSyncHostNCVersionLoop?
// SyncHostNCVersion will check NC version from NMAgent and save it as host NC version in container status.
// If NMAgent NC version got updated, CNS will refresh the pending programming IP status.
// returns true if soemthing was progammeed
func (service *HTTPRestService) SyncHostNCVersion(ctx context.Context, channelMode string) bool {
	service.Lock()
	defer service.Unlock()
	start := time.Now()
	programmedNCCount, err := service.syncHostNCVersion(ctx, channelMode)
	if err != nil {
		logger.Errorf("sync host error %v", err)
	}

	syncHostNCVersionCount.WithLabelValues(strconv.FormatBool(err == nil)).Inc()
	//does not include time to write out conflist.
	syncHostNCVersionLatency.WithLabelValues(strconv.FormatBool(err == nil)).Observe(time.Since(start).Seconds())
	return programmedNCCount > 0
}

var errNonExistentContainerStatus = errors.New("nonExistantContainerstatus")

// syncHostVersion updates the CNS state with the latest programmed versions of NCs attached to the VM. If any NC in local CNS state
// does not match the version that DNC claims to have published, this function will call NMAgent and list the latest programmed versions of
// all NCs and update the CNS state accordingly. This function returns the the total number of NCs on this VM that have been programmed to
// some version, NOT the number of NCs that are up-to-date.
func (service *HTTPRestService) syncHostNCVersion(ctx context.Context, channelMode string) (int, error) {
	outdatedNCs := map[string]struct{}{}
	programmedNCs := map[string]struct{}{}
	for idx := range service.state.ContainerStatus {
		// Will open a separate PR to convert all the NC version related variable to int. Change from string to int is a pain.
		localNCVersion, err := strconv.Atoi(service.state.ContainerStatus[idx].HostVersion)
		if err != nil {
			logger.Errorf("Received err when change containerstatus.HostVersion %s to int, err msg %v", service.state.ContainerStatus[idx].HostVersion, err)
			continue
		}
		dncNCVersion, err := strconv.Atoi(service.state.ContainerStatus[idx].CreateNetworkContainerRequest.Version)
		if err != nil {
			logger.Errorf("Received err when change nc version %s in containerstatus to int, err msg %v", service.state.ContainerStatus[idx].CreateNetworkContainerRequest.Version, err)
			continue
		}
		// host NC version is the NC version from NMAgent, if it's smaller than NC version from DNC, then append it to indicate it needs update.
		if localNCVersion < dncNCVersion {
			outdatedNCs[service.state.ContainerStatus[idx].ID] = struct{}{}
		} else if localNCVersion > dncNCVersion {
			logger.Errorf("NC version from NMAgent is larger than DNC, NC version from NMAgent is %d, NC version from DNC is %d", localNCVersion, dncNCVersion)
		}

		if localNCVersion > -1 {
			programmedNCs[service.state.ContainerStatus[idx].ID] = struct{}{}
		}
	}
	if len(outdatedNCs) == 0 {
		return len(programmedNCs), nil
	}

	ncVersionListResp, err := service.nma.GetNCVersionList(ctx)
	if err != nil {
		return len(programmedNCs), errors.Wrap(err, "failed to get nc version list from nmagent")
	}

	// Get IMDS NC versions for delegated NIC scenarios
	imdsNCVersions, err := service.GetIMDSNCs(ctx)
	if err != nil {
		// If any of the NMA API check calls, imds calls fails assume that nma build doesn't have the latest changes and create empty map
		imdsNCVersions = make(map[string]string)
	}

	nmaNCs := map[string]string{}
	for _, nc := range ncVersionListResp.Containers {
		nmaNCs[strings.ToLower(nc.NetworkContainerID)] = nc.Version
	}

	// Consolidate both nc's from NMA and IMDS calls
	nmaProgrammedNCs := make(map[string]string)
	for ncID, version := range nmaNCs {
		nmaProgrammedNCs[ncID] = version
	}
	for ncID, version := range imdsNCVersions {
		if _, exists := nmaProgrammedNCs[ncID]; !exists {
			nmaProgrammedNCs[strings.ToLower(ncID)] = version
		} else {
			//nolint:staticcheck // SA1019: suppress deprecated logger.Warnf usage. Todo: legacy logger usage is consistent in cns repo. Migrates when all logger usage is migrated
			logger.Warnf("NC %s exists in both NMA and IMDS responses, which is not expected", ncID)
		}
	}
	hasNC.Set(float64(len(nmaProgrammedNCs)))
	for ncID := range outdatedNCs {
		nmaProgrammedNCVersionStr, ok := nmaProgrammedNCs[ncID]
		if !ok {
			// Neither NMA nor IMDS has this NC that we need programmed yet, bail out
			continue
		}
		nmaProgrammedNCVersion, err := strconv.Atoi(nmaProgrammedNCVersionStr)
		if err != nil {
			logger.Errorf("failed to parse container version of %s: %s", ncID, err)
			continue
		}
		// Check whether it exist in service state and get the related nc info
		ncInfo, exist := service.state.ContainerStatus[ncID]
		if !exist {
			// if we marked this NC as needs update, but it no longer exists in internal state when we reach
			// this point, our internal state has changed unexpectedly and we should bail out and try again.
			return len(programmedNCs), errors.Wrapf(errNonExistentContainerStatus, "can't find NC with ID %s in service state, stop updating this host NC version", ncID)
		}
		// if the NC still exists in state and is programmed to some version (doesn't have to be latest), add it to our set of NCs that have been programmed
		if nmaProgrammedNCVersion > -1 {
			programmedNCs[ncID] = struct{}{}
		}

		localNCVersion, err := strconv.Atoi(ncInfo.HostVersion)
		if err != nil {
			logger.Errorf("failed to parse host nc version string %s: %s", ncInfo.HostVersion, err)
			continue
		}
		if localNCVersion > nmaProgrammedNCVersion {
			//nolint:staticcheck // SA1019: suppress deprecated logger.Printf usage. Todo: legacy logger usage is consistent in cns repo. Migrates when all logger usage is migrated
			logger.Errorf("NC version from consolidated sources is decreasing: have %d, got %d", localNCVersion, nmaProgrammedNCVersion)
			continue
		}
		if channelMode == cns.CRD {
			service.MarkIpsAsAvailableUntransacted(ncInfo.ID, nmaProgrammedNCVersion)
		}
		//nolint:staticcheck // SA1019: suppress deprecated logger.Printf usage. Todo: legacy logger usage is consistent in cns repo. Migrates when all logger usage is migrated
		logger.Printf("Updating NC %s host version from %s to %s", ncID, ncInfo.HostVersion, nmaProgrammedNCVersionStr)
		ncInfo.HostVersion = nmaProgrammedNCVersionStr
		logger.Printf("Updated NC %s host version to %s", ncID, ncInfo.HostVersion)
		service.state.ContainerStatus[ncID] = ncInfo
		// if we successfully updated the NC, pop it from the needs update set.
		delete(outdatedNCs, ncID)
	}
	// if we didn't empty out the needs update set, NMA has not programmed all the NCs we are expecting, and we
	// need to return an error indicating that
	if len(outdatedNCs) > 0 {
		return len(programmedNCs), errors.Errorf("Have outdated NCs: %v, Current Programmed nics from NMA/IMDS %v", outdatedNCs, programmedNCs)
	}

	return len(programmedNCs), nil
}
