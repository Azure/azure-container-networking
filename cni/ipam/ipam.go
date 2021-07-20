// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package ipam

import (
	"encoding/json"
	"net"
	"strconv"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/ipam"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/platform"

	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/current"
)

const (
	ipamV6 = "azure-vnet-ipamv6"
)

var (
	ipv4DefaultRouteDstPrefix = net.IPNet{net.IPv4zero, net.IPv4Mask(0, 0, 0, 0)}
)

// IpamPlugin represents the CNI IPAM plugin.
type ipamPlugin struct {
	*cni.Plugin
	am ipam.AddressManager
}

// NewPlugin creates a new ipamPlugin object.
func NewPlugin(name string, config *common.PluginConfig) (*ipamPlugin, error) {
	// Setup base plugin.
	plugin, err := cni.NewPlugin(name, config.Version)
	if err != nil {
		return nil, err
	}

	// Setup address manager.
	am, err := ipam.NewAddressManager()
	if err != nil {
		return nil, err
	}

	// Create IPAM plugin.
	ipamPlg := &ipamPlugin{
		Plugin: plugin,
		am:     am,
	}

	config.IpamApi = ipamPlg

	return ipamPlg, nil
}

// Starts the plugin.
func (plugin *ipamPlugin) Start(config *common.PluginConfig) error {
	// Initialize base plugin.
	err := plugin.Initialize(config)
	if err != nil {
		log.Printf("[cni-ipam] Failed to initialize base plugin, err:%v.", err)
		return err
	}

	// Log platform information.
	log.Printf("[cni-ipam] Plugin %v version %v.", plugin.Name, plugin.Version)
	log.Printf("[cni-ipam] Running on %v", platform.GetOSInfo())

	// Initialize address manager. rehyrdration not required on reboot for cni ipam plugin
	err = plugin.am.Initialize(config, false, plugin.Options)
	if err != nil {
		log.Printf("[cni-ipam] Failed to initialize address manager, err:%v.", err)
		return err
	}

	log.Printf("[cni-ipam] Plugin started.")

	return nil
}

// Stops the plugin.
func (plugin *ipamPlugin) Stop() {
	plugin.am.Uninitialize()
	plugin.Uninitialize()
	log.Printf("[cni-ipam] Plugin stopped.")
}

// Configure parses and applies the given network configuration.
func (plugin *ipamPlugin) Configure(stdinData []byte) (*cni.NetworkConfig, error) {
	// Parse network configuration from stdin.
	nwCfg, err := cni.ParseNetworkConfig(stdinData)
	if err != nil {
		return nil, err
	}

	log.Printf("[cni-ipam] Read network configuration %+v.", nwCfg)

	// Apply IPAM configuration.

	// Set deployment environment.
	if nwCfg.Ipam.Environment == "" {
		nwCfg.Ipam.Environment = common.OptEnvironmentAzure
	}
	plugin.SetOption(common.OptEnvironment, nwCfg.Ipam.Environment)

	// Set query interval.
	if nwCfg.Ipam.QueryInterval != "" {
		i, _ := strconv.Atoi(nwCfg.Ipam.QueryInterval)
		plugin.SetOption(common.OptIpamQueryInterval, i)
	}

	err = plugin.am.StartSource(plugin.Options)
	if err != nil {
		return nil, err
	}

	// Set default address space if not specified.
	if nwCfg.Ipam.AddrSpace == "" {
		nwCfg.Ipam.AddrSpace = ipam.LocalDefaultAddressSpaceId
	}

	return nwCfg, nil
}

//
// CNI implementation
// https://github.com/containernetworking/cni/blob/master/SPEC.md
//

// Add handles CNI add commands.
func (plugin *ipamPlugin) Add(args *cniSkel.CmdArgs) error {
	var result *cniTypesCurr.Result
	var err error

	log.Printf("[cni-ipam] Processing ADD command with args {ContainerID:%v Netns:%v IfName:%v Args:%v Path:%v StdinData:%s}.",
		args.ContainerID, args.Netns, args.IfName, args.Args, args.Path, args.StdinData)

	defer func() { log.Printf("[cni-ipam] ADD command completed with result:%+v err:%v.", result, err) }()

	// Parse network configuration from stdin.
	nwCfg, err := plugin.Configure(args.StdinData)
	if err != nil {
		err = plugin.Errorf("Failed to parse network configuration: %v", err)
		return err
	}

	// assign the container id
	options := make(map[string]string)
	options[ipam.OptAddressID] = args.ContainerID

	// Check if an address pool is specified.
	if nwCfg.Ipam.Subnet == "" {
		var poolID string
		var subnet string

		isIpv6 := false
		if nwCfg.Ipam.Type == ipamV6 {
			isIpv6 = true
		}

		// Select the requested interface.
		options[ipam.OptInterfaceName] = nwCfg.Master

		// Allocate an address pool.
		poolID, subnet, err = plugin.am.RequestPool(nwCfg.Ipam.AddrSpace, "", "", options, isIpv6)
		if err != nil {
			err = plugin.Errorf("Failed to allocate pool: %v", err)
			return err
		}

		// On failure, release the address pool.
		defer func() {
			if err != nil && poolID != "" {
				log.Printf("[cni-ipam] Releasing pool %v.", poolID)
				plugin.am.ReleasePool(nwCfg.Ipam.AddrSpace, poolID)
			}
		}()

		nwCfg.Ipam.Subnet = subnet
		log.Printf("[cni-ipam] Allocated address poolID %v with subnet %v.", poolID, subnet)
	}

	// Allocate an address for the endpoint.
	address, err := plugin.am.RequestAddress(nwCfg.Ipam.AddrSpace, nwCfg.Ipam.Subnet, nwCfg.Ipam.Address, options)
	if err != nil {
		err = plugin.Errorf("Failed to allocate address: %v", err)
		return err
	}

	// On failure, release the address.
	defer func() {
		if err != nil && address != "" {
			log.Printf("[cni-ipam] Releasing address %v.", address)
			plugin.am.ReleaseAddress(nwCfg.Ipam.AddrSpace, nwCfg.Ipam.Subnet, address, options)
		}
	}()

	log.Printf("[cni-ipam] Allocated address %v.", address)

	// Parse IP address.
	ipAddress, err := platform.ConvertStringToIPNet(address)
	if err != nil {
		err = plugin.Errorf("Failed to parse address: %v", err)
		return err
	}

	// Query pool information for gateways and DNS servers.
	apInfo, err := plugin.am.GetPoolInfo(nwCfg.Ipam.AddrSpace, nwCfg.Ipam.Subnet)
	if err != nil {
		err = plugin.Errorf("Failed to get pool information: %v", err)
		return err
	}

	version := "4"
	if ipAddress.IP.To4() == nil {
		version = "6"
	}

	// Populate result.
	result = &cniTypesCurr.Result{
		IPs: []*cniTypesCurr.IPConfig{
			{
				Version: version,
				Address: *ipAddress,
				Gateway: apInfo.Gateway,
			},
		},
		Routes: []*cniTypes.Route{
			{
				Dst: ipv4DefaultRouteDstPrefix,
				GW:  apInfo.Gateway,
			},
		},
	}

	// Populate DNS servers.
	for _, dnsServer := range apInfo.DnsServers {
		result.DNS.Nameservers = append(result.DNS.Nameservers, dnsServer.String())
	}

	// Convert result to the requested CNI version.
	res, err := result.GetAsVersion(nwCfg.CNIVersion)
	if err != nil {
		err = plugin.Errorf("Failed to convert result: %v", err)
		return err
	}

	// Output the result.
	if nwCfg.Ipam.Type == cni.Internal {
		// Called via the internal interface. Pass output back in args.
		args.StdinData, _ = json.Marshal(res)
	} else {
		// Called via the executable interface. Print output to stdout.
		res.Print()
	}

	return nil
}

// Get handles CNI Get commands.
func (plugin *ipamPlugin) Get(args *cniSkel.CmdArgs) error {
	return nil
}

// Delete handles CNI delete commands.
func (plugin *ipamPlugin) Delete(args *cniSkel.CmdArgs) error {
	var err error

	log.Printf("[cni-ipam] Processing DEL command with args {ContainerID:%v Netns:%v IfName:%v Args:%v Path:%v StdinData:%s}.",
		args.ContainerID, args.Netns, args.IfName, args.Args, args.Path, args.StdinData)

	defer func() { log.Printf("[cni-ipam] DEL command completed with err:%v.", err) }()

	// Parse network configuration from stdin.
	nwCfg, err := plugin.Configure(args.StdinData)
	if err != nil {
		err = plugin.Errorf("Failed to parse network configuration: %v", err)
		return err
	}

	// Select the requested interface.
	options := make(map[string]string)
	options[ipam.OptAddressID] = args.ContainerID

	err = plugin.am.ReleaseAddress(nwCfg.Ipam.AddrSpace, nwCfg.Ipam.Subnet, nwCfg.Ipam.Address, options)

	if err != nil {
		err = plugin.Errorf("Failed to release address: %v", err)
		return err
	}

	return nil
}

// Update handles CNI update command.
func (plugin *ipamPlugin) Update(args *cniSkel.CmdArgs) error {
	return nil
}
