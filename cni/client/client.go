package client

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cni/api"
	utilexec "k8s.io/utils/exec"
)

type CNIClient struct {
	exec utilexec.Interface
}

func NewCNIClient(exec utilexec.Interface) *CNIClient {
	return &CNIClient{
		exec: exec,
	}
}

func (c *CNIClient) GetState() (*api.AzureCNIState, error) {
	cmd := c.exec.Command("./azure-vnet")
	cmd.SetDir("/opt/cni/bin")

	envs := os.Environ()
	envs = append(envs, fmt.Sprintf("%s=%s", cni.Cmd, cni.CmdState))

	cmd.SetEnv(envs)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	state := &api.AzureCNIState{}
	if err := json.Unmarshal(output, state); err != nil {
		return nil, fmt.Errorf("Failed to decode response from CNI when retrieving state: [%w], response from CNI: [%s]", err, string(output))
	}

	return state, nil
}
