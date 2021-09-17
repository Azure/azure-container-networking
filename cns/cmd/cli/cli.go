package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/client"
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
	var states []cns.IPConfigState

	switch cns.IPConfigState(arg) {
	case cns.Available:
		states = append(states, cns.Available)

	case cns.Allocated:
		states = append(states, cns.Allocated)

	case cns.PendingRelease:
		states = append(states, cns.PendingRelease)

	case cns.PendingProgramming:
		states = append(states, cns.PendingProgramming)

	default:
		states = append(states, cns.Allocated)
		states = append(states, cns.Available)
		states = append(states, cns.PendingRelease)
		states = append(states, cns.PendingProgramming)
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

func getInMemory(ctx context.Context, client *client.Client) error {
	data, err := client.GetHTTPServiceData(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("PodIPIDByOrchestratorContext: %v\nPodIPConfigState: %v\nIPAMPoolMonitor: %v\n",
		data.HTTPRestServiceData.PodIPIDByPodInterfaceKey, data.HTTPRestServiceData.PodIPConfigState, data.HTTPRestServiceData.IPAMPoolMonitor)
	return nil
}
