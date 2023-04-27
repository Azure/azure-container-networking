package api

import (
	"encoding/json"
	"github.com/Azure/azure-container-networking/cni/log"
	"go.uber.org/zap"
	"net"
	"os"
)

type PodNetworkInterfaceInfo struct {
	PodName       string
	PodNamespace  string
	PodEndpointId string
	ContainerID   string
	IPAddresses   []net.IPNet
}

type AzureCNIState struct {
	ContainerInterfaces map[string]PodNetworkInterfaceInfo
}

func (a *AzureCNIState) PrintResult() error {
	b, err := json.MarshalIndent(a, "", "    ")
	if err != nil {
		log.Logger.Error("Failed to unmarshall Azure CNI state", zap.Any("error", err))
	}

	// write result to stdout to be captured by caller
	_, err = os.Stdout.Write(b)
	if err != nil {
		log.Logger.Error("Failed to write response to stdout", zap.Any("error", err))
		return err
	}

	return nil
}
