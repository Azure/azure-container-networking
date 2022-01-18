package controlplane

import (
	"bytes"
	"encoding/gob"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"
)

func EncodeString(name string) (*bytes.Buffer, error) {
	if name == "" {
		return nil, npmerrors.SimpleError("failed to encode, name is empty")
	}
	var payloadBuffer bytes.Buffer
	err := gob.NewEncoder(&payloadBuffer).Encode(&name)
	if err != nil {
		return nil, npmerrors.SimpleErrorWrapper("failed to encode", err)
	}
	return &payloadBuffer, nil
}

func DecodeString(payload *bytes.Buffer) (string, error) {
	if payload == nil {
		return "", npmerrors.SimpleError("failed to decode, payload is nil")
	}
	var name string
	err := gob.NewDecoder(payload).Decode(&name)
	if err != nil {
		return "", npmerrors.SimpleErrorWrapper("failed to decode", err)
	}
	return name, nil
}

func EncodeControllerIPSet(ipset *ControllerIPSets) (*bytes.Buffer, error) {
	if ipset == nil {
		return nil, npmerrors.SimpleError("failed to encode, ipset is nil")
	}
	var payloadBuffer bytes.Buffer
	err := gob.NewEncoder(&payloadBuffer).Encode(&ipset)
	if err != nil {
		return nil, npmerrors.SimpleErrorWrapper("failed to encode", err)
	}
	return &payloadBuffer, nil
}

func DecodeControllerIPSet(payload *bytes.Buffer) (*ControllerIPSets, error) {
	if payload == nil {
		return nil, npmerrors.SimpleError("failed to decode, payload is nil")
	}
	var ipset ControllerIPSets
	err := gob.NewDecoder(payload).Decode(&ipset)
	if err != nil {
		return nil, npmerrors.SimpleErrorWrapper("failed to decode", err)
	}
	return &ipset, nil
}

func EncodeNPMNetworkPolicy(netpol *policies.NPMNetworkPolicy) (*bytes.Buffer, error) {
	if netpol == nil {
		return nil, npmerrors.SimpleError("failed to encode, netpol is nil")
	}
	var payloadBuffer bytes.Buffer
	enc := gob.NewEncoder(&payloadBuffer)
	err := enc.Encode(netpol)
	if err != nil {
		return nil, npmerrors.SimpleErrorWrapper("failed to encode", err)
	}
	return &payloadBuffer, nil
}

func DecodeNPMNetworkPolicy(payload *bytes.Buffer) (*policies.NPMNetworkPolicy, error) {
	if payload == nil {
		return nil, npmerrors.SimpleError("failed to decode, payload is nil")
	}
	var netpol policies.NPMNetworkPolicy
	err := gob.NewDecoder(payload).Decode(&netpol)
	if err != nil {
		return nil, npmerrors.SimpleErrorWrapper("failed to decode", err)
	}
	return &netpol, nil
}
