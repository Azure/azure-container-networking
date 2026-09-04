package restserver

import (
	"fmt"
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
		return fmt.Errorf("writing endpoint delete intents: %w", err)
	}
	service.EndpointDeleteIntents = intents
	return nil
}

func (service *HTTPRestService) recordEndpointDeleteIntentLocked(containerID string, now time.Time) error {
	nextIntents := cloneEndpointDeleteIntents(service.EndpointDeleteIntents)
	for existingContainerID, intent := range nextIntents {
		if endpointDeleteIntentExpired(intent, now) {
			delete(nextIntents, existingContainerID)
		}
	}
	// Keep the timestamp of the first delete. A rejected ADD makes the runtime
	// retry CNI DEL, so refreshing the timestamp here would let those retries
	// renew the intent indefinitely and permanently block the container.
	if _, ok := nextIntents[containerID]; !ok {
		nextIntents[containerID] = EndpointDeleteIntent{CreatedAt: now}
	}
	return service.writeEndpointDeleteIntentsLocked(nextIntents)
}

func (service *HTTPRestService) endpointDeleteIntentBlocksAddLocked(containerID string, now time.Time) (bool, error) {
	intent, ok := service.EndpointDeleteIntents[containerID]
	if !ok {
		return false, nil
	}

	if !endpointDeleteIntentExpired(intent, now) {
		return true, nil
	}

	nextIntents := cloneEndpointDeleteIntents(service.EndpointDeleteIntents)
	delete(nextIntents, containerID)
	if err := service.writeEndpointDeleteIntentsLocked(nextIntents); err != nil {
		return true, err
	}
	return false, nil
}

func (service *HTTPRestService) replayEndpointDeleteIntentsLocked(now time.Time) error {
	if len(service.EndpointDeleteIntents) == 0 {
		return nil
	}

	nextIntents := cloneEndpointDeleteIntents(service.EndpointDeleteIntents)
	nextEndpointState := cloneEndpointState(service.EndpointState)
	intentsChanged := false
	endpointStateChanged := false
	for containerID, intent := range nextIntents {
		if endpointDeleteIntentExpired(intent, now) {
			delete(nextIntents, containerID)
			intentsChanged = true
			continue
		}
		if _, ok := nextEndpointState[containerID]; ok {
			delete(nextEndpointState, containerID)
			endpointStateChanged = true
		}
	}

	if endpointStateChanged {
		if err := service.EndpointStateStore.Write(EndpointStoreKey, nextEndpointState); err != nil {
			return fmt.Errorf("replaying endpoint delete intents: %w", err)
		}
		service.EndpointState = nextEndpointState
	}
	if intentsChanged {
		if err := service.writeEndpointDeleteIntentsLocked(nextIntents); err != nil {
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
