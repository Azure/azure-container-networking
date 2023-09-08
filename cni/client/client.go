package client

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cni/api"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/platform"
	semver "github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	utilexec "k8s.io/utils/exec"
)

type client struct {
	exec utilexec.Interface
}

var ErrSemVerParse = errors.New("error parsing version")

func New(exec utilexec.Interface) *client {
	return &client{exec: exec}
}

func (c *client) GetEndpointState() (*api.AzureCNIState, error) {
	cmd := c.exec.Command(platform.CNIBinaryPath)
	log.Printf("first cmd is %+v", cmd)
	cmd.SetDir(CNIExecDir)
	log.Printf("second cmd is %+v", cmd)
	envs := os.Environ()
	cmdenv := fmt.Sprintf("%s=%s", cni.Cmd, cni.CmdGetEndpointsState)
	log.Printf("Setting cmd to %s", cmdenv)
	envs = append(envs, cmdenv)
	log.Printf("envs is %+v", envs)
	cmd.SetEnv(envs)
	log.Printf("third cmd is %+v", cmd)

	output, err := cmd.CombinedOutput()
	log.Printf("CombinedOutput output is %s", string(output))
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
	cmd := c.exec.Command(platform.CNIBinaryPath, "-v")
	cmd.SetDir(CNIExecDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get Azure CNI version with err: [%w], output: [%s]", err, string(output))
	}

	res := strings.Fields(string(output))

	if len(res) != 4 {
		return nil, fmt.Errorf("Unexpected Azure CNI Version formatting: %v", output)
	}

	version, versionErr := semver.NewVersion(res[3])
	if versionErr != nil {
		return nil, errors.Wrap(ErrSemVerParse, versionErr.Error())
	}

	return version, nil
}
