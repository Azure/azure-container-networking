// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package restserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/state"
	"github.com/Azure/azure-container-networking/cns/types"
)

var (
	errNilDurableStateDB           = errors.New("durable state database is nil")
	errNilDurableRestService       = errors.New("rest service is nil")
	errNilDurableRestServiceState  = errors.New("rest service state is nil")
	errNilDurableSnapshotOperation = errors.New("durable state snapshot operation is nil")
	errNilDurableReplaceOperation  = errors.New("durable state replace operation is nil")
	errNilDurableMetadataOperation = errors.New("durable state metadata operation is nil")
	errNilDurableStatusOperation   = errors.New("durable state status operation is nil")
	errNilDurableCloseOperation    = errors.New("durable state close operation is nil")
	errDurableStateNotProjected    = errors.New("durable state has not been projected")
	errUnexpectedDurableBackend    = errors.New("unexpected durable state backend")
	errUnexpectedDurableAuthority  = errors.New("unexpected durable state authority")
	errUnexpectedDurableSchema     = errors.New("unexpected durable state schema version")
	errDurableInvariantFailed      = errors.New("durable state invariant failed")
	errInvalidProjectionSchema     = errors.New("durable state projection schema version is invalid")
	errInvalidProjectionAuthority  = errors.New("durable state projection authority is invalid")
	errMissingProjectedNC          = errors.New("missing projected network container")
	errEmptyProjectedNCHostVersion = errors.New("projected network container host version is empty")
	errProjectedNCIDContainsComma  = errors.New("projected network container ID contains a comma")
)

type durableStateOperations struct {
	snapshot       func(context.Context) (state.Snapshot, error)
	replace        func(context.Context, uint64, state.DurableState) (bool, error)
	updateMetadata func(context.Context, uint64, state.Metadata) (bool, error)
	status         func(context.Context) (state.Status, error)
	close          func() error
}

type durableStateAdapter struct {
	service *HTTPRestService
	store   durableStateOperations

	// mu is acquired before the HTTPRestService lock. Callers must not hold the
	// service lock; the adapter applies complete projections under that lock.
	mu                   sync.Mutex
	projectEndpointState bool
	projected            bool
	generation           uint64
	closeOnce            sync.Once
	closeErr             error
}

type durableServiceMetadata struct {
	OrchestratorType string
	NodeID           string
	Location         string
	NetworkType      string
	Initialized      bool
	TimeStamp        time.Time
}

type durableCacheProjection struct {
	generation        uint64
	metadata          durableServiceMetadata
	containerStatus   map[string]containerstatus
	ipConfigState     map[string]cns.IPConfigurationStatus
	durableIPState    map[string]cns.IPConfigurationStatus
	ipIDsByPodKey     map[string][]string
	endpointState     map[string]*EndpointInfo
	networks          map[string]*networkInfo
	orchestratorNCs   map[string]*ncList
	pnpIDByMACAddress map[string]string
}

// NewDurableStateLifecycle binds the Bolt durable-state adapter to the service
// while leaving lifecycle ownership with the startup coordinator.
func NewDurableStateLifecycle(
	service *HTTPRestService,
	db *state.DB,
	projectEndpointState bool,
) (restore func(context.Context) error, closeFn func() error, err error) {
	adapter, err := newDurableStateAdapter(service, db, projectEndpointState)
	if err != nil {
		return nil, nil, err
	}
	return adapter.restore, adapter.Close, nil
}

func newDurableStateAdapter(
	service *HTTPRestService,
	db *state.DB,
	projectEndpointState bool,
) (*durableStateAdapter, error) {
	if db == nil {
		return nil, errNilDurableStateDB
	}
	return newDurableStateAdapterWithOperations(service, durableStateOperations{
		snapshot: db.Snapshot,
		replace:  db.ReplaceDurableState,
		updateMetadata: func(ctx context.Context, expectedGeneration uint64, metadata state.Metadata) (bool, error) {
			err := db.Update(ctx, func(tx *state.WriteTx) error {
				current, err := tx.Metadata()
				if err != nil {
					return fmt.Errorf("reading durable state metadata: %w", err)
				}
				if current.Generation != expectedGeneration {
					return fmt.Errorf(
						"%w: expected=%d actual=%d",
						state.ErrStaleGeneration,
						expectedGeneration,
						current.Generation,
					)
				}
				return tx.PutMetadata(metadata)
			})
			if err != nil {
				return false, fmt.Errorf("updating durable state metadata: %w", err)
			}
			return true, nil
		},
		status: db.Status,
		close:  db.Close,
	}, projectEndpointState)
}

func newDurableStateAdapterWithOperations(
	service *HTTPRestService,
	operations durableStateOperations,
	projectEndpointState bool,
) (*durableStateAdapter, error) {
	switch {
	case service == nil:
		return nil, errNilDurableRestService
	case service.state == nil:
		return nil, errNilDurableRestServiceState
	case operations.snapshot == nil:
		return nil, errNilDurableSnapshotOperation
	case operations.replace == nil:
		return nil, errNilDurableReplaceOperation
	case operations.updateMetadata == nil:
		return nil, errNilDurableMetadataOperation
	case operations.status == nil:
		return nil, errNilDurableStatusOperation
	case operations.close == nil:
		return nil, errNilDurableCloseOperation
	}
	return &durableStateAdapter{
		service:              service,
		store:                operations,
		projectEndpointState: projectEndpointState,
	}, nil
}

func (a *durableStateAdapter) restore(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	snapshot, err := a.store.snapshot(ctx)
	if err != nil {
		return err
	}
	projection, err := buildDurableCacheProjection(snapshot)
	if err != nil {
		return err
	}
	if err := a.verifyStatus(ctx, projection.generation); err != nil {
		return err
	}
	if a.projected {
		switch {
		case projection.generation == a.generation:
			return nil
		case projection.generation < a.generation:
			return fmt.Errorf(
				"%w: projected=%d database=%d",
				state.ErrStaleGeneration,
				a.generation,
				projection.generation,
			)
		}
	}
	a.applyProjection(projection)
	return nil
}

func (a *durableStateAdapter) applyNetworkContainer(
	ctx context.Context,
	record state.NetworkContainerRecord,
	ips []state.IPRecord,
) error {
	if record.Request.NetworkContainerid != record.ID {
		return fmt.Errorf(
			"%w: network container ID %q does not match request ID %q",
			state.ErrInvalidInput,
			record.ID,
			record.Request.NetworkContainerid,
		)
	}
	normalized := state.NewNetworkContainerRecord(
		record.ID,
		record.VMVersion,
		record.HostVersion,
		record.VFPUpdateComplete,
		record.Request,
	)
	if record.ID == "" {
		return fmt.Errorf("%w: network container ID is empty", state.ErrInvalidInput)
	}
	normalized, err := cloneJSON(normalized)
	if err != nil {
		return fmt.Errorf("%w: cloning network container %q: %w", state.ErrInvalidInput, record.ID, err)
	}
	inventory := make(map[string]state.IPRecord, len(ips))
	for _, ip := range ips {
		switch {
		case ip.ID == "":
			return fmt.Errorf("%w: IP ID is empty", state.ErrInvalidInput)
		case ip.NCID != record.ID:
			return fmt.Errorf(
				"%w: IP %q NC %q does not match network container %q",
				state.ErrInvalidInput,
				ip.ID,
				ip.NCID,
				record.ID,
			)
		}
		if _, exists := inventory[ip.ID]; exists {
			return fmt.Errorf("%w: duplicate IP ID %q", state.ErrInvalidInput, ip.ID)
		}
		inventory[ip.ID] = ip
	}
	return a.updateDurable(ctx, func(candidate *state.DurableState) error {
		candidate.NetworkContainers[record.ID] = normalized
		for id, ip := range candidate.IPs {
			if ip.NCID == record.ID {
				delete(candidate.IPs, id)
			}
		}
		for id, ip := range inventory {
			candidate.IPs[id] = ip
		}
		return nil
	})
}

func (a *durableStateAdapter) deleteNetworkContainer(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("%w: network container ID is empty", state.ErrInvalidInput)
	}
	return a.updateDurable(ctx, func(candidate *state.DurableState) error {
		delete(candidate.NetworkContainers, id)
		for ipID, ip := range candidate.IPs {
			if ip.NCID == id {
				delete(candidate.IPs, ipID)
			}
		}
		return nil
	})
}

func (a *durableStateAdapter) putNetwork(ctx context.Context, record state.NetworkRecord) error {
	if record.NetworkName == "" {
		return fmt.Errorf("%w: network name is empty", state.ErrInvalidInput)
	}
	cloned, err := cloneJSON(record)
	if err != nil {
		return fmt.Errorf("%w: cloning network %q: %w", state.ErrInvalidInput, record.NetworkName, err)
	}
	return a.updateDurable(ctx, func(candidate *state.DurableState) error {
		candidate.Networks[record.NetworkName] = cloned
		return nil
	})
}

func (a *durableStateAdapter) deleteNetwork(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("%w: network name is empty", state.ErrInvalidInput)
	}
	return a.updateDurable(ctx, func(candidate *state.DurableState) error {
		delete(candidate.Networks, name)
		return nil
	})
}

func (a *durableStateAdapter) putOrchestratorContext(ctx context.Context, id string, ncIDs []string) error {
	if id == "" {
		return fmt.Errorf("%w: orchestrator context ID is empty", state.ErrInvalidInput)
	}
	ids := append([]string{}, ncIDs...)
	return a.updateDurable(ctx, func(candidate *state.DurableState) error {
		candidate.OrchestratorContexts[id] = ids
		return nil
	})
}

func (a *durableStateAdapter) deleteOrchestratorContext(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("%w: orchestrator context ID is empty", state.ErrInvalidInput)
	}
	return a.updateDurable(ctx, func(candidate *state.DurableState) error {
		delete(candidate.OrchestratorContexts, id)
		return nil
	})
}

func (a *durableStateAdapter) putPnPID(ctx context.Context, macAddress, pnpID string) error {
	mac, err := net.ParseMAC(macAddress)
	if err != nil {
		return fmt.Errorf("%w: invalid MAC address %q: %w", state.ErrInvalidInput, macAddress, err)
	}
	if pnpID == "" {
		return fmt.Errorf("%w: PnP ID is empty", state.ErrInvalidInput)
	}
	return a.updateDurable(ctx, func(candidate *state.DurableState) error {
		candidate.PnPIDByMAC[mac.String()] = pnpID
		return nil
	})
}

func (a *durableStateAdapter) deletePnPID(ctx context.Context, macAddress string) error {
	mac, err := net.ParseMAC(macAddress)
	if err != nil {
		return fmt.Errorf("%w: invalid MAC address %q: %w", state.ErrInvalidInput, macAddress, err)
	}
	return a.updateDurable(ctx, func(candidate *state.DurableState) error {
		delete(candidate.PnPIDByMAC, mac.String())
		return nil
	})
}

func (a *durableStateAdapter) putServiceMetadata(ctx context.Context, metadata durableServiceMetadata) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	snapshot, err := a.currentSnapshot(ctx)
	if err != nil {
		return err
	}
	candidate := snapshot
	candidate.Metadata.OrchestratorType = metadata.OrchestratorType
	candidate.Metadata.NodeID = metadata.NodeID
	candidate.Metadata.Location = metadata.Location
	candidate.Metadata.NetworkType = metadata.NetworkType
	candidate.Metadata.Initialized = metadata.Initialized
	candidate.Metadata.TimeStamp = metadata.TimeStamp
	projection, err := buildDurableCacheProjection(candidate)
	if err != nil {
		return err
	}
	changed, err := a.store.updateMetadata(ctx, a.generation, candidate.Metadata)
	if err != nil {
		return err
	}
	generation, err := a.committedGeneration(ctx, changed)
	if err != nil {
		return err
	}
	projection.generation = generation
	a.applyProjection(projection)
	return nil
}

func (a *durableStateAdapter) updateDurable(
	ctx context.Context,
	mutate func(*state.DurableState) error,
) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	snapshot, err := a.currentSnapshot(ctx)
	if err != nil {
		return err
	}
	durable, err := cloneJSON(state.DurableState{
		NetworkContainers:    snapshot.NetworkContainers,
		IPs:                  snapshot.IPs,
		Networks:             snapshot.Networks,
		OrchestratorContexts: snapshot.OrchestratorContexts,
		PnPIDByMAC:           snapshot.PnPIDByMAC,
	})
	if err != nil {
		return fmt.Errorf("%w: cloning durable state: %w", state.ErrInvalidInput, err)
	}
	if mutateErr := mutate(&durable); mutateErr != nil {
		return mutateErr
	}
	candidate := snapshot
	candidate.NetworkContainers = durable.NetworkContainers
	candidate.IPs = durable.IPs
	candidate.Networks = durable.Networks
	candidate.OrchestratorContexts = durable.OrchestratorContexts
	candidate.PnPIDByMAC = durable.PnPIDByMAC
	projection, err := buildDurableCacheProjection(candidate)
	if err != nil {
		return err
	}
	changed, err := a.store.replace(ctx, a.generation, durable)
	if err != nil {
		return err
	}
	generation, err := a.committedGeneration(ctx, changed)
	if err != nil {
		return err
	}
	projection.generation = generation
	a.applyProjection(projection)
	return nil
}

func (a *durableStateAdapter) currentSnapshot(ctx context.Context) (state.Snapshot, error) {
	if !a.projected {
		return state.Snapshot{}, errDurableStateNotProjected
	}
	snapshot, err := a.store.snapshot(ctx)
	if err != nil {
		return state.Snapshot{}, err
	}
	if snapshot.Metadata.Generation != a.generation {
		return state.Snapshot{}, fmt.Errorf(
			"%w: projected=%d database=%d",
			state.ErrStaleGeneration,
			a.generation,
			snapshot.Metadata.Generation,
		)
	}
	return snapshot, nil
}

func (a *durableStateAdapter) committedGeneration(ctx context.Context, changed bool) (uint64, error) {
	expected := a.generation
	if changed {
		expected++
	}
	if err := a.verifyStatus(ctx, expected); err != nil {
		return 0, err
	}
	return expected, nil
}

func (a *durableStateAdapter) verifyStatus(ctx context.Context, expectedGeneration uint64) error {
	status, err := a.store.status(ctx)
	if err != nil {
		return err
	}
	switch {
	case status.Backend != state.BackendBolt:
		return fmt.Errorf("%w: %q", errUnexpectedDurableBackend, status.Backend)
	case status.Authority != state.AuthorityBolt:
		return fmt.Errorf("%w: %q", errUnexpectedDurableAuthority, status.Authority)
	case status.SchemaVersion != state.SchemaVersion:
		return fmt.Errorf("%w: %d", errUnexpectedDurableSchema, status.SchemaVersion)
	case status.InvariantStatus != state.InvariantHealthy:
		return fmt.Errorf("%w: %q", errDurableInvariantFailed, status.FailedInvariant)
	case status.Generation != expectedGeneration:
		return fmt.Errorf(
			"%w: expected=%d actual=%d",
			state.ErrStaleGeneration,
			expectedGeneration,
			status.Generation,
		)
	}
	return nil
}

func (a *durableStateAdapter) applyProjection(projection durableCacheProjection) {
	a.service.Lock()
	defer a.service.Unlock()

	a.service.state.OrchestratorType = projection.metadata.OrchestratorType
	a.service.state.NodeID = projection.metadata.NodeID
	a.service.state.Location = projection.metadata.Location
	a.service.state.NetworkType = projection.metadata.NetworkType
	a.service.state.Initialized = projection.metadata.Initialized
	a.service.state.TimeStamp = projection.metadata.TimeStamp
	a.service.state.ContainerStatus = projection.containerStatus
	a.service.state.ContainerIDByOrchestratorContext = projection.orchestratorNCs
	a.service.state.Networks = projection.networks
	a.service.state.PnpIDByMacAddress = projection.pnpIDByMACAddress
	if a.projectEndpointState {
		// Delete intents have no legacy cache equivalent. They remain authoritative
		// in the unified database and are intentionally absent from this projection.
		a.service.PodIPConfigState = projection.ipConfigState
		a.service.PodIPIDByPodInterfaceKey = projection.ipIDsByPodKey
		a.service.EndpointState = projection.endpointState
	} else {
		a.service.PodIPConfigState = projection.durableIPState
	}
	a.generation = projection.generation
	a.projected = true
}

func (a *durableStateAdapter) cacheGeneration() (uint64, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.generation, a.projected
}

func (a *durableStateAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closeOnce.Do(func() {
		a.closeErr = a.store.close()
	})
	return a.closeErr
}

func buildDurableCacheProjection(snapshot state.Snapshot) (durableCacheProjection, error) {
	if err := snapshot.Validate(); err != nil {
		return durableCacheProjection{}, fmt.Errorf("validating durable state projection: %w", err)
	}
	switch {
	case snapshot.Metadata.SchemaVersion != state.SchemaVersion:
		return durableCacheProjection{}, fmt.Errorf("%w: %d", errInvalidProjectionSchema, snapshot.Metadata.SchemaVersion)
	case snapshot.Metadata.Authority != state.AuthorityBolt:
		return durableCacheProjection{}, fmt.Errorf("%w: %q", errInvalidProjectionAuthority, snapshot.Metadata.Authority)
	}

	projection := durableCacheProjection{
		generation: snapshot.Metadata.Generation,
		metadata: durableServiceMetadata{
			OrchestratorType: snapshot.Metadata.OrchestratorType,
			NodeID:           snapshot.Metadata.NodeID,
			Location:         snapshot.Metadata.Location,
			NetworkType:      snapshot.Metadata.NetworkType,
			Initialized:      snapshot.Metadata.Initialized,
			TimeStamp:        snapshot.Metadata.TimeStamp,
		},
		containerStatus:   make(map[string]containerstatus, len(snapshot.NetworkContainers)),
		ipConfigState:     make(map[string]cns.IPConfigurationStatus, len(snapshot.IPs)),
		durableIPState:    make(map[string]cns.IPConfigurationStatus, len(snapshot.IPs)),
		ipIDsByPodKey:     make(map[string][]string, len(snapshot.Assignments)),
		endpointState:     make(map[string]*EndpointInfo, len(snapshot.Endpoints)),
		networks:          make(map[string]*networkInfo, len(snapshot.Networks)),
		orchestratorNCs:   make(map[string]*ncList, len(snapshot.OrchestratorContexts)),
		pnpIDByMACAddress: make(map[string]string, len(snapshot.PnPIDByMAC)),
	}

	hostVersions := make(map[string]int, len(snapshot.NetworkContainers))
	unassignedIPStates := make(map[string]types.IPState, len(snapshot.IPs))
	for id := range snapshot.NetworkContainers {
		record := snapshot.NetworkContainers[id]
		request, err := cloneJSON(record.Request)
		if err != nil {
			return durableCacheProjection{}, fmt.Errorf("cloning network container %q request: %w", id, err)
		}
		request.SecondaryIPConfigs = make(map[string]cns.SecondaryIPConfig)
		projection.containerStatus[id] = containerstatus{
			ID:                            record.ID,
			VMVersion:                     record.VMVersion,
			HostVersion:                   record.HostVersion,
			CreateNetworkContainerRequest: request,
			VfpUpdateComplete:             record.VFPUpdateComplete,
		}
		if record.HostVersion != "" {
			hostVersion, err := strconv.Atoi(record.HostVersion)
			if err != nil {
				return durableCacheProjection{}, fmt.Errorf(
					"converting network container %q host version %q: %w",
					id,
					record.HostVersion,
					err,
				)
			}
			hostVersions[id] = hostVersion
		}
	}

	for id, record := range snapshot.IPs {
		nc, ok := projection.containerStatus[record.NCID]
		if !ok {
			return durableCacheProjection{}, fmt.Errorf("%w: IP %q network container %q", errMissingProjectedNC, id, record.NCID)
		}
		hostVersion, ok := hostVersions[record.NCID]
		if !ok {
			return durableCacheProjection{}, fmt.Errorf(
				"%w: IP %q network container %q",
				errEmptyProjectedNCHostVersion,
				id,
				record.NCID,
			)
		}
		nc.CreateNetworkContainerRequest.SecondaryIPConfigs[id] = cns.SecondaryIPConfig{
			IPAddress: record.IPAddress,
			NCVersion: record.NCVersion,
		}
		projection.containerStatus[record.NCID] = nc

		ipStatus := cns.IPConfigurationStatus{
			ID:        id,
			IPAddress: record.IPAddress,
			NCID:      record.NCID,
		}
		ipState := types.Available
		if hostVersion < record.NCVersion {
			ipState = types.PendingProgramming
		}
		unassignedIPStates[id] = ipState
		projection.ipConfigState[id] = ipStatus
	}

	for name, record := range snapshot.Networks {
		cloned, err := cloneJSON(record)
		if err != nil {
			return durableCacheProjection{}, fmt.Errorf("cloning network %q: %w", name, err)
		}
		projection.networks[name] = &networkInfo{
			NetworkName: cloned.NetworkName,
			NicInfo:     cloned.NicInfo,
			Options:     cloned.Options,
		}
	}
	for id, ncIDs := range snapshot.OrchestratorContexts {
		for _, ncID := range ncIDs {
			if strings.Contains(ncID, ",") {
				return durableCacheProjection{}, fmt.Errorf(
					"%w: orchestrator context %q network container %q",
					errProjectedNCIDContainsComma,
					id,
					ncID,
				)
			}
		}
		value := ncList(strings.Join(ncIDs, ","))
		projection.orchestratorNCs[id] = &value
	}
	for macAddress, pnpID := range snapshot.PnPIDByMAC {
		mac, err := net.ParseMAC(macAddress)
		if err != nil {
			return durableCacheProjection{}, fmt.Errorf("parsing PnP MAC address %q: %w", macAddress, err)
		}
		projection.pnpIDByMACAddress[mac.String()] = pnpID
	}
	for containerID, record := range snapshot.Endpoints {
		endpoint, err := projectEndpoint(record)
		if err != nil {
			return durableCacheProjection{}, fmt.Errorf("projecting endpoint %q: %w", containerID, err)
		}
		projection.endpointState[containerID] = endpoint
	}
	for podKey, assignment := range snapshot.Assignments {
		if assignment.Pod.PodKey != podKey {
			return durableCacheProjection{}, fmt.Errorf(
				"assignment key %q does not match projected pod key %q",
				podKey,
				assignment.Pod.PodKey,
			)
		}
		endpoint, ok := snapshot.Endpoints[assignment.Pod.InfraContainerID]
		if !ok {
			return durableCacheProjection{}, fmt.Errorf(
				"assignment %q references missing projected endpoint %q",
				podKey,
				assignment.Pod.InfraContainerID,
			)
		}
		podInfo := newProjectedPodInfo(assignment.Pod, len(endpoint.IfnameToIPMap) > 1)
		ipIDs := append([]string(nil), assignment.IPIDs...)
		for _, ipID := range ipIDs {
			ipStatus, ok := projection.ipConfigState[ipID]
			if !ok {
				return durableCacheProjection{}, fmt.Errorf(
					"assignment %q references missing projected IP %q",
					podKey,
					ipID,
				)
			}
			owner, ok := snapshot.IPOwners[ipID]
			if !ok {
				return durableCacheProjection{}, fmt.Errorf(
					"assignment %q IP %q has no projected owner",
					podKey,
					ipID,
				)
			}
			if owner != podKey {
				return durableCacheProjection{}, fmt.Errorf(
					"assignment %q IP %q is owned by %q",
					podKey,
					ipID,
					owner,
				)
			}
			ipStatus.PodInfo = podInfo
			ipStatus.SetState(types.Assigned)
			ipStatus.WithStateMiddleware(stateTransitionMiddleware)
			projection.ipConfigState[ipID] = ipStatus
		}
		projection.ipIDsByPodKey[podKey] = ipIDs
	}
	for ipID, owner := range snapshot.IPOwners {
		assignment, ok := snapshot.Assignments[owner]
		if !ok {
			return durableCacheProjection{}, fmt.Errorf(
				"IP %q references missing projected assignment %q",
				ipID,
				owner,
			)
		}
		if !containsString(assignment.IPIDs, ipID) {
			return durableCacheProjection{}, fmt.Errorf(
				"IP %q owner %q does not contain the IP",
				ipID,
				owner,
			)
		}
	}
	for ipID, ipStatus := range projection.ipConfigState {
		durableStatus := cns.IPConfigurationStatus{
			ID:        ipStatus.ID,
			IPAddress: ipStatus.IPAddress,
			NCID:      ipStatus.NCID,
		}
		durableStatus.SetState(unassignedIPStates[ipID])
		durableStatus.WithStateMiddleware(stateTransitionMiddleware)
		projection.durableIPState[ipID] = durableStatus
		if ipStatus.PodInfo == nil {
			projection.ipConfigState[ipID] = durableStatus
		}
	}
	return projection, nil
}

type projectedPodInfo struct {
	podKey           string
	infraContainerID string
	interfaceID      string
	name             string
	namespace        string
	secondary        bool
}

func newProjectedPodInfo(pod state.PodIdentity, secondary bool) *projectedPodInfo {
	return &projectedPodInfo{
		podKey:           pod.PodKey,
		infraContainerID: pod.InfraContainerID,
		interfaceID:      pod.InterfaceID,
		name:             pod.PodName,
		namespace:        pod.PodNamespace,
		secondary:        secondary,
	}
}

func (p *projectedPodInfo) InfraContainerID() string {
	return p.infraContainerID
}

func (p *projectedPodInfo) InterfaceID() string {
	return p.interfaceID
}

func (p *projectedPodInfo) Key() string {
	return p.podKey
}

func (p *projectedPodInfo) Name() string {
	return p.name
}

func (p *projectedPodInfo) Namespace() string {
	return p.namespace
}

func (p *projectedPodInfo) OrchestratorContext() (json.RawMessage, error) {
	return json.Marshal(cns.KubernetesPodInfo{
		PodName:      p.name,
		PodNamespace: p.namespace,
	})
}

func (p *projectedPodInfo) MarshalJSON() ([]byte, error) {
	version := any(cns.InfraIDPodInfoScheme)
	if p.interfaceID != "" {
		version = cns.InterfaceIDPodInfoScheme
	}
	return json.Marshal(struct {
		cns.KubernetesPodInfo
		PodInfraContainerID   string
		PodInterfaceID        string
		Version               any
		SecondaryInterfaceSet bool
	}{
		KubernetesPodInfo: cns.KubernetesPodInfo{
			PodName:      p.name,
			PodNamespace: p.namespace,
		},
		PodInfraContainerID:   p.infraContainerID,
		PodInterfaceID:        p.interfaceID,
		Version:               version,
		SecondaryInterfaceSet: p.secondary,
	})
}

func (p *projectedPodInfo) Equals(other cns.PodInfo) bool {
	return other != nil && p.podKey == other.Key()
}

func (p *projectedPodInfo) String() string {
	return fmt.Sprintf(
		"InfraContainerID: [%s], InterfaceID: [%s], Key: [%s], Name: [%s], Namespace: [%s]",
		p.infraContainerID,
		p.interfaceID,
		p.podKey,
		p.name,
		p.namespace,
	)
}

func (p *projectedPodInfo) SecondaryInterfacesExist() bool {
	return p.secondary
}

func projectEndpoint(record state.EndpointRecord) (*EndpointInfo, error) {
	endpoint := &EndpointInfo{
		PodName:       record.PodName,
		PodNamespace:  record.PodNamespace,
		IfnameToIPMap: make(map[string]*IPInfo, len(record.IfnameToIPMap)),
	}
	for ifname, infoRecord := range record.IfnameToIPMap {
		if infoRecord == nil {
			return nil, fmt.Errorf("interface %q is nil", ifname)
		}
		endpoint.IfnameToIPMap[ifname] = &IPInfo{
			IPv4:               cloneIPNets(infoRecord.IPv4),
			IPv6:               cloneIPNets(infoRecord.IPv6),
			HnsEndpointID:      infoRecord.HNSEndpointID,
			HnsNetworkID:       infoRecord.HNSNetworkID,
			HostVethName:       infoRecord.HostVethName,
			MacAddress:         infoRecord.MACAddress,
			NetworkContainerID: infoRecord.NetworkContainerID,
			NICType:            infoRecord.NICType,
		}
	}
	return endpoint, nil
}

func cloneIPNets(values []net.IPNet) []net.IPNet {
	cloned := make([]net.IPNet, len(values))
	for i := range values {
		cloned[i] = net.IPNet{
			IP:   append(net.IP(nil), values[i].IP...),
			Mask: append(net.IPMask(nil), values[i].Mask...),
		}
	}
	return cloned
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func cloneJSON[T any](input T) (T, error) {
	var output T
	data, err := json.Marshal(input)
	if err != nil {
		return output, fmt.Errorf("marshaling cloned value: %w", err)
	}
	if err := json.Unmarshal(data, &output); err != nil {
		return output, fmt.Errorf("unmarshaling cloned value: %w", err)
	}
	return output, nil
}
