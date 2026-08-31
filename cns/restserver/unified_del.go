// Copyright 2026 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/state"
	"github.com/Azure/azure-container-networking/cns/types"
)

type unifiedReleasePlan struct {
	pod   state.PodIdentity
	stale bool
}

type unifiedDeleteCommittedError struct {
	err error
}

var (
	errUnifiedReleaseOperationNil = errors.New("unified DEL: endpoint release operation is nil")
	errUnifiedDeleteOperationNil  = errors.New("unified DEL: endpoint deletion operation is nil")
	errUnifiedPruneOperationNil   = errors.New("unified DEL: delete intent prune operation is nil")
)

func (e *unifiedDeleteCommittedError) Error() string {
	return fmt.Sprintf("unified DEL committed but cache projection failed: %v", e.err)
}

func (e *unifiedDeleteCommittedError) Unwrap() error {
	return e.err
}

func (a *durableStateAdapter) releaseIPConfigs(
	ctx context.Context,
	request cns.IPConfigsRequest,
	podInfo cns.PodInfo,
) error {
	if a.store.releaseEndpoint == nil {
		return errUnifiedReleaseOperationNil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.service.Lock()
	defer a.service.Unlock()

	snapshot, err := a.currentSnapshot(ctx)
	if err != nil {
		return err
	}
	plan, err := a.service.releaseIPConfigsUnifiedLocked(ctx, request, podInfo, snapshot)
	if err != nil {
		return err
	}
	if plan.stale {
		return nil
	}

	now := a.now()
	var projection durableCacheProjection
	changed, err := a.store.releaseEndpoint(
		ctx,
		a.generation,
		plan.pod,
		now,
		func(candidate state.Snapshot) error {
			var buildErr error
			projection, buildErr = a.buildProjection(candidate)
			return buildErr
		},
	)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	return a.applyDeleteCommitLocked(ctx, projection)
}

// releaseIPConfigsUnifiedLocked preflights the complete release without
// mutating service state. The caller owns the service lock.
func (service *HTTPRestService) releaseIPConfigsUnifiedLocked(
	ctx context.Context,
	request cns.IPConfigsRequest,
	podInfo cns.PodInfo,
	snapshot state.Snapshot,
) (unifiedReleasePlan, error) {
	if err := ctx.Err(); err != nil {
		return unifiedReleasePlan{}, fmt.Errorf("preflighting unified endpoint release: %w", err)
	}
	requestedPod := state.PodIdentity{
		PodKey:           strings.TrimSpace(podInfo.Key()),
		InfraContainerID: strings.TrimSpace(podInfo.InfraContainerID()),
		InterfaceID:      strings.TrimSpace(podInfo.InterfaceID()),
		PodName:          strings.TrimSpace(podInfo.Name()),
		PodNamespace:     strings.TrimSpace(podInfo.Namespace()),
	}
	if err := validateUnifiedReleasePod(requestedPod); err != nil {
		return unifiedReleasePlan{}, err
	}
	plan := unifiedReleasePlan{pod: requestedPod}

	if endpoint, ok := snapshot.Endpoints[requestedPod.InfraContainerID]; ok {
		if endpoint.PodName != requestedPod.PodName || endpoint.PodNamespace != requestedPod.PodNamespace {
			plan.stale = true
			return plan, nil
		}
		if ifname := strings.TrimSpace(request.Ifname); ifname != "" {
			if _, ok := endpoint.IfnameToIPMap[ifname]; !ok {
				plan.stale = true
				return plan, nil
			}
		}
	}
	if assignment, ok := snapshot.Assignments[requestedPod.PodKey]; ok &&
		assignment.Pod.InfraContainerID != requestedPod.InfraContainerID {
		plan.stale = true
		return plan, nil
	}

	assignmentKeys := make([]string, 0)
	requestedAssignmentExists := false
	currentAddresses := make(map[netip.Addr]struct{})
	for key, assignment := range snapshot.Assignments {
		if assignment.Pod.InfraContainerID != requestedPod.InfraContainerID {
			continue
		}
		if assignment.Pod.PodName != requestedPod.PodName ||
			assignment.Pod.PodNamespace != requestedPod.PodNamespace {
			plan.stale = true
			return plan, nil
		}
		assignmentKeys = append(assignmentKeys, key)
		requestedAssignmentExists = requestedAssignmentExists || key == requestedPod.PodKey
		for _, ipID := range assignment.IPIDs {
			record, ok := snapshot.IPs[ipID]
			if !ok {
				return unifiedReleasePlan{}, fmt.Errorf("%w: assignment IP is missing", state.ErrStaleGeneration)
			}
			address, err := netip.ParseAddr(record.IPAddress)
			if err != nil {
				return unifiedReleasePlan{}, fmt.Errorf("%w: assignment IP address is invalid", state.ErrInvalidInput)
			}
			currentAddresses[address.Unmap()] = struct{}{}
		}
	}
	sort.Strings(assignmentKeys)
	if len(assignmentKeys) != 0 && !requestedAssignmentExists {
		plan.stale = true
		return plan, nil
	}
	if err := service.validateUnifiedReleaseProjectionLocked(snapshot, assignmentKeys); err != nil {
		return unifiedReleasePlan{}, err
	}

	requestedAddresses, err := parseUnifiedReleaseAddresses(request.DesiredIPAddresses)
	if err != nil {
		return unifiedReleasePlan{}, err
	}
	if len(requestedAddresses) == 0 || len(assignmentKeys) == 0 {
		return plan, nil
	}
	matches := 0
	for address := range requestedAddresses {
		if _, ok := currentAddresses[address]; ok {
			matches++
		}
	}
	if matches == 0 {
		plan.stale = true
		return plan, nil
	}
	if matches != len(requestedAddresses) || len(requestedAddresses) != len(currentAddresses) {
		return unifiedReleasePlan{}, fmt.Errorf(
			"%w: release IP set does not match the current assignment",
			state.ErrInvalidInput,
		)
	}
	return plan, nil
}

func validateUnifiedReleasePod(pod state.PodIdentity) error {
	switch {
	case pod.PodKey == "":
		return fmt.Errorf("%w: pod key is empty", state.ErrInvalidInput)
	case pod.InfraContainerID == "":
		return fmt.Errorf("%w: infra container ID is empty", state.ErrInvalidInput)
	case pod.InterfaceID == "" && pod.PodKey != pod.InfraContainerID:
		return fmt.Errorf("%w: pod key must equal infra container ID without an interface ID", state.ErrInvalidInput)
	case pod.InterfaceID != "" && pod.PodKey != pod.InterfaceID:
		return fmt.Errorf("%w: pod key must equal interface ID", state.ErrInvalidInput)
	case pod.PodName == "":
		return fmt.Errorf("%w: pod name is empty", state.ErrInvalidInput)
	case pod.PodNamespace == "":
		return fmt.Errorf("%w: pod namespace is empty", state.ErrInvalidInput)
	default:
		return nil
	}
}

func (service *HTTPRestService) validateUnifiedReleaseProjectionLocked(
	snapshot state.Snapshot,
	assignmentKeys []string,
) error {
	for _, key := range assignmentKeys {
		assignment := snapshot.Assignments[key]
		cachedIDs, ok := service.PodIPIDByPodInterfaceKey[key]
		if !ok || len(cachedIDs) != len(assignment.IPIDs) {
			return fmt.Errorf("%w: projected assignment does not match database", state.ErrStaleGeneration)
		}
		for index, ipID := range assignment.IPIDs {
			if cachedIDs[index] != ipID {
				return fmt.Errorf("%w: projected assignment does not match database", state.ErrStaleGeneration)
			}
			status, ok := service.PodIPConfigState[ipID]
			if !ok || status.GetState() != types.Assigned || status.PodInfo == nil ||
				status.PodInfo.Key() != key {
				return fmt.Errorf("%w: projected IP owner does not match database", state.ErrStaleGeneration)
			}
		}
	}
	return nil
}

func parseUnifiedReleaseAddresses(values []string) (map[netip.Addr]struct{}, error) {
	addresses := make(map[netip.Addr]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" && len(values) == 1 {
			continue
		}
		address, err := netip.ParseAddr(value)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid release IP address", state.ErrInvalidInput)
		}
		address = address.Unmap()
		if _, ok := addresses[address]; ok {
			return nil, fmt.Errorf("%w: duplicate release IP address", state.ErrInvalidInput)
		}
		addresses[address] = struct{}{}
	}
	return addresses, nil
}

func (a *durableStateAdapter) deleteEndpointRecord(ctx context.Context, infraContainerID string) error {
	if a.store.deleteEndpoint == nil {
		return errUnifiedDeleteOperationNil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.service.Lock()
	defer a.service.Unlock()

	snapshot, err := a.currentSnapshot(ctx)
	if err != nil {
		return err
	}
	if _, ok := snapshot.Endpoints[strings.TrimSpace(infraContainerID)]; !ok {
		return ErrEndpointStateNotFound
	}
	var projection durableCacheProjection
	changed, err := a.store.deleteEndpoint(
		ctx,
		a.generation,
		infraContainerID,
		func(candidate state.Snapshot) error {
			var buildErr error
			projection, buildErr = a.buildProjection(candidate)
			return buildErr
		},
	)
	if err != nil {
		return err
	}
	if !changed {
		return ErrEndpointStateNotFound
	}
	return a.applyDeleteCommitLocked(ctx, projection)
}

func (a *durableStateAdapter) pruneDeleteIntents(ctx context.Context, now time.Time) (int, error) {
	if a.store.pruneDeleteIntents == nil {
		return 0, errUnifiedPruneOperationNil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.service.Lock()
	defer a.service.Unlock()

	if _, err := a.currentSnapshot(ctx); err != nil {
		return 0, err
	}
	var projection durableCacheProjection
	count, err := a.store.pruneDeleteIntents(
		ctx,
		a.generation,
		now,
		unifiedDeleteIntentTTL,
		func(candidate state.Snapshot) error {
			var buildErr error
			projection, buildErr = a.buildProjection(candidate)
			return buildErr
		},
	)
	if err != nil || count == 0 {
		return count, err
	}
	if err := a.applyDeleteCommitLocked(ctx, projection); err != nil {
		return 0, err
	}
	return count, nil
}

func (a *durableStateAdapter) applyDeleteCommitLocked(
	ctx context.Context,
	projection durableCacheProjection,
) error {
	apply := a.applyDeleteProjection
	if apply == nil {
		apply = func(value durableCacheProjection) error {
			a.applyProjectionLocked(value)
			return nil
		}
	}
	var postCommitErr error
	if err := apply(projection); err != nil {
		postCommitErr = a.restoreCommittedDeleteProjectionLocked(ctx, projection, err)
	}
	if a.store.refreshMetrics != nil {
		if _, err := a.store.refreshMetrics(context.WithoutCancel(ctx)); err != nil {
			postCommitErr = errors.Join(postCommitErr, fmt.Errorf("refreshing persistent state metrics: %w", err))
		}
	}
	if postCommitErr != nil {
		return &unifiedDeleteCommittedError{err: postCommitErr}
	}
	return nil
}

func (a *durableStateAdapter) restoreCommittedDeleteProjectionLocked(
	ctx context.Context,
	committed durableCacheProjection,
	applyErr error,
) error {
	restoreErr := error(nil)
	snapshot, err := a.store.snapshot(context.WithoutCancel(ctx))
	if err != nil {
		restoreErr = fmt.Errorf("reading committed endpoint deletion: %w", err)
	} else {
		projection, buildErr := a.buildProjection(snapshot)
		if buildErr != nil {
			restoreErr = fmt.Errorf("building committed endpoint deletion projection: %w", buildErr)
		} else {
			committed = projection
		}
	}
	a.applyProjectionLocked(committed)
	return errors.Join(applyErr, restoreErr)
}

func unifiedReleaseResponseCode(err error) types.ResponseCode {
	switch {
	case errors.Is(err, state.ErrInvalidInput):
		return types.InvalidRequest
	case errors.Is(err, state.ErrStaleGeneration):
		return types.InconsistentIPConfigState
	default:
		return types.UnexpectedError
	}
}
