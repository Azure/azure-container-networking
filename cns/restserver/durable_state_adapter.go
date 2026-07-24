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
	mu         sync.Mutex
	projected  bool
	generation uint64
	closeOnce  sync.Once
	closeErr   error
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
	networks          map[string]*networkInfo
	orchestratorNCs   map[string]*ncList
	pnpIDByMACAddress map[string]string
}

func newDurableStateAdapter(service *HTTPRestService, db *state.DB) (*durableStateAdapter, error) {
	if db == nil {
		return nil, errors.New("durable state database is nil")
	}
	return newDurableStateAdapterWithOperations(service, durableStateOperations{
		snapshot: db.Snapshot,
		replace:  db.ReplaceDurableState,
		updateMetadata: func(ctx context.Context, expectedGeneration uint64, metadata state.Metadata) (bool, error) {
			err := db.Update(ctx, func(tx *state.WriteTx) error {
				current, err := tx.Metadata()
				if err != nil {
					return err
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
			return err == nil, err
		},
		status: db.Status,
		close:  db.Close,
	})
}

func newDurableStateAdapterWithOperations(
	service *HTTPRestService,
	operations durableStateOperations,
) (*durableStateAdapter, error) {
	switch {
	case service == nil:
		return nil, errors.New("rest service is nil")
	case service.state == nil:
		return nil, errors.New("rest service state is nil")
	case operations.snapshot == nil:
		return nil, errors.New("durable state snapshot operation is nil")
	case operations.replace == nil:
		return nil, errors.New("durable state replace operation is nil")
	case operations.updateMetadata == nil:
		return nil, errors.New("durable state metadata operation is nil")
	case operations.status == nil:
		return nil, errors.New("durable state status operation is nil")
	case operations.close == nil:
		return nil, errors.New("durable state close operation is nil")
	}
	return &durableStateAdapter{service: service, store: operations}, nil
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
		return fmt.Errorf("%w: cloning network container %q: %v", state.ErrInvalidInput, record.ID, err)
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
		return fmt.Errorf("%w: cloning network %q: %v", state.ErrInvalidInput, record.NetworkName, err)
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
		return fmt.Errorf("%w: invalid MAC address %q: %v", state.ErrInvalidInput, macAddress, err)
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
		return fmt.Errorf("%w: invalid MAC address %q: %v", state.ErrInvalidInput, macAddress, err)
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
		return fmt.Errorf("%w: cloning durable state: %v", state.ErrInvalidInput, err)
	}
	if err := mutate(&durable); err != nil {
		return err
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
		return state.Snapshot{}, errors.New("durable state has not been projected")
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
		return fmt.Errorf("unexpected durable state backend %q", status.Backend)
	case status.Authority != state.AuthorityBolt:
		return fmt.Errorf("unexpected durable state authority %q", status.Authority)
	case status.SchemaVersion != state.SchemaVersion:
		return fmt.Errorf("unexpected durable state schema version %d", status.SchemaVersion)
	case status.InvariantStatus != state.InvariantHealthy:
		return fmt.Errorf("durable state invariant %q failed", status.FailedInvariant)
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
	a.service.PodIPConfigState = projection.ipConfigState
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
		return durableCacheProjection{}, fmt.Errorf(
			"durable state projection schema version %d is invalid",
			snapshot.Metadata.SchemaVersion,
		)
	case snapshot.Metadata.Authority != state.AuthorityBolt:
		return durableCacheProjection{}, fmt.Errorf(
			"durable state projection authority %q is invalid",
			snapshot.Metadata.Authority,
		)
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
		networks:          make(map[string]*networkInfo, len(snapshot.Networks)),
		orchestratorNCs:   make(map[string]*ncList, len(snapshot.OrchestratorContexts)),
		pnpIDByMACAddress: make(map[string]string, len(snapshot.PnPIDByMAC)),
	}

	hostVersions := make(map[string]int, len(snapshot.NetworkContainers))
	for id, record := range snapshot.NetworkContainers {
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
			return durableCacheProjection{}, fmt.Errorf("ip %q references missing projected network container %q", id, record.NCID)
		}
		hostVersion, ok := hostVersions[record.NCID]
		if !ok {
			return durableCacheProjection{}, fmt.Errorf(
				"ip %q network container %q has an empty host version",
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
		ipStatus.WithStateMiddleware(stateTransitionMiddleware)
		ipState := types.Available
		if hostVersion < record.NCVersion {
			ipState = types.PendingProgramming
		}
		ipStatus.SetState(ipState)
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
					"orchestrator context %q network container %q contains a comma",
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
	return projection, nil
}

func cloneJSON[T any](input T) (T, error) {
	var output T
	data, err := json.Marshal(input)
	if err != nil {
		return output, err
	}
	if err := json.Unmarshal(data, &output); err != nil {
		return output, err
	}
	return output, nil
}
