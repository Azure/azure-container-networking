// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/wireserver"
	bolt "go.etcd.io/bbolt"
	bolterrors "go.etcd.io/bbolt/errors"
)

// ExportOptions identifies both legacy JSON rollback destinations.
type ExportOptions struct {
	CNSJSONPath      string
	EndpointJSONPath string
}

type rollbackCNSState struct {
	Location                         string                             `json:"Location"`
	NetworkType                      string                             `json:"NetworkType"`
	OrchestratorType                 string                             `json:"OrchestratorType"`
	NodeID                           string                             `json:"NodeID"`
	Initialized                      bool                               `json:"Initialized"`
	ContainerIDByOrchestratorContext map[string]*string                 `json:"ContainerIDByOrchestratorContext"`
	ContainerStatus                  map[string]rollbackContainerStatus `json:"ContainerStatus"`
	Networks                         map[string]*rollbackNetworkInfo    `json:"Networks"`
	TimeStamp                        time.Time                          `json:"TimeStamp"`
	PnpIDByMacAddress                map[string]string                  `json:"PnpIDByMacAddress"`
}

type rollbackContainerStatus struct {
	ID                            string                            `json:"ID"`
	VMVersion                     string                            `json:"VMVersion"`
	HostVersion                   string                            `json:"HostVersion"`
	CreateNetworkContainerRequest cns.CreateNetworkContainerRequest `json:"CreateNetworkContainerRequest"`
	VfpUpdateComplete             bool                              `json:"VfpUpdateComplete"`
}

type rollbackNetworkInfo struct {
	NetworkName string                    `json:"NetworkName"`
	NicInfo     *wireserver.InterfaceInfo `json:"NicInfo"`
	Options     map[string]any            `json:"Options"`
}

type rollbackEndpointInfo struct {
	PodName       string                     `json:"PodName"`
	PodNamespace  string                     `json:"PodNamespace"`
	IfnameToIPMap map[string]*rollbackIPInfo `json:"IfnameToIPMap"`
}

type rollbackIPInfo struct {
	IPv4               []net.IPNet `json:"IPv4"`
	IPv6               []net.IPNet `json:"IPv6,omitempty"`
	HnsEndpointID      string      `json:"HnsEndpointID,omitempty"`
	HnsNetworkID       string      `json:"HnsNetworkID,omitempty"`
	HostVethName       string      `json:"HostVethName,omitempty"`
	MacAddress         string      `json:"MacAddress,omitempty"`
	NetworkContainerID string      `json:"NetworkContainerID,omitempty"`
	NICType            cns.NICType `json:"NICType"`
}

type rollbackTemporaryFile interface {
	io.WriteCloser
	Name() string
	Sync() error
}

type rollbackFileOperations struct {
	mkdirAll       func(string, fs.FileMode) error
	createTemp     func(string, string) (rollbackTemporaryFile, error)
	remove         func(string) error
	durableReplace func(string, string) error
}

func osRollbackFileOperations() rollbackFileOperations {
	return rollbackFileOperations{
		mkdirAll: os.MkdirAll,
		createTemp: func(dir, pattern string) (rollbackTemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		remove:         os.Remove,
		durableReplace: durableReplace,
	}
}

// ExportLegacy converts one validated Bolt snapshot into both legacy JSON files.
// It returns false without touching the files after rollback export is complete.
func (s *DB) ExportLegacy(ctx context.Context, opts ExportOptions) (bool, error) {
	return s.exportLegacy(ctx, opts, osRollbackFileOperations())
}

func (s *DB) exportLegacy(
	ctx context.Context,
	opts ExportOptions,
	files rollbackFileOperations,
) (bool, error) {
	if err := validateExportOptions(opts); err != nil {
		return false, err
	}
	if err := validateRollbackFileOperations(files); err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("exporting legacy state: %w", err)
	}
	if err := s.acquireWriteGate(ctx); err != nil {
		return false, fmt.Errorf("exporting legacy state: %w", err)
	}
	defer s.releaseWriteGate()

	if s.db.IsReadOnly() {
		return false, fmt.Errorf("exporting legacy state: %w", bolterrors.ErrDatabaseReadOnly)
	}

	snapshot, complete, err := s.rollbackSnapshotLocked(ctx)
	if err != nil {
		return false, fmt.Errorf("exporting legacy state: %w", err)
	}
	if complete {
		return false, nil
	}

	cnsData, endpointData, err := encodeRollbackFiles(snapshot)
	if err != nil {
		return false, err
	}
	if err := writeRollbackFile(ctx, files, opts.CNSJSONPath, cnsData); err != nil {
		return false, err
	}
	if err := writeRollbackFile(ctx, files, opts.EndpointJSONPath, endpointData); err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("exporting legacy state: %w", err)
	}

	changed, err := s.updateLocked(ctx, func(tx *WriteTx) (bool, error) {
		meta, err := tx.Metadata()
		if err != nil {
			return false, err
		}
		if err := validateRollbackExportState(meta.Authority, meta.RollbackExportComplete); err != nil {
			return false, err
		}
		if meta.RollbackExportComplete {
			return false, nil
		}
		meta.Authority = AuthorityJSON
		if err := tx.PutMetadata(meta); err != nil {
			return false, err
		}
		if err := setRollbackExportComplete(tx.tx); err != nil {
			return false, err
		}
		if s.exportBeforeCommit != nil {
			if err := s.exportBeforeCommit(); err != nil {
				return false, fmt.Errorf("finishing rollback export: %w", err)
			}
		}
		return true, nil
	})
	if err != nil {
		return false, fmt.Errorf("marking JSON state authoritative: %w", err)
	}
	return changed, nil
}

func validateExportOptions(opts ExportOptions) error {
	if strings.TrimSpace(opts.CNSJSONPath) == "" {
		return invalidInput("rollback CNS path is empty", nil)
	}
	if strings.TrimSpace(opts.EndpointJSONPath) == "" {
		return invalidInput("rollback endpoint path is empty", nil)
	}
	cnsPath, err := filepath.Abs(opts.CNSJSONPath)
	if err != nil {
		return invalidInput("resolving rollback CNS path", err)
	}
	endpointPath, err := filepath.Abs(opts.EndpointJSONPath)
	if err != nil {
		return invalidInput("resolving rollback endpoint path", err)
	}
	if filepath.Clean(cnsPath) == filepath.Clean(endpointPath) {
		return invalidInput("rollback paths must be distinct", nil)
	}
	return nil
}

func validateRollbackFileOperations(files rollbackFileOperations) error {
	if files.mkdirAll == nil || files.createTemp == nil || files.remove == nil || files.durableReplace == nil {
		return invalidInput("rollback file operations are incomplete", nil)
	}
	return nil
}

func (s *DB) rollbackSnapshotLocked(ctx context.Context) (Snapshot, bool, error) {
	var snapshot Snapshot
	var complete bool
	err := s.db.View(func(tx *bolt.Tx) error {
		readTx := &ReadTx{tx: tx, ctx: ctx}
		meta, err := readTx.Metadata()
		if err != nil {
			return err
		}
		if err := validateRollbackExportState(meta.Authority, meta.RollbackExportComplete); err != nil {
			return err
		}
		if meta.RollbackExportComplete {
			complete = true
			return nil
		}
		snapshot, err = snapshotFromTx(ctx, readTx)
		if err != nil {
			return err
		}
		return snapshot.Validate()
	})
	if err != nil {
		return Snapshot{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, false, err
	}
	return snapshot, complete, nil
}

func encodeRollbackFiles(snapshot Snapshot) ([]byte, []byte, error) {
	cnsState := rollbackCNSState{
		Location:                         snapshot.Metadata.Location,
		NetworkType:                      snapshot.Metadata.NetworkType,
		OrchestratorType:                 snapshot.Metadata.OrchestratorType,
		NodeID:                           snapshot.Metadata.NodeID,
		Initialized:                      snapshot.Metadata.Initialized,
		ContainerIDByOrchestratorContext: make(map[string]*string, len(snapshot.OrchestratorContexts)),
		ContainerStatus:                  make(map[string]rollbackContainerStatus, len(snapshot.NetworkContainers)),
		Networks:                         make(map[string]*rollbackNetworkInfo, len(snapshot.Networks)),
		TimeStamp:                        snapshot.Metadata.TimeStamp,
		PnpIDByMacAddress:                snapshot.PnPIDByMAC,
	}
	for contextID, ncIDs := range snapshot.OrchestratorContexts {
		value := strings.Join(ncIDs, ",")
		cnsState.ContainerIDByOrchestratorContext[contextID] = &value
	}
	for name, network := range snapshot.Networks {
		cnsState.Networks[name] = &rollbackNetworkInfo{
			NetworkName: network.NetworkName,
			NicInfo:     network.NicInfo,
			Options:     network.Options,
		}
	}
	for ncID, record := range snapshot.NetworkContainers {
		request := record.Request
		request.AuthorizationToken = ""
		request.SecondaryIPConfigs = map[string]cns.SecondaryIPConfig{}
		for ipID, ip := range snapshot.IPs {
			if ip.NCID != ncID {
				continue
			}
			request.SecondaryIPConfigs[ipID] = cns.SecondaryIPConfig{
				IPAddress: ip.IPAddress,
				NCVersion: ip.NCVersion,
			}
		}
		cnsState.ContainerStatus[ncID] = rollbackContainerStatus{
			ID:                            record.ID,
			VMVersion:                     record.VMVersion,
			HostVersion:                   record.HostVersion,
			CreateNetworkContainerRequest: request,
			VfpUpdateComplete:             record.VFPUpdateComplete,
		}
	}

	endpoints := make(map[string]*rollbackEndpointInfo, len(snapshot.Endpoints))
	for containerID, endpoint := range snapshot.Endpoints {
		legacyEndpoint := &rollbackEndpointInfo{
			PodName:       endpoint.PodName,
			PodNamespace:  endpoint.PodNamespace,
			IfnameToIPMap: make(map[string]*rollbackIPInfo, len(endpoint.IfnameToIPMap)),
		}
		for ifname, info := range endpoint.IfnameToIPMap {
			if info == nil {
				continue
			}
			legacyEndpoint.IfnameToIPMap[ifname] = &rollbackIPInfo{
				IPv4:               info.IPv4,
				IPv6:               info.IPv6,
				HnsEndpointID:      info.HNSEndpointID,
				HnsNetworkID:       info.HNSNetworkID,
				HostVethName:       info.HostVethName,
				MacAddress:         info.MACAddress,
				NetworkContainerID: info.NetworkContainerID,
				NICType:            info.NICType,
			}
		}
		endpoints[containerID] = legacyEndpoint
	}

	cnsData, err := json.MarshalIndent(map[string]any{
		"ContainerNetworkService": cnsState,
	}, "", "\t")
	if err != nil {
		return nil, nil, fmt.Errorf("encoding rollback CNS state: %w", err)
	}
	endpointData, err := json.MarshalIndent(map[string]any{
		"DeleteIntents": snapshot.DeleteIntents,
		"Endpoints":     endpoints,
	}, "", "\t")
	if err != nil {
		return nil, nil, fmt.Errorf("encoding rollback endpoint state: %w", err)
	}
	return cnsData, endpointData, nil
}

func writeRollbackFile(
	ctx context.Context,
	files rollbackFileOperations,
	path string,
	data []byte,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("writing rollback file %q: %w", path, err)
	}
	parent := filepath.Dir(path)
	if err := files.mkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("creating rollback directory %q: %w", parent, err)
	}
	file, err := files.createTemp(parent, filepath.Base(path)+".rollback-*")
	if err != nil {
		return fmt.Errorf("creating rollback temp file for %q: %w", path, err)
	}
	tempPath := file.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = files.remove(tempPath)
		}
	}()

	if err := writeAll(file, data); err != nil {
		_ = file.Close()
		return fmt.Errorf("writing rollback temp file for %q: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("syncing rollback temp file for %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("closing rollback temp file for %q: %w", path, err)
	}
	if err := files.durableReplace(tempPath, path); err != nil {
		return fmt.Errorf("replacing rollback file %q: %w", path, err)
	}
	removeTemp = false
	return nil
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) != 0 {
		written, err := writer.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}
