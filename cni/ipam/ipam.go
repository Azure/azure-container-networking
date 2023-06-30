// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package ipam

import (
	"encoding/json"
	"net"
	"strconv"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cni/log"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/ipam"
	"github.com/Azure/azure-container-networking/platform"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/100"
	"go.uber.org/zap"
)

const ipamV6 = "azure-vnet-ipamv6"

var ipv4DefaultRouteDstPrefix = net.IPNet{
	IP:   net.IPv4zero,
	Mask: net.IPv4Mask(0, 0, 0, 0),
}

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
		log.Logger.Error("Failed to initialize base plugin.",
			zap.Error(err), zap.String("component", "cni-ipam"))
		return err
	}

	// Log platform information.
	log.Logger.Info("Plugin version.", zap.String("name", plugin.Name),
		zap.String("version", plugin.Version),
		zap.String("component", "cni-ipam"))
	log.Logger.Info("Running on",
		zap.String("platform", platform.GetOSInfo()),
		zap.String("component", "cni-ipam"))

	// Initialize address manager. rehyrdration not required on reboot for cni ipam plugin
	err = plugin.am.Initialize(config, false, plugin.Options)
	if err != nil {
		log.Logger.Error("Failed to initialize address manager",
			zap.String("error", err.Error()),
			zap.String("component", "cni-ipam"))
		return err
	}

	log.Logger.Info("Plugin started", zap.String("component", "cni-ipam"))

	return nil
}

// Stops the plugin.
func (plugin *ipamPlugin) Stop() {
	plugin.am.Uninitialize()
	plugin.Uninitialize()
	log.Logger.Info("Plugin stopped", zap.String("component", "cni-ipam"))
}

// Configure parses and applies the given network configuration.
func (plugin *ipamPlugin) Configure(stdinData []byte) (*cni.NetworkConfig, error) {
	// Parse network configuration from stdin.
	nwCfg, err := cni.ParseNetworkConfig(stdinData)
	if err != nil {
		return nil, err
	}

	log.Logger.Info("Read network configuration",
		zap.Any("config", nwCfg),
		zap.String("component", "cni-ipam"))

	// Apply IPAM configuration.

	// Set deployment environment.
	if nwCfg.IPAM.Environment == "" {
		nwCfg.IPAM.Environment = common.OptEnvironmentAzure
	}
	plugin.SetOption(common.OptEnvironment, nwCfg.IPAM.Environment)

	// Set query interval.
	if nwCfg.IPAM.QueryInterval != "" {
		i, _ := strconv.Atoi(nwCfg.IPAM.QueryInterval)
		plugin.SetOption(common.OptIpamQueryInterval, i)
	}

	err = plugin.am.StartSource(plugin.Options)
	if err != nil {
		return nil, err
	}

	// Set default address space if not specified.
	if nwCfg.IPAM.AddrSpace == "" {
		nwCfg.IPAM.AddrSpace = ipam.LocalDefaultAddressSpaceId
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

	log.Logger.Info("Processing ADD command",
		zap.String("ContainerId", args.ContainerID),
		zap.String("Netns", args.Netns),
		zap.String("IfName", args.IfName),
		zap.String("Args", args.Args),
		zap.String("Path", args.Path),
		zap.ByteString("StdinData", args.StdinData),
		zap.String("component", "cni-ipam"))

	defer func() {
		log.Logger.Info("ADD command completed",
			zap.Any("result", result),
			zap.Any("error:", err),
			zap.String("component", "cni-ipam"))
	}()

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
	if nwCfg.IPAM.Subnet == "" {
		var poolID string
		var subnet string

		isIpv6 := false
		if nwCfg.IPAM.Type == ipamV6 {
			isIpv6 = true
		}

		// Select the requested interface.
		options[ipam.OptInterfaceName] = nwCfg.Master

		// Allocate an address pool.
		poolID, subnet, err = plugin.am.RequestPool(nwCfg.IPAM.AddrSpace, "", "", options, isIpv6)
		if err != nil {
			err = plugin.Errorf("Failed to allocate pool: %v", err)
			return err
		}

		// On failure, release the address pool.
		defer func() {
			if err != nil && poolID != "" {
				log.Logger.Info("Releasing pool",
					zap.String("poolId", poolID),
					zap.String("component", "cni-ipam"))
				_ = plugin.am.ReleasePool(nwCfg.IPAM.AddrSpace, poolID)
			}
		}()

		nwCfg.IPAM.Subnet = subnet
		log.Logger.Info("Allocated address with subnet",
			zap.String("poolId", poolID),
			zap.String("subnet", subnet),
			zap.String("component", "cni-ipam"))
	}

	// Allocate an address for the endpoint.
	address, err := plugin.am.RequestAddress(nwCfg.IPAM.AddrSpace, nwCfg.IPAM.Subnet, nwCfg.IPAM.Address, options)
	if err != nil {
		err = plugin.Errorf("Failed to allocate address: %v", err)
		return err
	}

	// On failure, release the address.
	defer func() {
		if err != nil && address != "" {
			log.Logger.Info("Releasing address",
				zap.String("address", address),
				zap.String("component", "cni-ipam"))
			_ = plugin.am.ReleaseAddress(nwCfg.IPAM.AddrSpace, nwCfg.IPAM.Subnet, address, options)
		}
	}()

	log.Logger.Info("Allocated address",
		zap.String("address", address),
		zap.String("component", "cni-ipam"))

	// Parse IP address.
	ipAddress, err := platform.ConvertStringToIPNet(address)
	if err != nil {
		err = plugin.Errorf("Failed to parse address: %v", err)
		return err
	}

	// Query pool information for gateways and DNS servers.
	apInfo, err := plugin.am.GetPoolInfo(nwCfg.IPAM.AddrSpace, nwCfg.IPAM.Subnet)
	if err != nil {
		err = plugin.Errorf("Failed to get pool information: %v", err)
		return err
	}

	// Populate result.
	result = &cniTypesCurr.Result{
		IPs: []*cniTypesCurr.IPConfig{
			{
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
	if nwCfg.IPAM.Type == cni.Internal {
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

	log.Logger.Info("[cni-ipam] Processing DEL command",
		zap.String("ContainerId", args.ContainerID),
		zap.String("Netns", args.Netns),
		zap.String("IfName", args.IfName),
		zap.String("Args", args.Args),
		zap.String("Path", args.Path),
		zap.ByteString("StdinData", args.StdinData))

	defer func() {
		log.Logger.Info("[cni-ipam] DEL command completed",
			zap.Error(err))
	}()

	// Parse network configuration from stdin.
	nwCfg, err := plugin.Configure(args.StdinData)
	if err != nil {
		err = plugin.Errorf("Failed to parse network configuration: %v", err)
		return err
	}

	// Select the requested interface.
	options := make(map[string]string)
	options[ipam.OptAddressID] = args.ContainerID

	err = plugin.am.ReleaseAddress(nwCfg.IPAM.AddrSpace, nwCfg.IPAM.Subnet, nwCfg.IPAM.Address, options)

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
