// Copyright 2026 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strings"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/state"
	"github.com/Azure/azure-container-networking/cns/types"
)

var (
	errNilEndpointPatchOperation = errors.New("durable state endpoint patch operation is nil")
	errDuplicatePatchPrefix      = errors.New("duplicate endpoint patch prefix")
	errInvalidPatchIP            = errors.New("invalid endpoint patch IP")
	errInvalidPatchMaskSize      = errors.New("invalid endpoint patch mask size")
	errIPv6InIPv4Patch           = errors.New("ipv6 address in ipv4 endpoint patch")
	errIPv4InIPv6Patch           = errors.New("ipv4 address in ipv6 endpoint patch")
	errInvalidPatchPrefixLength  = errors.New("invalid endpoint patch prefix length")
)

type unifiedPatchPlan struct {
	pod      state.PodIdentity
	endpoint state.EndpointRecord
}

type unifiedPatchCommittedError struct {
	err error
}

func (e *unifiedPatchCommittedError) Error() string {
	return fmt.Sprintf("unified PATCH committed but cache projection failed: %v", e.err)
}

func (e *unifiedPatchCommittedError) Unwrap() error {
	return e.err
}

func (a *durableStateAdapter) patchEndpoint(
	ctx context.Context,
	infraContainerID string,
	request map[string]*IPInfo,
) error {
	if a.store.patchEndpoint == nil {
		return errNilEndpointPatchOperation
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.service.Lock()
	defer a.service.Unlock()

	snapshot, err := a.currentSnapshot(ctx)
	if err != nil {
		return err
	}
	now := a.now()
	if intent, ok := snapshot.DeleteIntents[strings.TrimSpace(infraContainerID)]; ok &&
		now.Before(intent.CreatedAt.Add(unifiedDeleteIntentTTL)) {
		return fmt.Errorf("%w: infra container %q", state.ErrDeleteIntent, strings.TrimSpace(infraContainerID))
	}
	if projectionErr := a.service.validateUnifiedPatchProjectionLocked(
		snapshot,
		strings.TrimSpace(infraContainerID),
	); projectionErr != nil {
		return projectionErr
	}
	plan, err := buildUnifiedPatchPlan(ctx, infraContainerID, request, snapshot)
	if err != nil {
		return err
	}

	a.service.reachFaultPoint(
		faultPointPatchBeforeEndpointCommit,
		plan.pod.PodName,
		plan.pod.PodNamespace,
	)

	var projection durableCacheProjection
	changed, err := a.store.patchEndpoint(
		ctx,
		a.generation,
		plan.pod,
		plan.endpoint,
		now,
		unifiedDeleteIntentTTL,
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

	apply := a.applyPatchProjection
	if apply == nil {
		apply = func(value durableCacheProjection) error {
			a.applyProjectionLocked(value)
			return nil
		}
	}
	var postCommitErr error
	if err := apply(projection); err != nil {
		postCommitErr = a.restoreCommittedPatchProjectionLocked(ctx, projection, err)
	}
	if a.store.refreshMetrics != nil {
		if _, err := a.store.refreshMetrics(context.WithoutCancel(ctx)); err != nil {
			postCommitErr = errors.Join(postCommitErr, fmt.Errorf("refreshing persistent state metrics: %w", err))
		}
	}
	if postCommitErr != nil {
		return &unifiedPatchCommittedError{err: postCommitErr}
	}
	return nil
}

func (a *durableStateAdapter) restoreCommittedPatchProjectionLocked(
	ctx context.Context,
	committed durableCacheProjection,
	applyErr error,
) error {
	var restoreErr error
	snapshot, err := a.store.snapshot(context.WithoutCancel(ctx))
	if err != nil {
		restoreErr = fmt.Errorf("reading committed endpoint patch: %w", err)
	} else {
		projection, buildErr := a.buildProjection(snapshot)
		if buildErr != nil {
			restoreErr = fmt.Errorf("building committed endpoint patch projection: %w", buildErr)
		} else {
			committed = projection
		}
	}
	a.applyProjectionLocked(committed)
	return errors.Join(applyErr, restoreErr)
}

func (service *HTTPRestService) validateUnifiedPatchProjectionLocked(
	snapshot state.Snapshot,
	infraContainerID string,
) error {
	record, ok := snapshot.Endpoints[infraContainerID]
	if !ok {
		return nil
	}
	projected, err := projectEndpoint(record)
	if err != nil {
		return fmt.Errorf("%w: projecting endpoint cache: %w", state.ErrInvalidInput, err)
	}
	cached, ok := service.EndpointState[infraContainerID]
	if !ok {
		return fmt.Errorf("%w: projected endpoint is missing from cache", state.ErrStaleGeneration)
	}
	projectedData, err := json.Marshal(projected) //nolint:musttag // EndpointInfo is the existing endpoint API wire type.
	if err != nil {
		return fmt.Errorf("%w: encoding projected endpoint cache: %w", state.ErrInvalidInput, err)
	}
	cachedData, err := json.Marshal(cached) //nolint:musttag // EndpointInfo is the existing endpoint API wire type.
	if err != nil {
		return fmt.Errorf("%w: encoding endpoint cache: %w", state.ErrInvalidInput, err)
	}
	if !bytes.Equal(projectedData, cachedData) {
		return fmt.Errorf("%w: projected endpoint cache does not match database", state.ErrStaleGeneration)
	}
	return nil
}

func buildUnifiedPatchPlan(
	ctx context.Context,
	infraContainerID string,
	request map[string]*IPInfo,
	snapshot state.Snapshot,
) (unifiedPatchPlan, error) {
	if err := ctx.Err(); err != nil {
		return unifiedPatchPlan{}, fmt.Errorf("planning endpoint patch: %w", err)
	}
	infraContainerID = strings.TrimSpace(infraContainerID)
	if infraContainerID == "" {
		return unifiedPatchPlan{}, fmt.Errorf("%w: infra container ID is empty", state.ErrInvalidInput)
	}
	existing, ok := snapshot.Endpoints[infraContainerID]
	if !ok {
		return unifiedPatchPlan{}, fmt.Errorf("%w: endpoint %q", state.ErrNotFound, infraContainerID)
	}
	if len(request) == 0 {
		return unifiedPatchPlan{}, fmt.Errorf("%w: endpoint patch has no interfaces", state.ErrInvalidInput)
	}
	candidate, err := cloneJSON(existing)
	if err != nil {
		return unifiedPatchPlan{}, fmt.Errorf("%w: cloning endpoint: %w", state.ErrInvalidInput, err)
	}

	ifnames := make([]string, 0, len(request))
	for ifname := range request {
		ifnames = append(ifnames, ifname)
	}
	sort.Strings(ifnames)
	var pod state.PodIdentity
	for _, rawIfname := range ifnames {
		if err := ctx.Err(); err != nil {
			return unifiedPatchPlan{}, fmt.Errorf("planning endpoint interface %q patch: %w", rawIfname, err)
		}
		ifname := strings.TrimSpace(rawIfname)
		if ifname == "" || ifname != rawIfname {
			return unifiedPatchPlan{}, fmt.Errorf("%w: endpoint interface name is invalid", state.ErrInvalidInput)
		}
		patch := request[rawIfname]
		if patch == nil {
			return unifiedPatchPlan{}, fmt.Errorf("%w: interface %q is null", state.ErrInvalidInput, ifname)
		}
		current, ok := existing.IfnameToIPMap[ifname]
		if !ok || current == nil {
			return unifiedPatchPlan{}, fmt.Errorf(
				"%w: endpoint %q interface %q does not exist",
				state.ErrInvalidInput,
				infraContainerID,
				ifname,
			)
		}
		interfacePod, err := validateUnifiedPatchOwnership(snapshot, infraContainerID, existing, *current)
		if err != nil {
			return unifiedPatchPlan{}, err
		}
		if pod.PodKey == "" {
			pod = interfacePod
		}
		updated, err := applyUnifiedIPInfoPatch(snapshot, ifname, *current, *patch)
		if err != nil {
			return unifiedPatchPlan{}, err
		}
		candidate.IfnameToIPMap[ifname] = &updated
	}
	if pod.PodKey == "" {
		return unifiedPatchPlan{}, fmt.Errorf("%w: endpoint ownership is missing", state.ErrNotFound)
	}
	return unifiedPatchPlan{pod: pod, endpoint: candidate}, nil
}

func validateUnifiedPatchOwnership(
	snapshot state.Snapshot,
	infraContainerID string,
	endpoint state.EndpointRecord,
	info state.IPInfoRecord,
) (state.PodIdentity, error) {
	recordsByAddress := make(map[netip.Addr]state.IPRecord, len(snapshot.IPs))
	for _, record := range snapshot.IPs {
		address, err := netip.ParseAddr(record.IPAddress)
		if err != nil {
			return state.PodIdentity{}, fmt.Errorf("%w: endpoint IP inventory is invalid", state.ErrInvalidInput)
		}
		recordsByAddress[address.Unmap()] = record
	}
	var ownerPod state.PodIdentity
	for _, family := range []struct {
		bits     int
		prefixes []net.IPNet
	}{
		{bits: 32, prefixes: info.IPv4},
		{bits: 128, prefixes: info.IPv6},
	} {
		for _, prefix := range family.prefixes {
			address, _, err := parseUnifiedPatchPrefix(prefix, family.bits)
			if err != nil {
				return state.PodIdentity{}, err
			}
			record, ok := recordsByAddress[address]
			if !ok {
				return state.PodIdentity{}, fmt.Errorf("%w: endpoint IP is missing", state.ErrNotFound)
			}
			owner, ok := snapshot.IPOwners[record.ID]
			if !ok {
				return state.PodIdentity{}, fmt.Errorf("%w: endpoint IP owner is missing", state.ErrNotFound)
			}
			assignment, ok := snapshot.Assignments[owner]
			if !ok {
				return state.PodIdentity{}, fmt.Errorf("%w: endpoint assignment is missing", state.ErrNotFound)
			}
			if assignment.Pod.InfraContainerID != infraContainerID ||
				assignment.Pod.PodName != endpoint.PodName ||
				assignment.Pod.PodNamespace != endpoint.PodNamespace {
				return state.PodIdentity{}, fmt.Errorf(
					"%w: endpoint ownership identity does not match",
					state.ErrInvalidInput,
				)
			}
			if ownerPod.PodKey == "" {
				ownerPod = assignment.Pod
			} else if ownerPod != assignment.Pod {
				return state.PodIdentity{}, fmt.Errorf(
					"%w: endpoint interface spans assignments",
					state.ErrInvalidInput,
				)
			}
		}
	}
	if ownerPod.PodKey == "" {
		return state.PodIdentity{}, fmt.Errorf("%w: endpoint interface owner is missing", state.ErrNotFound)
	}
	return ownerPod, nil
}

func applyUnifiedIPInfoPatch(
	snapshot state.Snapshot,
	ifname string,
	current state.IPInfoRecord,
	patch IPInfo,
) (state.IPInfoRecord, error) {
	updated, err := cloneJSON(current)
	if err != nil {
		return state.IPInfoRecord{}, fmt.Errorf("%w: cloning interface %q: %w", state.ErrInvalidInput, ifname, err)
	}
	for _, family := range []struct {
		name     string
		bits     int
		request  []net.IPNet
		existing []net.IPNet
	}{
		{name: "IPv4", bits: 32, request: patch.IPv4, existing: current.IPv4},
		{name: "IPv6", bits: 128, request: patch.IPv6, existing: current.IPv6},
	} {
		if len(family.request) == 0 {
			continue
		}
		equal, compareErr := equalUnifiedPatchPrefixes(family.request, family.existing, family.bits)
		if compareErr != nil {
			return state.IPInfoRecord{}, fmt.Errorf(
				"%w: interface %q %s prefixes are invalid: %w",
				state.ErrInvalidInput,
				ifname,
				family.name,
				compareErr,
			)
		}
		if !equal {
			return state.IPInfoRecord{}, fmt.Errorf(
				"%w: interface %q cannot change assigned %s addresses",
				state.ErrInvalidInput,
				ifname,
				family.name,
			)
		}
	}

	if patch.HnsEndpointID != "" {
		updated.HNSEndpointID, err = normalizedUnifiedPatchValue(patch.HnsEndpointID, "HNS endpoint ID")
		if err != nil {
			return state.IPInfoRecord{}, err
		}
	}
	if patch.HnsNetworkID != "" {
		updated.HNSNetworkID, err = normalizedUnifiedPatchValue(patch.HnsNetworkID, "HNS network ID")
		if err != nil {
			return state.IPInfoRecord{}, err
		}
	}
	if patch.HostVethName != "" {
		updated.HostVethName, err = normalizedUnifiedPatchValue(patch.HostVethName, "host veth name")
		if err != nil {
			return state.IPInfoRecord{}, err
		}
	}
	if patch.MacAddress != "" {
		mac, parseErr := net.ParseMAC(strings.TrimSpace(patch.MacAddress))
		if parseErr != nil {
			return state.IPInfoRecord{}, fmt.Errorf("%w: interface %q MAC address is invalid", state.ErrInvalidInput, ifname)
		}
		updated.MACAddress = mac.String()
	}
	if patch.NetworkContainerID != "" {
		networkContainerID, valueErr := normalizedUnifiedPatchValue(
			patch.NetworkContainerID,
			"network container ID",
		)
		if valueErr != nil {
			return state.IPInfoRecord{}, valueErr
		}
		if _, ok := snapshot.NetworkContainers[networkContainerID]; !ok {
			return state.IPInfoRecord{}, fmt.Errorf(
				"%w: interface %q network container %q does not exist",
				state.ErrInvalidInput,
				ifname,
				networkContainerID,
			)
		}
		updated.NetworkContainerID = networkContainerID
	}
	if patch.NICType != "" {
		if !validUnifiedPatchNICType(patch.NICType) {
			return state.IPInfoRecord{}, fmt.Errorf("%w: interface %q NIC type is invalid", state.ErrInvalidInput, ifname)
		}
		updated.NICType = patch.NICType
	}
	return updated, nil
}

func normalizedUnifiedPatchValue(value, name string) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", fmt.Errorf("%w: %s is empty", state.ErrInvalidInput, name)
	}
	return normalized, nil
}

func validUnifiedPatchNICType(value cns.NICType) bool {
	switch value {
	case cns.InfraNIC,
		cns.DelegatedVMNIC,
		cns.BackendNIC,
		cns.NodeNetworkInterfaceAccelnetFrontendNIC,
		cns.ApipaNIC:
		return true
	default:
		return false
	}
}

type unifiedPatchPrefix struct {
	address netip.Addr
	bits    int
}

func equalUnifiedPatchPrefixes(left, right []net.IPNet, expectedBits int) (bool, error) {
	leftSet, err := unifiedPatchPrefixSet(left, expectedBits)
	if err != nil {
		return false, err
	}
	rightSet, err := unifiedPatchPrefixSet(right, expectedBits)
	if err != nil {
		return false, err
	}
	if len(leftSet) != len(rightSet) {
		return false, nil
	}
	for prefix := range leftSet {
		if _, ok := rightSet[prefix]; !ok {
			return false, nil
		}
	}
	return true, nil
}

func unifiedPatchPrefixSet(values []net.IPNet, expectedBits int) (map[unifiedPatchPrefix]struct{}, error) {
	result := make(map[unifiedPatchPrefix]struct{}, len(values))
	for _, value := range values {
		address, bits, err := parseUnifiedPatchPrefix(value, expectedBits)
		if err != nil {
			return nil, err
		}
		prefix := unifiedPatchPrefix{address: address, bits: bits}
		if _, ok := result[prefix]; ok {
			return nil, errDuplicatePatchPrefix
		}
		result[prefix] = struct{}{}
	}
	return result, nil
}

func parseUnifiedPatchPrefix(value net.IPNet, expectedBits int) (netip.Addr, int, error) {
	address, ok := netip.AddrFromSlice(value.IP)
	if !ok {
		return netip.Addr{}, 0, errInvalidPatchIP
	}
	address = address.Unmap()
	ones, bits := value.Mask.Size()
	if bits != expectedBits {
		return netip.Addr{}, 0, fmt.Errorf("%w: got %d bits, expected %d", errInvalidPatchMaskSize, bits, expectedBits)
	}
	if expectedBits == 32 && !address.Is4() {
		return netip.Addr{}, 0, errIPv6InIPv4Patch
	}
	if expectedBits == 128 && !address.Is6() {
		return netip.Addr{}, 0, errIPv4InIPv6Patch
	}
	if !netip.PrefixFrom(address, ones).IsValid() {
		return netip.Addr{}, 0, errInvalidPatchPrefixLength
	}
	return address, ones, nil
}

func unifiedPatchResponseCode(err error) types.ResponseCode {
	switch {
	case errors.Is(err, state.ErrNotFound):
		return types.NotFound
	case errors.Is(err, state.ErrStaleGeneration):
		return types.InconsistentIPConfigState
	case errors.Is(err, state.ErrInvalidInput):
		return types.InvalidRequest
	default:
		return types.UnexpectedError
	}
}
