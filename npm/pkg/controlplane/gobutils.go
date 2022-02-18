package controlplane

import (
	"bytes"
	"encoding/gob"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"
)

func EncodeStrings(names []string) (*bytes.Buffer, error) {
	if len(names) == 0 {
		return nil, npmerrors.SimpleError("failed to encode, name is empty")
	}
	var payloadBuffer bytes.Buffer
	err := gob.NewEncoder(&payloadBuffer).Encode(&names)
	if err != nil {
		return nil, npmerrors.SimpleErrorWrapper("failed to encode", err)
	}
	return &payloadBuffer, nil
}

func DecodeStrings(payload *bytes.Buffer) ([]string, error) {
	if payload == nil {
		return nil, npmerrors.SimpleError("failed to decode, payload is nil")
	}
	var names []string
	err := gob.NewDecoder(payload).Decode(&names)
	if err != nil {
		return nil, npmerrors.SimpleErrorWrapper("failed to decode", err)
	}
	return names, nil
}

func EncodeControllerIPSets(ipsets []*ControllerIPSets) (*bytes.Buffer, error) {
	if len(ipsets) == 0 {
		return nil, npmerrors.SimpleError("failed to encode, ipset is nil")
	}
	var payloadBuffer bytes.Buffer
	err := gob.NewEncoder(&payloadBuffer).Encode(&ipsets)
	if err != nil {
		return nil, npmerrors.SimpleErrorWrapper("failed to encode", err)
	}
	return &payloadBuffer, nil
}

func DecodeControllerIPSets(payload *bytes.Buffer) ([]*ControllerIPSets, error) {
	if payload == nil {
		return nil, npmerrors.SimpleError("failed to decode, payload is nil")
	}
	var ipsets []*ControllerIPSets
	err := gob.NewDecoder(payload).Decode(&ipsets)
	if err != nil {
		return nil, npmerrors.SimpleErrorWrapper("failed to decode", err)
	}
	return ipsets, nil
}

func EncodeNPMNetworkPolicies(netpols []*policies.NPMNetworkPolicy) (*bytes.Buffer, error) {
	if len(netpols) == 0 {
		return nil, npmerrors.SimpleError("failed to encode, netpol is nil")
	}
	var payloadBuffer bytes.Buffer
	enc := gob.NewEncoder(&payloadBuffer)
	err := enc.Encode(netpols)
	if err != nil {
		return nil, npmerrors.SimpleErrorWrapper("failed to encode", err)
	}
	return &payloadBuffer, nil
}

func DecodeNPMNetworkPolicies(payload *bytes.Buffer) ([]*policies.NPMNetworkPolicy, error) {
	if payload == nil {
		return nil, npmerrors.SimpleError("failed to decode, payload is nil")
	}
	var netpols []*policies.NPMNetworkPolicy
	err := gob.NewDecoder(payload).Decode(&netpols)
	if err != nil {
		return nil, npmerrors.SimpleErrorWrapper("failed to decode", err)
	}
	return netpols, nil
}
