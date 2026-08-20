package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/client"
	"github.com/Azure/azure-container-networking/cns/types"
)

const (
	envCNSIPAddress = "CNSIpAddress"
	envCNSPort      = "CNSPort"
	getCmdArg       = "get"
	getInMemoryData = "getInMemory"
	getPodCmdArg    = "getPodContexts"
)

func HandleCNSClientCommands(ctx context.Context, cmd string, arg string) error {
	cnsIPAddress := os.Getenv(envCNSIPAddress)
	cnsPort := os.Getenv(envCNSPort)

	cnsClient, err := client.New("http://"+cnsIPAddress+":"+cnsPort, client.DefaultTimeout)
	if err != nil {
		return err
	}

	switch {
	case strings.EqualFold(getCmdArg, cmd):
		return getCmd(ctx, cnsClient, arg)
	case strings.EqualFold(getPodCmdArg, cmd):
		return getPodCmd(ctx, cnsClient)
	case strings.EqualFold(getInMemoryData, cmd):
		return getInMemory(ctx, cnsClient)
	default:
		return fmt.Errorf("No debug cmd supplied, options are: %v", getCmdArg)
	}
}

func getCmd(ctx context.Context, client *client.Client, arg string) error {
	var states []types.IPState

	switch types.IPState(arg) {
	case types.Available:
		states = append(states, types.Available)
	case types.Assigned:
		states = append(states, types.Assigned)
	case types.PendingProgramming:
		states = append(states, types.PendingProgramming)
	case types.PendingRelease:
		states = append(states, types.PendingRelease)
	default:
		states = append(states, types.Assigned, types.Available, types.PendingProgramming, types.PendingRelease)
	}

	addr, err := client.GetIPAddressesMatchingStates(ctx, states...)
	if err != nil {
		return err
	}

	printIPAddresses(addr)
	return nil
}

// Sort the addresses based on IP, then write to stdout
func printIPAddresses(addrSlice []cns.IPConfigurationStatus) {
	sort.Slice(addrSlice, func(i, j int) bool {
		return addrSlice[i].IPAddress < addrSlice[j].IPAddress
	})

	for _, addr := range addrSlice {
		fmt.Println(addr.String())
	}
}

func getPodCmd(ctx context.Context, client *client.Client) error {
	resp, err := client.GetPodOrchestratorContext(ctx)
	if err != nil {
		return err
	}
	i := 1
	for orchContext, podID := range resp {
		fmt.Printf("%d %s : %s\n", i, orchContext, podID)
		i++
	}
	return nil
}

// getInMemory prints CNS's in-memory HTTPRestServiceData as JSON, matching the
// shape returned by the /debug/restdata HTTP endpoint. Callers (e.g. pipeline
// health checks) that previously curled /debug/restdata can invoke this
// command instead so they don't depend on curl/wget being present in the
// container image.
func getInMemory(ctx context.Context, client *client.Client) error {
	data, err := client.GetHTTPServiceData(ctx)
	if err != nil {
		return err
	}
	out, err := json.Marshal(data.HTTPRestServiceData)
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}
