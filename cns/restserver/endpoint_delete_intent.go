package restserver

import (
	"fmt"
	"net"
	"time"

	"github.com/Azure/azure-container-networking/store"
	"github.com/pkg/errors"
)

func (service *HTTPRestService) loadEndpointDeleteIntentsLocked() error {
	if service.EndpointStateStore == nil {
		return ErrStoreEmpty
	}

	var intents map[string]EndpointDeleteIntent
	err := service.EndpointStateStore.Read(EndpointDeleteIntentStoreKey, &intents)
	if err != nil {
		if errors.Is(err, store.ErrKeyNotFound) || errors.Is(err, store.ErrStoreEmpty) {
			service.EndpointDeleteIntents = make(map[string]EndpointDeleteIntent)
			return nil
		}
		return fmt.Errorf("reading endpoint delete intents: %w", err)
	}
	if intents == nil {
		intents = make(map[string]EndpointDeleteIntent)
	}
	service.EndpointDeleteIntents = intents
	return nil
}

func (service *HTTPRestService) writeEndpointDeleteIntentsLocked(intents map[string]EndpointDeleteIntent) error {
	if service.EndpointStateStore == nil {
		return ErrStoreEmpty
	}
	if err := service.EndpointStateStore.Write(EndpointDeleteIntentStoreKey, intents); err != nil {
		return fmt.Errorf("failed to write endpoint delete intents to store: %w", err)
	}
	service.EndpointDeleteIntents = intents
	return nil
}

func (service *HTTPRestService) recordEndpointDeleteIntentLocked(containerID string, now time.Time) error {
	intents := cloneEndpointDeleteIntents(service.EndpointDeleteIntents)
	for existingContainerID, intent := range intents {
		if endpointDeleteIntentExpired(intent, now) {
			delete(intents, existingContainerID)
		}
	}
	intents[containerID] = EndpointDeleteIntent{CreatedAt: now}
	return service.writeEndpointDeleteIntentsLocked(intents)
}

func (service *HTTPRestService) endpointDeleteIntentBlocksAddLocked(containerID string, now time.Time) (bool, error) {
	intent, ok := service.EndpointDeleteIntents[containerID]
	if !ok {
		return false, nil
	}

	if !endpointDeleteIntentExpired(intent, now) {
		return true, nil
	}

	intents := cloneEndpointDeleteIntents(service.EndpointDeleteIntents)
	delete(intents, containerID)
	if err := service.writeEndpointDeleteIntentsLocked(intents); err != nil {
		return true, err
	}
	return false, nil
}

func (service *HTTPRestService) replayEndpointDeleteIntentsLocked(now time.Time) error {
	if len(service.EndpointDeleteIntents) == 0 {
		return nil
	}

	intents := cloneEndpointDeleteIntents(service.EndpointDeleteIntents)
	endpointState := cloneEndpointState(service.EndpointState)
	intentsChanged := false
	endpointStateChanged := false
	for containerID, intent := range intents {
		if endpointDeleteIntentExpired(intent, now) {
			delete(intents, containerID)
			intentsChanged = true
			continue
		}
		if _, ok := endpointState[containerID]; ok {
			delete(endpointState, containerID)
			endpointStateChanged = true
		}
	}

	if endpointStateChanged {
		if err := service.EndpointStateStore.Write(EndpointStoreKey, endpointState); err != nil {
			return fmt.Errorf("replaying endpoint delete intents: %w", err)
		}
		service.EndpointState = endpointState
	}
	if intentsChanged {
		if err := service.writeEndpointDeleteIntentsLocked(intents); err != nil {
			return fmt.Errorf("pruning endpoint delete intents: %w", err)
		}
	}
	return nil
}

func endpointDeleteIntentExpired(intent EndpointDeleteIntent, now time.Time) bool {
	return intent.CreatedAt.IsZero() || !now.Before(intent.CreatedAt.Add(endpointDeleteIntentTTL))
}

func cloneEndpointDeleteIntents(intents map[string]EndpointDeleteIntent) map[string]EndpointDeleteIntent {
	cloned := make(map[string]EndpointDeleteIntent, len(intents))
	for containerID, intent := range intents {
		cloned[containerID] = intent
	}
	return cloned
}

func cloneEndpointState(state map[string]*EndpointInfo) map[string]*EndpointInfo {
	cloned := make(map[string]*EndpointInfo, len(state))
	for containerID, endpointInfo := range state {
		if endpointInfo == nil {
			cloned[containerID] = nil
			continue
		}
		info := &EndpointInfo{
			PodName:       endpointInfo.PodName,
			PodNamespace:  endpointInfo.PodNamespace,
			IfnameToIPMap: make(map[string]*IPInfo, len(endpointInfo.IfnameToIPMap)),
		}
		for ifName, ipInfo := range endpointInfo.IfnameToIPMap {
			if ipInfo == nil {
				info.IfnameToIPMap[ifName] = nil
				continue
			}
			info.IfnameToIPMap[ifName] = &IPInfo{
				IPv4:               append([]net.IPNet(nil), ipInfo.IPv4...),
				IPv6:               append([]net.IPNet(nil), ipInfo.IPv6...),
				HnsEndpointID:      ipInfo.HnsEndpointID,
				HnsNetworkID:       ipInfo.HnsNetworkID,
				HostVethName:       ipInfo.HostVethName,
				MacAddress:         ipInfo.MacAddress,
				NetworkContainerID: ipInfo.NetworkContainerID,
				NICType:            ipInfo.NICType,
			}
		}
		cloned[containerID] = info
	}
	return cloned
}
