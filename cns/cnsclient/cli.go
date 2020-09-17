package cnsclient

import (
	"fmt"
	"net"
	"sort"

	"github.com/Azure/azure-container-networking/cns"
	acn "github.com/Azure/azure-container-networking/common"
)

var (
	getArg = "get"

	availableCmds = []string{
		getArg,
	}

	getAvailableArg      = "Available"
	getAllocatedArg      = "Allocated"
	getAllArg            = "All"
	getPendingReleaseArg = "PendingRelease"

	getFlags = []string{
		getAvailableArg,
		getAllocatedArg,
		getAllocatedArg,
	}

	releaseArg = "release"

	eth0InterfaceName   = "eth0"
	azure0InterfaceName = "azure0"
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

	switch cmd {
	case getArg:
		getCmd(cnsClient, arg)
	default:
		return fmt.Errorf("No debug cmd supplied, options are: %v", getArg)
	}
	return nil
}

func getCmd(client *CNSClient, arg string) {
	switch arg {
	case cns.Available:
		addr, err := client.GetIPAddressesMatchingStates(cns.Available)
		if err != nil {
			fmt.Println(err)
			return
		}
		printIPAddresses(addr)

	case cns.Allocated:
		addr, err := client.GetIPAddressesMatchingStates(cns.Allocated)
		if err != nil {
			fmt.Println(err)
			return
		}
		printIPAddresses(addr)

	case getAllArg:
		addr, err := client.GetIPAddressesMatchingStates(cns.Allocated, cns.Available, cns.PendingRelease)
		if err != nil {
			fmt.Println(err)
			return
		}
		printIPAddresses(addr)

	case cns.PendingRelease:
		addr, err := client.GetIPAddressesMatchingStates(cns.PendingRelease)
		if err != nil {
			fmt.Println(err)
			return
		}
		printIPAddresses(addr)

	default:
		fmt.Printf("argument supplied for the get cmd, use the '%v' flag", acn.OptDebugCmdAlias)
	}
}

// Sort the addresses based on IP, then write to stdout
func printIPAddresses(addrSlice []cns.IPAddressState) {
	sort.Slice(addrSlice, func(i, j int) bool {
		if addrSlice[i].IPAddress < addrSlice[j].IPAddress {
			return true
		}

		if addrSlice[i].IPAddress > addrSlice[j].IPAddress {
			return false
		}

		return addrSlice[i].IPAddress < addrSlice[j].IPAddress
	})

	for _, addr := range addrSlice {
		fmt.Printf("%+v\n", addr)
	}
}
