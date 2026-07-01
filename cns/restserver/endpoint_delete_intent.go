package restserver

import (
	"fmt"
	"net"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/store"
	"github.com/pkg/errors"
)

func (service *HTTPRestService) loadEndpointDeleteIntents() {
	if service.EndpointStateStore == nil {
		return
	}

	var intents map[string]EndpointDeleteIntent
	err := service.EndpointStateStore.Read(EndpointDeleteIntentStoreKey, &intents)
	if err != nil {
		if errors.Is(err, store.ErrKeyNotFound) || errors.Is(err, store.ErrStoreEmpty) {
			service.EndpointDeleteIntents = make(map[string]EndpointDeleteIntent)
			return
		}
		logger.Errorf("[Azure CNS] Failed to restore endpoint delete intents, err:%v", err)
		service.EndpointDeleteIntents = make(map[string]EndpointDeleteIntent)
		return
	}
	service.EndpointDeleteIntents = intents
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

func (service *HTTPRestService) recordEndpointDeleteIntentLocked(req cns.IPConfigsRequest, podInfo cns.PodInfo, now time.Time) error {
	intents := cloneEndpointDeleteIntents(service.EndpointDeleteIntents)
	intents[podInfo.InfraContainerID()] = EndpointDeleteIntent{
		InfraContainerID: podInfo.InfraContainerID(),
		PodInterfaceID:   podInfo.InterfaceID(),
		PodName:          podInfo.Name(),
		PodNamespace:     podInfo.Namespace(),
		Ifname:           req.Ifname,
		CreatedAt:        now,
	}
	if err := service.writeEndpointDeleteIntentsLocked(intents); err != nil {
		return err
	}
	logger.Printf("[endpointDeleteIntent] recorded delete intent for infra container %s pod %s/%s",
		podInfo.InfraContainerID(), podInfo.Namespace(), podInfo.Name())
	return nil
}

func (service *HTTPRestService) hasEndpointDeleteIntentLocked(containerID string) bool {
	if service.EndpointDeleteIntents == nil {
		return false
	}
	_, ok := service.EndpointDeleteIntents[containerID]
	return ok
}

func (service *HTTPRestService) pruneEndpointDeleteIntentsLocked(now time.Time) error {
	if len(service.EndpointDeleteIntents) == 0 {
		return nil
	}

	intents := cloneEndpointDeleteIntents(service.EndpointDeleteIntents)
	pruned := 0
	for containerID, intent := range intents {
		if !intent.CreatedAt.IsZero() && now.Sub(intent.CreatedAt) > endpointDeleteIntentTTL {
			delete(intents, containerID)
			pruned++
		}
	}
	if pruned == 0 {
		return nil
	}
	if err := service.writeEndpointDeleteIntentsLocked(intents); err != nil {
		return err
	}
	logger.Printf("[endpointDeleteIntent] pruned %d expired endpoint delete intents", pruned)
	return nil
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
