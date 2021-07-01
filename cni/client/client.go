package client

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cni/api"
	"github.com/Azure/azure-container-networking/log"
	semver "github.com/hashicorp/go-version"
	utilexec "k8s.io/utils/exec"
)

const (
	azureVnetExecutable = "/opt/cni/bin/azure-vnet"
)

type Client interface {
	GetEndpointState() (*api.AzureCNIState, error)
}

var _ (Client) = (*client)(nil)

type client struct {
	exec utilexec.Interface
}

func New(exec utilexec.Interface) *client {
	return &client{exec: exec}
}

func (c *client) GetEndpointState() (*api.AzureCNIState, error) {
	cmd := c.exec.Command(azureVnetExecutable)

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

func (c *client) GetVersion() (*semver.Version, error) {
	cmd := c.exec.Command(azureVnetExecutable, "-v")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get Azure CNI version with err: [%w], output: [%s]", err, string(output))
	}

	res := strings.Fields(string(output))

	if len(res) != 4 {
		return nil, fmt.Errorf("Unexpected Azure CNI Version formatting: %v", output)
	}

	return semver.NewVersion(res[3])
}
