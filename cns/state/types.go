// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"net"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/wireserver"
)

const SchemaVersion uint32 = 1

type Authority string

const (
	AuthorityBolt Authority = "bolt"
	AuthorityJSON Authority = "json"
)

type Metadata struct {
	SchemaVersion    uint32    `json:"schemaVersion"`
	Authority        Authority `json:"authority"`
	Generation       uint64    `json:"generation"`
	BootID           string    `json:"bootID,omitempty"`
	OrchestratorType string    `json:"orchestratorType,omitempty"`
	NodeID           string    `json:"nodeID,omitempty"`
	Location         string    `json:"location,omitempty"`
	NetworkType      string    `json:"networkType,omitempty"`
	Initialized      bool      `json:"initialized"`
	TimeStamp        time.Time `json:"timestamp,omitempty"`
}

type NetworkContainerRecord struct {
	ID                string                            `json:"id"`
	VMVersion         string                            `json:"vmVersion,omitempty"`
	HostVersion       string                            `json:"hostVersion,omitempty"`
	VFPUpdateComplete bool                              `json:"vfpUpdateComplete"`
	Request           cns.CreateNetworkContainerRequest `json:"request"`
}

type IPRecord struct {
	ID        string `json:"id"`
	IPAddress string `json:"ipAddress"`
	NCID      string `json:"ncID"`
	NCVersion int    `json:"ncVersion"`
}

type NetworkRecord struct {
	NetworkName string                    `json:"networkName"`
	NicInfo     *wireserver.InterfaceInfo `json:"nicInfo,omitempty"`
	Options     map[string]any            `json:"options,omitempty"`
}

type EndpointRecord struct {
	PodName       string                   `json:"podName,omitempty"`
	PodNamespace  string                   `json:"podNamespace,omitempty"`
	IfnameToIPMap map[string]*IPInfoRecord `json:"ifnameToIPMap,omitempty"`
}

type IPInfoRecord struct {
	IPv4               []net.IPNet `json:"ipv4,omitempty"`
	IPv6               []net.IPNet `json:"ipv6,omitempty"`
	HNSEndpointID      string      `json:"hnsEndpointID,omitempty"`
	HNSNetworkID       string      `json:"hnsNetworkID,omitempty"`
	HostVethName       string      `json:"hostVethName,omitempty"`
	MACAddress         string      `json:"macAddress,omitempty"`
	NetworkContainerID string      `json:"networkContainerID,omitempty"`
	NICType            cns.NICType `json:"nicType,omitempty"`
}

type PodIdentity struct {
	PodKey           string `json:"podKey"`
	InfraContainerID string `json:"infraContainerID"`
	InterfaceID      string `json:"interfaceID,omitempty"`
	PodName          string `json:"podName,omitempty"`
	PodNamespace     string `json:"podNamespace,omitempty"`
}

type AssignmentRecord struct {
	Pod   PodIdentity `json:"pod"`
	IPIDs []string    `json:"ipIDs"`
}

type DeleteIntent struct {
	CreatedAt time.Time `json:"createdAt"`
}
