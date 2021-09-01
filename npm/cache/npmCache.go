package cache

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/Azure/azure-container-networking/npm"
	"github.com/Azure/azure-container-networking/npm/ipsm"
)

type NPMCache struct {
	Nodename string
	NsMap    map[string]*npm.Namespace
	PodMap   map[string]*npm.NpmPod
	ListMap  map[string]*ipsm.Ipset
	SetMap   map[string]*ipsm.Ipset
}

// Decode returns NPMCache object after decoding data.
// TODO(jungukcho): This Decode has tight ordering for decoding data due to dependency with Encode function.
// It needs to find a way of relaxing this strong ordering.
func Decode(reader io.Reader) (*NPMCache, error) {
	cache := &NPMCache{}
	dec := json.NewDecoder(reader)

	if err := dec.Decode(&cache.Nodename); err != nil {
		return nil, fmt.Errorf("failed to decode Nodename : %w", err)
	}

	if err := dec.Decode(&cache.NsMap); err != nil {
		return nil, fmt.Errorf("failed to decode NsMap : %w", err)
	}

	if err := dec.Decode(&cache.PodMap); err != nil {
		return nil, fmt.Errorf("failed to decode PodMap : %w", err)
	}

	if err := dec.Decode(&cache.ListMap); err != nil {
		return nil, fmt.Errorf("failed to decode ListMap : %w", err)
	}

	if err := dec.Decode(&cache.SetMap); err != nil {
		return nil, fmt.Errorf("failed to decode SetMap : %w", err)
	}

	return cache, nil
}

// Encode returns encoded NPMCache data.
// TODO(jungukcho): This Encode has tight ordering for encoding data due to dependency with Decode function.
// It needs to find a way of relaxing this strong ordering.
func Encode(writer io.Writer, npmEncoder npm.NetworkPolicyManagerEncoder) error {
	if err := npmEncoder.Encode(writer); err != nil {
		return fmt.Errorf("cannot encode NPMCache %w", err)
	}

	return nil
}
