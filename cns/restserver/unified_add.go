// Copyright 2026 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/state"
	"github.com/Azure/azure-container-networking/cns/types"
)

const unifiedDeleteIntentTTL = 10 * time.Minute

var errUnifiedIPUnavailable = errors.New("unified ADD: IP unavailable")

type unifiedAddPlan struct {
	assignment state.AssignmentRecord
	endpoint   state.EndpointRecord
	podIPInfo  []cns.PodIpInfo
	replay     bool
}

type unifiedIPCandidate struct {
	record  state.IPRecord
	status  cns.IPConfigurationStatus
	address netip.Addr
}

type unifiedAddCommittedError struct {
	err error
}

func (e *unifiedAddCommittedError) Error() string {
	return fmt.Sprintf("unified ADD committed but cache projection failed: %v", e.err)
}

func (e *unifiedAddCommittedError) Unwrap() error {
	return e.err
}

func (service *HTTPRestService) setUnifiedStateAdapter(adapter *durableStateAdapter) {
	service.Lock()
	defer service.Unlock()
	service.unifiedStateAdapter = adapter
}

func (service *HTTPRestService) selectedUnifiedStateAdapter() *durableStateAdapter {
	service.RLock()
	defer service.RUnlock()
	return service.unifiedStateAdapter
}

func (a *durableStateAdapter) requestIPConfigs(
	ctx context.Context,
	request cns.IPConfigsRequest,
	podInfo cns.PodInfo,
) ([]cns.PodIpInfo, error) {
	if a.store.assignEndpoint == nil {
		return nil, errors.New("durable state endpoint assignment operation is nil")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.service.Lock()
	defer a.service.Unlock()

	snapshot, err := a.currentSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	plan, err := a.service.requestIPConfigsUnifiedLocked(ctx, request, podInfo, snapshot)
	if err != nil {
		return nil, err
	}
	if plan.replay {
		return plan.podIPInfo, nil
	}

	var projection durableCacheProjection
	changed, err := a.store.assignEndpoint(
		ctx,
		a.generation,
		plan.assignment,
		plan.endpoint,
		a.now(),
		unifiedDeleteIntentTTL,
		func(candidate state.Snapshot) error {
			var buildErr error
			projection, buildErr = a.buildProjection(candidate)
			return buildErr
		},
	)
	if err != nil {
		return nil, err
	}
	if !changed {
		return nil, fmt.Errorf("%w: endpoint assignment unexpectedly made no change", state.ErrStaleGeneration)
	}

	apply := a.applyAddProjection
	if apply == nil {
		apply = func(value durableCacheProjection) error {
			a.applyProjectionLocked(value)
			return nil
		}
	}
	var postCommitErr error
	if err := apply(projection); err != nil {
		postCommitErr = a.restoreCommittedAddProjectionLocked(ctx, projection, err)
	}
	if a.store.refreshMetrics != nil {
		if _, err := a.store.refreshMetrics(context.WithoutCancel(ctx)); err != nil {
			postCommitErr = errors.Join(postCommitErr, fmt.Errorf("refreshing persistent state metrics: %w", err))
		}
	}
	if postCommitErr != nil {
		return nil, &unifiedAddCommittedError{err: postCommitErr}
	}
	return plan.podIPInfo, nil
}

func (a *durableStateAdapter) restoreCommittedAddProjectionLocked(
	ctx context.Context,
	committed durableCacheProjection,
	applyErr error,
) error {
	restoreErr := error(nil)
	snapshot, err := a.store.snapshot(context.WithoutCancel(ctx))
	if err != nil {
		restoreErr = fmt.Errorf("reading committed endpoint assignment: %w", err)
	} else {
		projection, buildErr := a.buildProjection(snapshot)
		if buildErr != nil {
			restoreErr = fmt.Errorf("building committed endpoint assignment projection: %w", buildErr)
		} else {
			committed = projection
		}
	}
	a.applyProjectionLocked(committed)
	return errors.Join(applyErr, restoreErr)
}

// requestIPConfigsUnifiedLocked builds and preflights a complete ADD without
// mutating service state. The caller owns the service lock.
func (service *HTTPRestService) requestIPConfigsUnifiedLocked(
	ctx context.Context,
	request cns.IPConfigsRequest,
	podInfo cns.PodInfo,
	snapshot state.Snapshot,
) (unifiedAddPlan, error) {
	if err := ctx.Err(); err != nil {
		return unifiedAddPlan{}, err
	}
	requestedPod := state.PodIdentity{
		PodKey:           podInfo.Key(),
		InfraContainerID: podInfo.InfraContainerID(),
		InterfaceID:      podInfo.InterfaceID(),
		PodName:          podInfo.Name(),
		PodNamespace:     podInfo.Namespace(),
	}
	if existing, ok := snapshot.Assignments[requestedPod.PodKey]; ok {
		if existing.Pod != requestedPod {
			return unifiedAddPlan{}, fmt.Errorf(
				"%w: existing assignment identity does not match request",
				state.ErrInvalidInput,
			)
		}
		endpoint, ok := snapshot.Endpoints[requestedPod.InfraContainerID]
		if !ok {
			return unifiedAddPlan{}, fmt.Errorf(
				"%w: existing assignment endpoint is missing",
				state.ErrStaleGeneration,
			)
		}
		if request.Ifname != "" {
			if _, ok := endpoint.IfnameToIPMap[strings.TrimSpace(request.Ifname)]; !ok {
				return unifiedAddPlan{}, fmt.Errorf(
					"%w: existing assignment interface does not match request",
					state.ErrInvalidInput,
				)
			}
		}
		responses, err := service.podIPInfoForAssignmentLocked(ctx, snapshot, existing)
		if err != nil {
			return unifiedAddPlan{}, err
		}
		return unifiedAddPlan{
			assignment: existing,
			endpoint:   endpoint,
			podIPInfo:  responses,
			replay:     true,
		}, nil
	}

	ifname := strings.TrimSpace(request.Ifname)
	if ifname == "" {
		return unifiedAddPlan{}, fmt.Errorf("%w: interface name is empty", state.ErrInvalidInput)
	}
	candidates, err := service.selectUnifiedIPCandidatesLocked(request, snapshot)
	if err != nil {
		return unifiedAddPlan{}, err
	}
	responses := make([]cns.PodIpInfo, 0, len(candidates))
	ipIDs := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		response, err := service.podIPInfoForCandidateLocked(ctx, candidate)
		if err != nil {
			return unifiedAddPlan{}, err
		}
		responses = append(responses, response)
		ipIDs = append(ipIDs, candidate.record.ID)
	}

	endpoint, err := buildUnifiedEndpoint(snapshot, requestedPod, ifname, candidates, responses)
	if err != nil {
		return unifiedAddPlan{}, err
	}
	return unifiedAddPlan{
		assignment: state.AssignmentRecord{Pod: requestedPod, IPIDs: ipIDs},
		endpoint:   endpoint,
		podIPInfo:  responses,
	}, nil
}

func (service *HTTPRestService) selectUnifiedIPCandidatesLocked(
	request cns.IPConfigsRequest,
	snapshot state.Snapshot,
) ([]unifiedIPCandidate, error) {
	inventory, byAddress, err := service.unifiedIPInventoryLocked(snapshot)
	if err != nil {
		return nil, err
	}
	if len(request.DesiredIPAddresses) != 0 {
		selected := make([]unifiedIPCandidate, 0, len(request.DesiredIPAddresses))
		seen := make(map[netip.Addr]struct{}, len(request.DesiredIPAddresses))
		for _, rawAddress := range request.DesiredIPAddresses {
			address, err := netip.ParseAddr(rawAddress)
			if err != nil {
				return nil, fmt.Errorf("%w: invalid desired IP address", state.ErrInvalidInput)
			}
			address = address.Unmap()
			if _, ok := seen[address]; ok {
				return nil, fmt.Errorf("%w: duplicate desired IP address", state.ErrInvalidInput)
			}
			seen[address] = struct{}{}
			candidate, ok := byAddress[address]
			if !ok {
				return nil, fmt.Errorf("%w: desired IP was not found", errUnifiedIPUnavailable)
			}
			if owner, ok := snapshot.IPOwners[candidate.record.ID]; ok {
				return nil, fmt.Errorf("%w: desired IP is owned by %q", state.ErrIPAlreadyAssigned, owner)
			}
			switch candidate.status.GetState() {
			case types.Available, types.PendingProgramming:
				selected = append(selected, candidate)
			default:
				return nil, fmt.Errorf("%w: desired IP is not available", errUnifiedIPUnavailable)
			}
		}
		return selected, nil
	}

	if len(service.state.ContainerStatus) == 0 {
		return nil, ErrNoNCs
	}
	families := service.getIPFamiliesMap()
	selectedByFamily := make(map[cns.IPFamily]unifiedIPCandidate, len(families))
	for _, candidate := range inventory {
		if candidate.status.GetState() != types.Available {
			continue
		}
		if _, owned := snapshot.IPOwners[candidate.record.ID]; owned {
			continue
		}
		family := cns.IPv6
		if candidate.address.Is4() {
			family = cns.IPv4
		}
		if _, wanted := families[family]; !wanted {
			continue
		}
		if _, exists := selectedByFamily[family]; !exists {
			selectedByFamily[family] = candidate
		}
	}
	selected := make([]unifiedIPCandidate, 0, len(families))
	for _, family := range []cns.IPFamily{cns.IPv4, cns.IPv6} {
		if _, wanted := families[family]; !wanted {
			continue
		}
		candidate, ok := selectedByFamily[family]
		if !ok {
			return nil, fmt.Errorf("%w: not enough %s IPs are available", errUnifiedIPUnavailable, family)
		}
		selected = append(selected, candidate)
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("%w: no IP families are available", errUnifiedIPUnavailable)
	}
	return selected, nil
}

func (service *HTTPRestService) unifiedIPInventoryLocked(
	snapshot state.Snapshot,
) ([]unifiedIPCandidate, map[netip.Addr]unifiedIPCandidate, error) {
	ids := make([]string, 0, len(service.PodIPConfigState))
	for id := range service.PodIPConfigState {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	inventory := make([]unifiedIPCandidate, 0, len(ids))
	byAddress := make(map[netip.Addr]unifiedIPCandidate, len(ids))
	for _, id := range ids {
		status := service.PodIPConfigState[id]
		record, ok := snapshot.IPs[id]
		if !ok || status.ID != record.ID || status.IPAddress != record.IPAddress || status.NCID != record.NCID {
			return nil, nil, fmt.Errorf("%w: projected IP inventory does not match database", state.ErrStaleGeneration)
		}
		address, err := netip.ParseAddr(record.IPAddress)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: projected IP address is invalid", state.ErrInvalidInput)
		}
		candidate := unifiedIPCandidate{record: record, status: status, address: address.Unmap()}
		if previous, exists := byAddress[candidate.address]; exists && previous.record.ID != candidate.record.ID {
			return nil, nil, fmt.Errorf("%w: projected IP inventory contains a duplicate address", state.ErrStaleGeneration)
		}
		inventory = append(inventory, candidate)
		byAddress[candidate.address] = candidate
	}
	return inventory, byAddress, nil
}

func (service *HTTPRestService) podIPInfoForAssignmentLocked(
	ctx context.Context,
	snapshot state.Snapshot,
	assignment state.AssignmentRecord,
) ([]cns.PodIpInfo, error) {
	responses := make([]cns.PodIpInfo, 0, len(assignment.IPIDs))
	for _, ipID := range assignment.IPIDs {
		record, ok := snapshot.IPs[ipID]
		if !ok {
			return nil, fmt.Errorf("%w: assignment IP is missing", state.ErrStaleGeneration)
		}
		status, ok := service.PodIPConfigState[ipID]
		if !ok || status.GetState() != types.Assigned || status.PodInfo == nil ||
			status.PodInfo.Key() != assignment.Pod.PodKey {
			return nil, fmt.Errorf("%w: projected assignment does not match database", state.ErrStaleGeneration)
		}
		address, err := netip.ParseAddr(record.IPAddress)
		if err != nil {
			return nil, fmt.Errorf("%w: assignment IP address is invalid", state.ErrInvalidInput)
		}
		response, err := service.podIPInfoForCandidateLocked(ctx, unifiedIPCandidate{
			record: record, status: status, address: address.Unmap(),
		})
		if err != nil {
			return nil, err
		}
		responses = append(responses, response)
	}
	return responses, nil
}

func (service *HTTPRestService) podIPInfoForCandidateLocked(
	ctx context.Context,
	candidate unifiedIPCandidate,
) (cns.PodIpInfo, error) {
	ncStatus, ok := service.state.ContainerStatus[candidate.record.NCID]
	if !ok {
		return cns.PodIpInfo{}, fmt.Errorf("%w: selected IP references a missing NC", state.ErrInvalidInput)
	}
	primaryIPConfig := ncStatus.CreateNetworkContainerRequest.IPConfiguration
	primaryHostInterface, err := service.getPrimaryHostInterface(ctx)
	if err != nil {
		return cns.PodIpInfo{}, err
	}
	return cns.PodIpInfo{
		PodIPConfig: cns.IPSubnet{
			IPAddress:    candidate.record.IPAddress,
			PrefixLength: primaryIPConfig.IPSubnet.PrefixLength,
		},
		NetworkContainerPrimaryIPConfig: primaryIPConfig,
		HostPrimaryIPInfo: cns.HostIPInfo{
			PrimaryIP: primaryHostInterface.PrimaryIP,
			Subnet:    primaryHostInterface.Subnet,
			Gateway:   primaryHostInterface.Gateway,
		},
		MacAddress:        ncStatus.CreateNetworkContainerRequest.NetworkInterfaceInfo.MACAddress,
		NICType:           cns.InfraNIC,
		SkipDefaultRoutes: ncStatus.CreateNetworkContainerRequest.SkipDefaultRoutes,
	}, nil
}

func buildUnifiedEndpoint(
	snapshot state.Snapshot,
	pod state.PodIdentity,
	ifname string,
	candidates []unifiedIPCandidate,
	responses []cns.PodIpInfo,
) (state.EndpointRecord, error) {
	endpoint := state.EndpointRecord{
		PodName:       pod.PodName,
		PodNamespace:  pod.PodNamespace,
		IfnameToIPMap: map[string]*state.IPInfoRecord{},
	}
	if existing, ok := snapshot.Endpoints[pod.InfraContainerID]; ok {
		if existing.PodName != pod.PodName || existing.PodNamespace != pod.PodNamespace {
			return state.EndpointRecord{}, fmt.Errorf(
				"%w: existing endpoint identity does not match request",
				state.ErrInvalidInput,
			)
		}
		var err error
		endpoint, err = cloneJSON(existing)
		if err != nil {
			return state.EndpointRecord{}, fmt.Errorf("%w: cloning endpoint: %v", state.ErrInvalidInput, err)
		}
	}
	if _, exists := endpoint.IfnameToIPMap[ifname]; exists {
		return state.EndpointRecord{}, fmt.Errorf(
			"%w: endpoint interface is already assigned",
			state.ErrInvalidInput,
		)
	}
	info := &state.IPInfoRecord{NICType: cns.InfraNIC}
	commonNCID := ""
	for index, candidate := range candidates {
		if index == 0 {
			commonNCID = candidate.record.NCID
		} else if commonNCID != candidate.record.NCID {
			commonNCID = ""
		}
		prefixLength := int(responses[index].PodIPConfig.PrefixLength)
		bits := 128
		target := &info.IPv6
		if candidate.address.Is4() {
			bits = 32
			target = &info.IPv4
		}
		prefix := net.IPNet{
			IP:   net.IP(candidate.address.AsSlice()),
			Mask: net.CIDRMask(prefixLength, bits),
		}
		if !containsIPNet(*target, prefix) {
			*target = append(*target, prefix)
		}
	}
	if info.NetworkContainerID == "" {
		info.NetworkContainerID = commonNCID
	}
	endpoint.IfnameToIPMap[ifname] = info
	return endpoint, nil
}

func containsIPNet(values []net.IPNet, target net.IPNet) bool {
	targetOnes, targetBits := target.Mask.Size()
	for _, value := range values {
		ones, bits := value.Mask.Size()
		if value.IP.Equal(target.IP) && ones == targetOnes && bits == targetBits {
			return true
		}
	}
	return false
}

func unifiedAddResponseCode(err error) types.ResponseCode {
	switch {
	case errors.Is(err, state.ErrIPAlreadyAssigned),
		errors.Is(err, state.ErrDeleteIntent),
		errors.Is(err, errUnifiedIPUnavailable):
		return types.AddressUnavailable
	case errors.Is(err, state.ErrStaleGeneration):
		return types.InconsistentIPConfigState
	case errors.Is(err, state.ErrInvalidInput):
		return types.InvalidRequest
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return types.FailedToAllocateIPConfig
	default:
		return types.UnexpectedError
	}
}
