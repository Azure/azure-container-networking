package cnsclient

import (
	"fmt"
	"net"
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
	var ip net.IP

	// retrieve the primary interface that CNS is listening on
	interfaces, _ := net.Interfaces()
FindIP:
	for _, iface := range interfaces {
		if iface.Name == eth0InterfaceName || iface.Name == azure0InterfaceName {
			addrs, _ := iface.Addrs()
			for _, address := range addrs {
				if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
					ip = ipnet.IP
					break FindIP
				}
			}
		}
	}

	if ip == nil {
		return fmt.Errorf("Primary IP not found")
	}

	cnsurl := "http://" + ip.String() + ":10090"
	cnsClient, err := InitCnsClient(cnsurl)
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
