package network

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns/cnsclient"
	"github.com/Azure/azure-container-networking/iptables"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/telemetry"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
)

// Delete handles CNI delete commands.
func (plugin *netPlugin) Delete(args *cniSkel.CmdArgs) error {
	var (
		err          error
		nwCfg        *cni.NetworkConfig
		k8sPodName   string
		k8sNamespace string
		networkId    string
		nwInfo       network.NetworkInfo
		epInfo       *network.EndpointInfo
		cniMetric    telemetry.AIMetric
		msg          string
	)

	startTime := time.Now()

	log.Printf("[cni-net] Processing DEL command with args {ContainerID:%v Netns:%v IfName:%v Args:%v Path:%v, StdinData:%s}.",
		args.ContainerID, args.Netns, args.IfName, args.Args, args.Path, args.StdinData)

	defer func() {
		log.Printf("[cni-net] DEL command completed with err:%v.", err)
	}()

	// Parse network configuration from stdin.
	if nwCfg, err = cni.ParseNetworkConfig(args.StdinData); err != nil {
		err = plugin.Errorf("[cni-net] Failed to parse network configuration: %v", err)
		return err
	}

	log.Printf("[cni-net] Read network configuration %+v.", nwCfg)

	// Parse Pod arguments.
	if k8sPodName, k8sNamespace, err = plugin.getPodInfo(args.Args); err != nil {
		log.Printf("[cni-net] Failed to get POD info due to error: %v", err)
	}

	plugin.setCNIReportDetails(nwCfg, CNI_DEL, "")
	iptables.DisableIPTableLock = nwCfg.DisableIPTableLock

	sendMetricFunc := func() {
		operationTimeMs := time.Since(startTime).Milliseconds()
		cniMetric.Metric = aitelemetry.Metric{
			Name:             telemetry.CNIDelTimeMetricStr,
			Value:            float64(operationTimeMs),
			CustomDimensions: make(map[string]string),
		}
		SetCustomDimensions(&cniMetric, nwCfg, err)
		telemetry.SendCNIMetric(&cniMetric, plugin.tb)
	}

	log.Printf("Execution mode :%s", nwCfg.ExecutionMode)
	if nwCfg.ExecutionMode == string(Baremetal) {

		log.Printf("Baremetal mode. Calling vnet agent for delete container")

		// schedule send metric before attempting delete
		defer sendMetricFunc()
		err, _ = plugin.nnsClient.DeleteContainerNetworking(context.Background(), k8sPodName, args.Netns)
		return err
	}

	if nwCfg.MultiTenancy {
		// Initialize CNSClient
		cnsclient.InitCnsClient(nwCfg.CNSUrl, defaultRequestTimeout)
	}

	switch nwCfg.Ipam.Type {
	case network.AzureCNS:
		plugin.ipamInvoker, err = NewCNSInvoker(k8sPodName, k8sNamespace)
		if err != nil {
			log.Printf("[cni-net] Creating network %v failed with err %v.", networkId, err)
			return err
		}
	default:
		plugin.ipamInvoker = NewAzureIpamInvoker(plugin, &nwInfo)
	}

	// Initialize values from network config.
	networkId, err = getNetworkName(k8sPodName, k8sNamespace, args.IfName, nwCfg)

	// If error is not found error, then we ignore it, to comply with CNI SPEC.
	if err != nil {
		log.Printf("[cni-net] Failed to extract network name from network config. error: %v", err)

		if !cnsclient.IsNotFound(err) {
			err = plugin.Errorf("Failed to extract network name from network config. error: %v", err)
			return err
		}
	}

	endpointId := GetEndpointID(args)

	// Query the network.
	if nwInfo, err = plugin.nm.GetNetworkInfo(networkId); err != nil {

		if !nwCfg.MultiTenancy {
			// attempt to release address associated with this Endpoint id
			// This is to ensure clean up is done even in failure cases
			err = plugin.ipamInvoker.Delete(nil, nwCfg, args, nwInfo.Options)
			if err != nil {
				log.Printf("Network not found, attempted to release address with error:  %v", err)
			}
		}

		// Log the error but return success if the endpoint being deleted is not found.
		plugin.Errorf("[cni-net] Failed to query network: %v", err)
		err = nil
		return err
	}

	// Query the endpoint.
	if epInfo, err = plugin.nm.GetEndpointInfo(networkId, endpointId); err != nil {

		if !nwCfg.MultiTenancy {
			// attempt to release address associated with this Endpoint id
			// This is to ensure clean up is done even in failure cases
			log.Printf("release ip ep not found")
			if err = plugin.ipamInvoker.Delete(nil, nwCfg, args, nwInfo.Options); err != nil {
				log.Printf("Endpoint not found, attempted to release address with error: %v", err)
			}
		}

		// Log the error but return success if the endpoint being deleted is not found.
		plugin.Errorf("[cni-net] Failed to query endpoint: %v", err)
		err = nil
		return err
	}

	// schedule send metric before attempting delete
	defer sendMetricFunc()
	// Delete the endpoint.
	if err = plugin.nm.DeleteEndpoint(networkId, endpointId); err != nil {
		err = plugin.Errorf("Failed to delete endpoint: %v", err)
		return err
	}

	if !nwCfg.MultiTenancy {
		// Call into IPAM plugin to release the endpoint's addresses.
		for _, address := range epInfo.IPAddresses {
			log.Printf("release ip:%s", address.IP.String())
			err = plugin.ipamInvoker.Delete(&address, nwCfg, args, nwInfo.Options)
			if err != nil {
				err = plugin.Errorf("Failed to release address %v with error: %v", address, err)
				return err
			}
		}
	} else if epInfo.EnableInfraVnet {
		nwCfg.Ipam.Subnet = nwInfo.Subnets[0].Prefix.String()
		nwCfg.Ipam.Address = epInfo.InfraVnetIP.IP.String()
		err = plugin.ipamInvoker.Delete(nil, nwCfg, args, nwInfo.Options)
		if err != nil {
			log.Printf("Failed to release address: %v", err)
			err = plugin.Errorf("Failed to release address %v with error: %v", nwCfg.Ipam.Address, err)
		}
	}

	msg = fmt.Sprintf("CNI DEL succeeded : Released ip %+v podname %v namespace %v", nwCfg.Ipam.Address, k8sPodName, k8sNamespace)
	plugin.setCNIReportDetails(nwCfg, CNI_DEL, msg)

	return err
}
