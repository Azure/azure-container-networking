package cnsclient

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/restserver"
)

const (
	getCmdArg            = "get"
	getAvailableArg      = "Available"
	getAllocatedArg      = "Allocated"
	getAllArg            = "All"
	getPendingReleaseArg = "PendingRelease"
	getPodCmdArg         = "getPodContexts"
	getInMemoryData      = "getInMemory"

	releaseArg = "release"

	eth0InterfaceName   = "eth0"
	azure0InterfaceName = "azure0"

	envCNSIPAddress = "CNSIpAddress"
	envCNSPort      = "CNSPort"
)

var (
	availableCmds = []string{
		getCmdArg,
		getPodCmdArg,
		getInMemoryData,
	}

	getFlags = []string{
		getAvailableArg,
		getAllocatedArg,
		getAllocatedArg,
	}
)

func HandleCNSClientCommands(cmd, arg string) error {
	cnsIPAddress := os.Getenv(envCNSIPAddress)
	cnsPort := os.Getenv(envCNSPort)

	cnsClient, err := InitCnsClient("http://"+cnsIPAddress+":"+cnsPort, 5*time.Second)
	if err != nil {
		return err
	}

	switch {
	case strings.EqualFold(getCmdArg, cmd):
		return getCmd(cnsClient, arg)
	case strings.EqualFold(getPodCmdArg, cmd):
		return getPodCmd(cnsClient)
	case strings.EqualFold(getInMemoryData, cmd):
		return getInMemory(cnsClient)
	default:
		return fmt.Errorf("No debug cmd supplied, options are: %v", getCmdArg)
	}
}

func getCmd(client *CNSClient, arg string) error {
	var states []string

	switch arg {
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

	addr, err := client.GetIPAddressesMatchingStates(states...)
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
		cns.IPConfigurationStatus.String(addr)
	}
}

func getPodCmd(client *CNSClient) error {

	resp, err := client.GetPodOrchestratorContext()
	if err != nil {
		return err
	}

	printPodContext(resp)
	return nil
}

func printPodContext(podContext map[string]string) {
	var i = 1
	for orchContext, podID := range podContext {
		fmt.Println(i, " ", orchContext, " : ", podID)
		i++
	}
}

func getInMemory(client *CNSClient) error {

	inmemoryData, err := client.GetHTTPServiceData()
	if err != nil {
		return err
	}

	printInMemoryStruct(inmemoryData.HttpRestServiceData)
	return nil
}

func printInMemoryStruct(data restserver.HttpRestServiceData) {
	fmt.Println("PodIPIDByOrchestratorContext: ", data.PodIPIDByPodInterfaceKey)
	fmt.Println("PodIPConfigState: ", data.PodIPConfigState)
	fmt.Println("IPAMPoolMonitor: ", data.IPAMPoolMonitor)
}
