package cnsclient

import (
	"fmt"
	"net"
	"sort"

	"github.com/Azure/azure-container-networking/cns"
	acn "github.com/Azure/azure-container-networking/common"
)

var (
	getArg          = "get"
	getAvailableArg = "Available"
	getAllocatedArg = "Allocated"
	getAllArg       = "All"

	releaseArg = "release"
)

func HandleCNSClientCommands(cmd, arg string) {
	var ip net.IP

	// retrieve the primary interface that CNS is listening on
	interfaces, _ := net.Interfaces()
	for _, iface := range interfaces {
		addrs, _ := iface.Addrs()
		for _, address := range addrs {
			if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
				ip = ipnet.IP
			}
		}
	}

	cnsurl := "http://" + ip.String() + ":10090"
	cnsClient, _ := InitCnsClient(cnsurl)
	switch cmd {
	case getArg:
		getCmd(cnsClient, arg)
	default:
		fmt.Printf("No debug cmd supplied, options are: %v", getArg)
	}
}

func getCmd(client *CNSClient, arg string) {
	switch arg {
	case getAvailableArg:
		fmt.Println(getAvailableArg)
		addr, err := client.GetIPAddressesMatchingStates(cns.Available)
		if err != nil {
			fmt.Println(err)
			return
		}

		printIPAddresses(addr)
	case getAllocatedArg:
		fmt.Println(getAllocatedArg)
		addr, err := client.GetIPAddressesMatchingStates(cns.Allocated)
		if err != nil {
			fmt.Println(err)
			return
		}

		printIPAddresses(addr)
	case getAllArg:
		fmt.Println(getAllArg)
		addr, err := client.GetIPAddressesMatchingStates(cns.Allocated, cns.Available)
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
		fmt.Printf("%v\n", addr)
	}
}
