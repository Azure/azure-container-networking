// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/wireserver"
	"github.com/google/uuid"
)

var (
	// ErrLegacyImportTargetNotEmpty indicates that import cannot replace existing unmarked state.
	ErrLegacyImportTargetNotEmpty = errors.New("cns state: legacy import target is not empty")
	// ErrLegacyImportSource indicates that a legacy source is malformed or inconsistent.
	ErrLegacyImportSource = errors.New("cns state: invalid legacy import source")
)

// ImportOptions identifies the legacy JSON sources and imported boot identity.
type ImportOptions struct {
	CNSPath             string
	EndpointPath        string
	ManageEndpointState bool
	BootID              string
}

type readFileFunc func(string) ([]byte, error)

type legacyCNSState struct {
	Location                         string
	NetworkType                      string
	OrchestratorType                 string
	NodeID                           string
	Initialized                      bool
	ContainerIDByOrchestratorContext map[string]*string
	ContainerStatus                  map[string]legacyContainerStatus
	Networks                         map[string]*legacyNetworkInfo
	TimeStamp                        time.Time
	PnpIDByMacAddress                map[string]string
}

type legacyContainerStatus struct {
	ID                            string
	VMVersion                     string
	HostVersion                   string
	CreateNetworkContainerRequest cns.CreateNetworkContainerRequest
	VfpUpdateComplete             bool
}

type legacyNetworkInfo struct {
	NetworkName string
	NicInfo     *wireserver.InterfaceInfo
	Options     map[string]any
}

// ImportLegacy atomically imports legacy JSON state into an initialized empty database.
// It returns false without reading the sources after a completed import.
func (s *DB) ImportLegacy(ctx context.Context, opts ImportOptions) (bool, error) {
	return s.importLegacy(ctx, opts, os.ReadFile)
}

func (s *DB) importLegacy(ctx context.Context, opts ImportOptions, readFile readFileFunc) (bool, error) {
	return s.importLegacyWithCommitHook(ctx, opts, readFile, nil)
}

func (s *DB) importLegacyWithCommitHook(
	ctx context.Context,
	opts ImportOptions,
	readFile readFileFunc,
	beforeCommit func() error,
) (bool, error) {
	if err := validateImportOptions(opts); err != nil {
		return false, err
	}
	if readFile == nil {
		return false, invalidInput("legacy state reader is nil", nil)
	}
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("importing legacy state: %w", err)
	}
	complete, err := s.importComplete(ctx)
	if err != nil {
		return false, err
	}
	if complete {
		return false, nil
	}

	cnsData, err := readLegacyFile(ctx, readFile, opts.CNSPath, "CNS")
	if err != nil {
		return false, err
	}
	snapshot, err := parseLegacyCNS(cnsData, opts.BootID)
	if err != nil {
		return false, err
	}
	if opts.ManageEndpointState {
		endpointData, err := readLegacyFile(ctx, readFile, opts.EndpointPath, "endpoint")
		if err != nil {
			return false, err
		}
		if err := addLegacyEndpoints(&snapshot, endpointData); err != nil {
			return false, err
		}
	}
	if err := snapshot.Validate(); err != nil {
		return false, fmt.Errorf("%w: validating imported snapshot: %w", ErrLegacyImportSource, err)
	}
	encoded, err := encodeImportedSnapshot(snapshot)
	if err != nil {
		return false, err
	}

	return s.update(ctx, func(tx *WriteTx) (bool, error) {
		complete, err := legacyImportComplete(tx.tx.Bucket(bucketMetadata))
		if err != nil {
			return false, err
		}
		if complete {
			return false, nil
		}
		if err := requireEmptyImportTarget(tx); err != nil {
			return false, err
		}
		for _, replacement := range encoded.buckets {
			if err := replaceJSONBucket(tx.tx, replacement.name, replacement.values); err != nil {
				return false, err
			}
		}
		meta := tx.tx.Bucket(bucketMetadata)
		if err := meta.Put(metaKeyBootID, []byte(snapshot.Metadata.BootID)); err != nil {
			return false, fmt.Errorf("writing import boot ID: %w", err)
		}
		if err := meta.Put(metaKeyAuthority, []byte(AuthorityBolt)); err != nil {
			return false, fmt.Errorf("writing import authority: %w", err)
		}
		if err := meta.Put(metaKeyService, encoded.serviceMetadata); err != nil {
			return false, fmt.Errorf("writing imported service metadata: %w", err)
		}
		if err := meta.Put(metaKeyLegacyImport, []byte(legacyImportMarker)); err != nil {
			return false, fmt.Errorf("writing legacy import marker: %w", err)
		}
		if beforeCommit != nil {
			if err := beforeCommit(); err != nil {
				return false, fmt.Errorf("finishing legacy import: %w", err)
			}
		}
		return true, nil
	})
}

func validateImportOptions(opts ImportOptions) error {
	if strings.TrimSpace(opts.CNSPath) == "" {
		return invalidInput("legacy CNS path is empty", nil)
	}
	if strings.TrimSpace(opts.BootID) == "" {
		return invalidInput("import boot ID is empty", nil)
	}
	if opts.ManageEndpointState && strings.TrimSpace(opts.EndpointPath) == "" {
		return invalidInput("legacy endpoint path is empty while endpoint state is managed", nil)
	}
	return nil
}

func readLegacyFile(ctx context.Context, readFile readFileFunc, path, name string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("reading legacy %s state: %w", name, err)
	}
	data, err := readFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading legacy %s state %q: %w", name, path, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("reading legacy %s state: %w", name, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("%w: legacy %s state is empty", ErrLegacyImportSource, name)
	}
	return data, nil
}

func parseLegacyCNS(data []byte, bootID string) (Snapshot, error) {
	var source legacyCNSState
	if err := decodeLegacyEnvelope(data, "ContainerNetworkService", &source); err != nil {
		return Snapshot{}, fmt.Errorf("%w: decoding legacy CNS state: %w", ErrLegacyImportSource, err)
	}
	if source.ContainerStatus == nil {
		return Snapshot{}, fmt.Errorf("%w: legacy CNS container status is null", ErrLegacyImportSource)
	}
	if source.ContainerIDByOrchestratorContext == nil {
		return Snapshot{}, fmt.Errorf("%w: legacy CNS orchestrator contexts are null", ErrLegacyImportSource)
	}
	if source.Networks == nil {
		return Snapshot{}, fmt.Errorf("%w: legacy CNS networks are null", ErrLegacyImportSource)
	}
	if source.PnpIDByMacAddress == nil {
		return Snapshot{}, fmt.Errorf("%w: legacy CNS PnP mappings are null", ErrLegacyImportSource)
	}
	if source.TimeStamp.IsZero() {
		return Snapshot{}, fmt.Errorf("%w: legacy CNS timestamp is zero", ErrLegacyImportSource)
	}

	snapshot := NewSnapshot()
	snapshot.Metadata = Metadata{
		SchemaVersion:        SchemaVersion,
		Authority:            AuthorityBolt,
		BootID:               bootID,
		OrchestratorType:     source.OrchestratorType,
		NodeID:               source.NodeID,
		Location:             source.Location,
		NetworkType:          source.NetworkType,
		Initialized:          source.Initialized,
		TimeStamp:            source.TimeStamp,
		LegacyImportComplete: true,
	}

	seenUUIDs := map[string]string{}
	for _, ncID := range sortedKeys(source.ContainerStatus) {
		status := source.ContainerStatus[ncID]
		if ncID == "" || status.ID == "" || status.ID != ncID {
			return Snapshot{}, fmt.Errorf(
				"%w: network container key %q does not match ID %q",
				ErrLegacyImportSource,
				ncID,
				status.ID,
			)
		}
		if status.CreateNetworkContainerRequest.NetworkContainerid != ncID {
			return Snapshot{}, fmt.Errorf(
				"%w: network container %q request ID does not match",
				ErrLegacyImportSource,
				ncID,
			)
		}
		if err := validateLegacyNetworkContainerRequest(status.CreateNetworkContainerRequest); err != nil {
			return Snapshot{}, fmt.Errorf("%w: network container %q: %w", ErrLegacyImportSource, ncID, err)
		}
		for _, ipID := range sortedKeys(status.CreateNetworkContainerRequest.SecondaryIPConfigs) {
			config := status.CreateNetworkContainerRequest.SecondaryIPConfigs[ipID]
			parsedID, err := uuid.Parse(ipID)
			if err != nil {
				return Snapshot{}, fmt.Errorf("%w: network container %q has invalid IP UUID", ErrLegacyImportSource, ncID)
			}
			canonicalID := parsedID.String()
			if other, ok := seenUUIDs[canonicalID]; ok {
				return Snapshot{}, fmt.Errorf(
					"%w: network containers %q and %q contain duplicate IP UUID",
					ErrLegacyImportSource,
					other,
					ncID,
				)
			}
			if _, err := netip.ParseAddr(config.IPAddress); err != nil {
				return Snapshot{}, fmt.Errorf(
					"%w: network container %q IP %q has invalid address",
					ErrLegacyImportSource,
					ncID,
					ipID,
				)
			}
			seenUUIDs[canonicalID] = ncID
			snapshot.IPs[ipID] = IPRecord{
				ID:        ipID,
				IPAddress: config.IPAddress,
				NCID:      ncID,
				NCVersion: config.NCVersion,
			}
		}
		snapshot.NetworkContainers[ncID] = NewNetworkContainerRecord(
			status.ID,
			status.VMVersion,
			status.HostVersion,
			status.VfpUpdateComplete,
			status.CreateNetworkContainerRequest,
		)
	}

	for _, contextID := range sortedKeys(source.ContainerIDByOrchestratorContext) {
		value := source.ContainerIDByOrchestratorContext[contextID]
		if contextID == "" || value == nil || *value == "" {
			return Snapshot{}, fmt.Errorf("%w: malformed orchestrator NC list %q", ErrLegacyImportSource, contextID)
		}
		ids := strings.Split(*value, ",")
		seen := map[string]struct{}{}
		for _, id := range ids {
			if id == "" || strings.TrimSpace(id) != id {
				return Snapshot{}, fmt.Errorf("%w: malformed orchestrator NC list %q", ErrLegacyImportSource, contextID)
			}
			if _, ok := seen[id]; ok {
				return Snapshot{}, fmt.Errorf("%w: orchestrator NC list %q contains a duplicate", ErrLegacyImportSource, contextID)
			}
			seen[id] = struct{}{}
		}
		snapshot.OrchestratorContexts[contextID] = ids
	}
	for _, name := range sortedKeys(source.Networks) {
		record := source.Networks[name]
		if record == nil {
			return Snapshot{}, fmt.Errorf("%w: network %q is null", ErrLegacyImportSource, name)
		}
		var nicInfo *wireserver.InterfaceInfo
		if record.NicInfo != nil {
			nicInfo = &wireserver.InterfaceInfo{
				Subnet:       record.NicInfo.Subnet,
				Gateway:      record.NicInfo.Gateway,
				PrimaryIP:    record.NicInfo.PrimaryIP,
				SecondaryIPs: append([]string(nil), record.NicInfo.SecondaryIPs...),
				IsPrimary:    record.NicInfo.IsPrimary,
			}
		}
		snapshot.Networks[name] = NetworkRecord{
			NetworkName: record.NetworkName,
			NicInfo:     nicInfo,
			Options:     record.Options,
		}
	}
	for _, rawMAC := range sortedKeys(source.PnpIDByMacAddress) {
		mac, err := canonicalMAC(rawMAC)
		if err != nil {
			return Snapshot{}, fmt.Errorf("%w: PnP mapping: %w", ErrLegacyImportSource, err)
		}
		if _, ok := snapshot.PnPIDByMAC[mac]; ok {
			return Snapshot{}, fmt.Errorf("%w: duplicate canonical PnP MAC", ErrLegacyImportSource)
		}
		snapshot.PnPIDByMAC[mac] = source.PnpIDByMacAddress[rawMAC]
	}
	return snapshot, nil
}

func addLegacyEndpoints(snapshot *Snapshot, data []byte) error {
	var envelope map[string]json.RawMessage
	if err := decodeJSONDocument(data, &envelope); err != nil {
		return fmt.Errorf("%w: decoding legacy endpoint state: %w", ErrLegacyImportSource, err)
	}
	endpointsRaw, ok := envelope["Endpoints"]
	if !ok {
		return fmt.Errorf("%w: legacy endpoint envelope is missing", ErrLegacyImportSource)
	}
	var endpoints map[string]*EndpointRecord
	if err := decodeStrictJSON(endpointsRaw, &endpoints); err != nil {
		return fmt.Errorf("%w: decoding legacy endpoints: %w", ErrLegacyImportSource, err)
	}
	if endpoints == nil {
		return fmt.Errorf("%w: legacy endpoints are null", ErrLegacyImportSource)
	}
	for _, containerID := range sortedKeys(endpoints) {
		record := endpoints[containerID]
		if strings.TrimSpace(containerID) != containerID {
			return fmt.Errorf("%w: legacy endpoint container ID is not canonical", ErrLegacyImportSource)
		}
		if record == nil {
			return fmt.Errorf("%w: legacy endpoint %q is null", ErrLegacyImportSource, containerID)
		}
		normalized, err := normalizeEndpoint(containerID, *record)
		if err != nil {
			return fmt.Errorf("%w: legacy endpoint %q: %w", ErrLegacyImportSource, containerID, err)
		}
		snapshot.Endpoints[containerID] = normalized
	}
	if intentsRaw, ok := envelope["DeleteIntents"]; ok {
		var intents map[string]DeleteIntent
		if err := decodeStrictJSON(intentsRaw, &intents); err != nil {
			return fmt.Errorf("%w: decoding legacy delete intents: %w", ErrLegacyImportSource, err)
		}
		if intents == nil {
			return fmt.Errorf("%w: legacy delete intents are null", ErrLegacyImportSource)
		}
		for _, containerID := range sortedKeys(intents) {
			intent, err := normalizeDeleteIntent(intents[containerID])
			if err != nil {
				return fmt.Errorf("%w: legacy delete intent %q: %w", ErrLegacyImportSource, containerID, err)
			}
			snapshot.DeleteIntents[containerID] = intent
		}
	}
	assignments, owners, err := reconstructRetainedEndpointOwnership(*snapshot)
	if err != nil {
		return fmt.Errorf("%w: reconstructing endpoint ownership: %w", ErrLegacyImportSource, err)
	}
	snapshot.Assignments = assignments
	snapshot.IPOwners = owners
	return nil
}

func validateLegacyNetworkContainerRequest(request cns.CreateNetworkContainerRequest) error {
	for _, item := range []struct {
		name    string
		address string
	}{
		{name: "host primary IP", address: request.HostPrimaryIP},
		{name: "primary interface identifier", address: request.PrimaryInterfaceIdentifier},
	} {
		if err := validateOptionalIPValue(item.address); err != nil {
			return fmt.Errorf("%s: %w", item.name, err)
		}
	}
	for _, item := range []struct {
		name   string
		config cns.IPConfiguration
	}{
		{name: "local IP configuration", config: request.LocalIPConfiguration},
		{name: "IP configuration", config: request.IPConfiguration},
		{name: "IPv6 configuration", config: request.IPv6Configuration},
	} {
		if err := validateLegacyIPConfiguration(item.config); err != nil {
			return fmt.Errorf("%s: %w", item.name, err)
		}
	}
	for index, subnet := range request.CnetAddressSpace {
		if err := validateLegacySubnet(subnet, 0); err != nil {
			return fmt.Errorf("CNet address space %d: %w", index, err)
		}
	}
	for index, route := range request.Routes {
		if route.IPAddress != "" {
			if _, err := netip.ParsePrefix(route.IPAddress); err != nil {
				if _, addrErr := netip.ParseAddr(route.IPAddress); addrErr != nil {
					return fmt.Errorf("route %d has invalid destination", index)
				}
			}
		}
		if err := validateOptionalAddress(route.GatewayIPAddress); err != nil {
			return fmt.Errorf("route %d gateway: %w", index, err)
		}
	}
	if request.NetworkInterfaceInfo.MACAddress != "" {
		if _, err := net.ParseMAC(request.NetworkInterfaceInfo.MACAddress); err != nil {
			return fmt.Errorf("invalid network interface MAC")
		}
	}
	return nil
}

func validateLegacyIPConfiguration(config cns.IPConfiguration) error {
	if err := validateLegacySubnet(config.IPSubnet, 0); err != nil {
		return err
	}
	if err := validateLegacySubnet(config.IPSubnetV6, 128); err != nil {
		return err
	}
	for _, address := range config.DNSServers {
		if err := validateOptionalAddress(address); err != nil {
			return err
		}
	}
	for _, gateway := range []string{config.GatewayIPAddress, config.GatewayIPv6Address} {
		if err := validateOptionalIPValue(gateway); err != nil {
			return err
		}
	}
	return nil
}

func validateLegacySubnet(subnet cns.IPSubnet, expectedBits int) error {
	if subnet.IPAddress == "" {
		if subnet.PrefixLength != 0 {
			return errors.New("prefix length is set without an address")
		}
		return nil
	}
	address, err := netip.ParseAddr(subnet.IPAddress)
	if err != nil {
		return errors.New("invalid subnet address")
	}
	address = address.Unmap()
	bits := 128
	if address.Is4() {
		bits = 32
	}
	if expectedBits != 0 && bits != expectedBits {
		return errors.New("subnet address family does not match")
	}
	if int(subnet.PrefixLength) > bits {
		return errors.New("invalid subnet prefix length")
	}
	return nil
}

func validateOptionalAddress(value string) error {
	if value == "" {
		return nil
	}
	if _, err := netip.ParseAddr(value); err != nil {
		return errors.New("invalid IP address")
	}
	return nil
}

func validateOptionalIPValue(value string) error {
	if value == "" {
		return nil
	}
	if _, err := netip.ParseAddr(value); err == nil {
		return nil
	}
	if _, err := netip.ParsePrefix(value); err != nil {
		return errors.New("invalid IP address or prefix")
	}
	return nil
}

type importedSnapshotEncoding struct {
	buckets         []encodedBucket
	serviceMetadata []byte
}

func encodeImportedSnapshot(snapshot Snapshot) (importedSnapshotEncoding, error) {
	values := []struct {
		name   []byte
		encode func() (map[string][]byte, error)
	}{
		{name: bucketNetworkContainers, encode: func() (map[string][]byte, error) {
			return encodeJSONMap(snapshot.NetworkContainers)
		}},
		{name: bucketIPs, encode: func() (map[string][]byte, error) { return encodeJSONMap(snapshot.IPs) }},
		{name: bucketNetworks, encode: func() (map[string][]byte, error) { return encodeJSONMap(snapshot.Networks) }},
		{name: bucketOrchestratorContexts, encode: func() (map[string][]byte, error) {
			return encodeJSONMap(snapshot.OrchestratorContexts)
		}},
		{name: bucketPnPIDByMAC, encode: func() (map[string][]byte, error) {
			return encodeJSONMap(snapshot.PnPIDByMAC)
		}},
		{name: bucketAssignments, encode: func() (map[string][]byte, error) {
			return encodeJSONMap(snapshot.Assignments)
		}},
		{name: bucketIPOwners, encode: func() (map[string][]byte, error) { return encodeJSONMap(snapshot.IPOwners) }},
		{name: bucketEndpoints, encode: func() (map[string][]byte, error) { return encodeJSONMap(snapshot.Endpoints) }},
		{name: bucketDeleteIntents, encode: func() (map[string][]byte, error) {
			return encodeJSONMap(snapshot.DeleteIntents)
		}},
	}
	encoded := importedSnapshotEncoding{buckets: make([]encodedBucket, 0, len(values))}
	for _, value := range values {
		data, err := value.encode()
		if err != nil {
			return importedSnapshotEncoding{}, err
		}
		encoded.buckets = append(encoded.buckets, encodedBucket{name: value.name, values: data})
	}
	var err error
	encoded.serviceMetadata, err = json.Marshal(serviceMetadata{
		OrchestratorType: snapshot.Metadata.OrchestratorType,
		NodeID:           snapshot.Metadata.NodeID,
		Location:         snapshot.Metadata.Location,
		NetworkType:      snapshot.Metadata.NetworkType,
		Initialized:      snapshot.Metadata.Initialized,
		TimeStamp:        snapshot.Metadata.TimeStamp,
	})
	if err != nil {
		return importedSnapshotEncoding{}, invalidInput("encoding imported service metadata", err)
	}
	return encoded, nil
}

func requireEmptyImportTarget(tx *WriteTx) error {
	snapshot, err := snapshotFromTx(tx.ctx, &tx.ReadTx)
	if err != nil {
		return err
	}
	if snapshot.Metadata.Generation != 0 ||
		snapshot.Metadata.Authority != AuthorityBolt ||
		snapshot.Metadata.BootID != "" ||
		snapshot.Metadata.OrchestratorType != "" ||
		snapshot.Metadata.NodeID != "" ||
		snapshot.Metadata.Location != "" ||
		snapshot.Metadata.NetworkType != "" ||
		snapshot.Metadata.Initialized ||
		!snapshot.Metadata.TimeStamp.IsZero() ||
		len(snapshot.NetworkContainers) != 0 ||
		len(snapshot.IPs) != 0 ||
		len(snapshot.Networks) != 0 ||
		len(snapshot.OrchestratorContexts) != 0 ||
		len(snapshot.PnPIDByMAC) != 0 ||
		len(snapshot.Assignments) != 0 ||
		len(snapshot.IPOwners) != 0 ||
		len(snapshot.Endpoints) != 0 ||
		len(snapshot.DeleteIntents) != 0 {
		return ErrLegacyImportTargetNotEmpty
	}
	return nil
}

func (s *DB) importComplete(ctx context.Context) (bool, error) {
	var complete bool
	err := s.View(ctx, func(tx *ReadTx) error {
		var err error
		complete, err = legacyImportComplete(tx.tx.Bucket(bucketMetadata))
		return err
	})
	if err != nil {
		return false, fmt.Errorf("checking legacy import marker: %w", err)
	}
	return complete, nil
}

func decodeLegacyEnvelope(data []byte, key string, destination any) error {
	var envelope map[string]json.RawMessage
	if err := decodeJSONDocument(data, &envelope); err != nil {
		return err
	}
	raw, ok := envelope[key]
	if !ok {
		return fmt.Errorf("missing %q envelope", key)
	}
	return decodeStrictJSON(raw, destination)
}

func decodeJSONDocument(data []byte, destination any) error {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return err
	}
	return decodeStrictJSON(data, destination)
}

func decodeStrictJSON(data []byte, destination any) error {
	if len(bytes.TrimSpace(data)) == 0 {
		return errors.New("json value is empty")
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return errors.New("json value is null")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple json values")
		}
		return err
	}
	return nil
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := scanJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple json values")
		}
		return err
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		keys := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			canonical := strings.ToLower(key)
			if _, ok := keys[canonical]; ok {
				return fmt.Errorf("duplicate json key %q", key)
			}
			keys[canonical] = struct{}{}
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	default:
		return errors.New("unexpected JSON delimiter")
	}
}
