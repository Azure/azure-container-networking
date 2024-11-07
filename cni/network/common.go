package network

import (
	"encoding/json"
	"io"
	"os"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

func validateConfig(jsonBytes []byte) error {
	var conf struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(jsonBytes, &conf); err != nil {
		return errors.Wrapf(err, "error reading network config")
	}
	if conf.Name == "" {
		return errors.New("missing network name")
	}
	return nil
}

func getCmdArgsFromEnv() (string, *skel.CmdArgs, error) {
	logger.Info("Going to read from stdin")
	stdinData, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", nil, errors.Wrapf(err, "error reading from stdin")
	}

	cmdArgs := &skel.CmdArgs{
		ContainerID: os.Getenv("CNI_CONTAINERID"),
		Netns:       os.Getenv("CNI_NETNS"),
		IfName:      os.Getenv("CNI_IFNAME"),
		Args:        os.Getenv("CNI_ARGS"),
		Path:        os.Getenv("CNI_PATH"),
		StdinData:   stdinData,
	}

	cmd := os.Getenv("CNI_COMMAND")
	return cmd, cmdArgs, nil
}

func HandleIfCniUpdate(update func(*skel.CmdArgs) error) (bool, error) {
	isupdate := true

	if os.Getenv("CNI_COMMAND") != cni.CmdUpdate {
		return false, nil
	}

	logger.Info("CNI UPDATE received")

	_, cmdArgs, err := getCmdArgsFromEnv()
	if err != nil {
		logger.Error("Received error while retrieving cmds from environment", zap.Error(err))
		return isupdate, err
	}

	logger.Info("Retrieved command args for update", zap.Any("args", cmdArgs))
	err = validateConfig(cmdArgs.StdinData)
	if err != nil {
		logger.Error("Failed to handle CNI UPDATE", zap.Error(err))
		return isupdate, err
	}

	err = update(cmdArgs)
	if err != nil {
		logger.Error("Failed to handle CNI UPDATE", zap.Error(err))
		return isupdate, err
	}

	return isupdate, nil
}

func PrintCNIError(msg string) {
	logger.Error(msg)
	cniErr := &cniTypes.Error{
		Code: cniTypes.ErrTryAgainLater,
		Msg:  msg,
	}
	cniErr.Print()
}
