package client

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cni/api"
	"github.com/Azure/azure-container-networking/log"
	utilexec "k8s.io/utils/exec"
)

const (
	azureVnetBinName      = "./azure-vnet"
	azureVnetBinDirectory = "/opt/cni/bin"
)

type CNIClient interface {
	GetEndpointState() (api.CNIState, error)
}

type AzureCNIClient struct {
	exec utilexec.Interface
}

func NewCNIClient(exec utilexec.Interface) *AzureCNIClient {
	return &AzureCNIClient{
		exec: exec,
	}
}

func (c *AzureCNIClient) GetEndpointState() (api.CNIState, error) {
	cmd := c.exec.Command(azureVnetBinName)
	cmd.SetDir(azureVnetBinDirectory)

	envs := os.Environ()
	cmdenv := fmt.Sprintf("%s=%s", cni.Cmd, cni.CmdGetEndpointsState)
	log.Printf("Setting cmd to %s", cmdenv)
	envs = append(envs, cmdenv)
	cmd.SetEnv(envs)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to call Azure CNI bin with err: [%w], output: [%s]", err, string(output))
	}

	state := &api.AzureCNIState{}
	if err := json.Unmarshal(output, state); err != nil {
		return nil, fmt.Errorf("failed to decode response from Azure CNI when retrieving state: [%w], response from CNI: [%s]", err, string(output))
	}

	return state, nil
}
