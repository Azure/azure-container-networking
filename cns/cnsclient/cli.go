package cnsclient

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Azure/azure-container-networking/cns"
)

const (
	getCmdArg            = "get"
	getAvailableArg      = "Available"
	getAllocatedArg      = "Allocated"
	getAllArg            = "All"
	getPendingReleaseArg = "PendingRelease"

	releaseArg = "release"

	eth0InterfaceName   = "eth0"
	azure0InterfaceName = "azure0"

	envCNSIPAddress = "CNSIpAddress"
	envCNSPort      = "CNSPort"
)

var (
	availableCmds = []string{
		getCmdArg,
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

	cnsClient, err := InitCnsClient("http://" + cnsIPAddress + ":" + cnsPort)
	if err != nil {
		return err
	}

	switch {
	case strings.EqualFold(getCmdArg, cmd):
		return getCmd(cnsClient, arg)
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

	default:
		states = append(states, cns.Allocated)
		states = append(states, cns.Available)
		states = append(states, cns.PendingRelease)
	}

	addr, err := client.GetIPAddressesMatchingStates(states...)
	if err != nil {
		return err
	}

	printIPAddresses(addr)
	return nil
}

// Sort the addresses based on IP, then write to stdout
func printIPAddresses(addrSlice []cns.IPAddressState) {
	sort.Slice(addrSlice, func(i, j int) bool {
		return addrSlice[i].IPAddress < addrSlice[j].IPAddress
	})

	for _, addr := range addrSlice {
		fmt.Printf("%+v\n", addr)
	}
}
