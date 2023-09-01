package api

import (
	"encoding/json"
	"net"
	"os"

	"github.com/Azure/azure-container-networking/cni/log"
	"go.uber.org/zap"
)

var (
	loggerName = "azure-vnet"
	logger     = log.InitZapLogCNI(loggerName, "azure-vnet.log")
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
		logger.Error("Failed to unmarshall Azure CNI state", zap.Error(err))
	}

	// write result to stdout to be captured by caller
	_, err = os.Stdout.Write(b)
	if err != nil {
		logger.Error("Failed to write response to stdout", zap.Error(err))
		return err
	}

	return nil
}
