package epcommon

import (
	"net"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/netlink"
)

func CreateEndpoint(hostVethName string, containerVethName string) error {
	log.Printf("[net] Creating veth pair %v %v.", hostVethName, containerVethName)

	link := netlink.VEthLink{
		LinkInfo: netlink.LinkInfo{
			Type: netlink.LINK_TYPE_VETH,
			Name: hostVethName,
		},
		PeerName: containerVethName,
	}

	err := netlink.AddLink(&link)
	if err != nil {
		log.Printf("[net] Failed to create veth pair, err:%v.", err)
		return err
	}

	log.Printf("[net] Setting link %v state up.", hostVethName)
	err = netlink.SetLinkState(hostVethName, true)
	if err != nil {
		return err
	}

	return nil
}

func SetupContainerInterface(containerVethName string, targetIfName string) error {
	// Interface needs to be down before renaming.
	log.Printf("[net] Setting link %v state down.", containerVethName)
	if err := netlink.SetLinkState(containerVethName, false); err != nil {
		return err
	}

	// Rename the container interface.
	log.Printf("[net] Setting link %v name %v.", containerVethName, targetIfName)
	if err := netlink.SetLinkName(containerVethName, targetIfName); err != nil {
		return err
	}

	// Bring the interface back up.
	log.Printf("[net] Setting link %v state up.", targetIfName)
	return netlink.SetLinkState(targetIfName, true)
}

func AssignIPToInterface(interfaceName string, ipAddresses []net.IPNet) error {
	// Assign IP address to container network interface.
	for _, ipAddr := range ipAddresses {
		log.Printf("[net] Adding IP address %v to link %v.", ipAddr.String(), interfaceName)
		err := netlink.AddIpAddress(interfaceName, ipAddr.IP, &ipAddr)
		if err != nil {
			return err
		}
	}

	return nil
}
