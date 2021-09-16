package network

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/cnsclient"
	"github.com/Azure/azure-container-networking/iptables"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/telemetry"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/current"
)

// Update handles CNI update commands.
// Update is only supported for multitenancy and to update routes.
func (plugin *netPlugin) Update(args *cniSkel.CmdArgs) error {
	var (
		result              *cniTypesCurr.Result
		err                 error
		nwCfg               *cni.NetworkConfig
		existingEpInfo      *network.EndpointInfo
		podCfg              *cni.K8SPodEnvArgs
		cnsClient           *cnsclient.CNSClient
		orchestratorContext []byte
		targetNetworkConfig *cns.GetNetworkContainerResponse
		cniMetric           telemetry.AIMetric
	)

	startTime := time.Now()

	log.Printf("[cni-net] Processing UPDATE command with args {Netns:%v Args:%v Path:%v}.",
		args.Netns, args.Args, args.Path)

	// Parse network configuration from stdin.
	if nwCfg, err = cni.ParseNetworkConfig(args.StdinData); err != nil {
		err = plugin.Errorf("Failed to parse network configuration: %v.", err)
		return err
	}

	log.Printf("[cni-net] Read network configuration %+v.", nwCfg)

	iptables.DisableIPTableLock = nwCfg.DisableIPTableLock
	plugin.setCNIReportDetails(nwCfg, CNI_UPDATE, "")

	defer func() {
		operationTimeMs := time.Since(startTime).Milliseconds()
		cniMetric.Metric = aitelemetry.Metric{
			Name:             telemetry.CNIUpdateTimeMetricStr,
			Value:            float64(operationTimeMs),
			CustomDimensions: make(map[string]string),
		}
		SetCustomDimensions(&cniMetric, nwCfg, err)
		telemetry.SendCNIMetric(&cniMetric, plugin.tb)

		if result == nil {
			result = &cniTypesCurr.Result{}
		}

		// Convert result to the requested CNI version.
		res, vererr := result.GetAsVersion(nwCfg.CNIVersion)
		if vererr != nil {
			log.Printf("GetAsVersion failed with error %v", vererr)
			plugin.Error(vererr)
		}

		if err == nil && res != nil {
			// Output the result to stdout.
			res.Print()
		}

		log.Printf("[cni-net] UPDATE command completed with result:%+v err:%v.", result, err)
	}()

	// Parse Pod arguments.
	if podCfg, err = cni.ParseCniArgs(args.Args); err != nil {
		log.Printf("[cni-net] Error while parsing CNI Args during UPDATE %v", err)
		return err
	}

	k8sNamespace := string(podCfg.K8S_POD_NAMESPACE)
	if len(k8sNamespace) == 0 {
		errMsg := "Required parameter Pod Namespace not specified in CNI Args during UPDATE"
		log.Printf(errMsg)
		return plugin.Errorf(errMsg)
	}

	k8sPodName := string(podCfg.K8S_POD_NAME)
	if len(k8sPodName) == 0 {
		errMsg := "Required parameter Pod Name not specified in CNI Args during UPDATE"
		log.Printf(errMsg)
		return plugin.Errorf(errMsg)
	}

	// Initialize values from network config.
	networkID := nwCfg.Name

	// Query the network.
	if _, err = plugin.nm.GetNetworkInfo(networkID); err != nil {
		errMsg := fmt.Sprintf("Failed to query network during CNI UPDATE: %v", err)
		log.Printf(errMsg)
		return plugin.Errorf(errMsg)
	}

	// Query the existing endpoint since this is an update.
	// Right now, we do not support updating pods that have multiple endpoints.
	existingEpInfo, err = plugin.nm.GetEndpointInfoBasedOnPODDetails(networkID, k8sPodName, k8sNamespace, nwCfg.EnableExactMatchForPodName)
	if err != nil {
		plugin.Errorf("Failed to retrieve target endpoint for CNI UPDATE [name=%v, namespace=%v]: %v", k8sPodName, k8sNamespace, err)
		return err
	}

	log.Printf("Retrieved existing endpoint from state that may get update: %+v", existingEpInfo)

	// now query CNS to get the target routes that should be there in the networknamespace (as a result of update)
	log.Printf("Going to collect target routes for [name=%v, namespace=%v] from CNS.", k8sPodName, k8sNamespace)
	if cnsClient, err = cnsclient.InitCnsClient(nwCfg.CNSUrl, defaultRequestTimeout); err != nil {
		log.Printf("Initializing CNS client error in CNI Update%v", err)
		log.Printf(err.Error())
		return plugin.Errorf(err.Error())
	}

	// create struct with info for target POD
	podInfo := cns.KubernetesPodInfo{
		PodName:      k8sPodName,
		PodNamespace: k8sNamespace,
	}
	if orchestratorContext, err = json.Marshal(podInfo); err != nil {
		log.Printf("Marshalling KubernetesPodInfo failed with %v", err)
		return plugin.Errorf(err.Error())
	}

	if targetNetworkConfig, err = cnsClient.GetNetworkConfiguration(orchestratorContext); err != nil {
		log.Printf("GetNetworkConfiguration failed with %v", err)
		return plugin.Errorf(err.Error())
	}

	log.Printf("Network config received from cns for [name=%v, namespace=%v] is as follows -> %+v", k8sPodName, k8sNamespace, targetNetworkConfig)
	targetEpInfo := &network.EndpointInfo{}

	// get the target routes that should replace existingEpInfo.Routes inside the network namespace
	log.Printf("Going to collect target routes for [name=%v, namespace=%v] from targetNetworkConfig.", k8sPodName, k8sNamespace)
	if targetNetworkConfig.Routes != nil && len(targetNetworkConfig.Routes) > 0 {
		for _, route := range targetNetworkConfig.Routes {
			log.Printf("Adding route from routes to targetEpInfo %+v", route)
			_, dstIPNet, _ := net.ParseCIDR(route.IPAddress)
			gwIP := net.ParseIP(route.GatewayIPAddress)
			targetEpInfo.Routes = append(targetEpInfo.Routes, network.RouteInfo{Dst: *dstIPNet, Gw: gwIP, DevName: existingEpInfo.IfName})
			log.Printf("Successfully added route from routes to targetEpInfo %+v", route)
		}
	}

	log.Printf("Going to collect target routes based on Cnetaddressspace for [name=%v, namespace=%v] from targetNetworkConfig.", k8sPodName, k8sNamespace)
	ipconfig := targetNetworkConfig.IPConfiguration
	for _, ipRouteSubnet := range targetNetworkConfig.CnetAddressSpace {
		log.Printf("Adding route from cnetAddressspace to targetEpInfo %+v", ipRouteSubnet)
		dstIPNet := net.IPNet{IP: net.ParseIP(ipRouteSubnet.IPAddress), Mask: net.CIDRMask(int(ipRouteSubnet.PrefixLength), 32)}
		gwIP := net.ParseIP(ipconfig.GatewayIPAddress)
		route := network.RouteInfo{Dst: dstIPNet, Gw: gwIP, DevName: existingEpInfo.IfName}
		targetEpInfo.Routes = append(targetEpInfo.Routes, route)
		log.Printf("Successfully added route from cnetAddressspace to targetEpInfo %+v", ipRouteSubnet)
	}

	log.Printf("Finished collecting new routes in targetEpInfo as follows: %+v", targetEpInfo.Routes)
	log.Printf("Now saving existing infravnetaddress space if needed.")
	for _, ns := range nwCfg.PodNamespaceForDualNetwork {
		if k8sNamespace == ns {
			targetEpInfo.EnableInfraVnet = true
			targetEpInfo.InfraVnetAddressSpace = nwCfg.InfraVnetAddressSpace
			log.Printf("Saving infravnet address space %s for [%s-%s]",
				targetEpInfo.InfraVnetAddressSpace, existingEpInfo.PODNameSpace, existingEpInfo.PODName)
			break
		}
	}

	// Update the endpoint.
	log.Printf("Now updating existing endpoint %v with targetNetworkConfig %+v.", existingEpInfo.Id, targetNetworkConfig)
	if err = plugin.nm.UpdateEndpoint(networkID, existingEpInfo, targetEpInfo); err != nil {
		err = plugin.Errorf("Failed to update endpoint: %v", err)
		return err
	}

	msg := fmt.Sprintf("CNI UPDATE succeeded : Updated %+v podname %v namespace %v", targetNetworkConfig, k8sPodName, k8sNamespace)
	plugin.setCNIReportDetails(nwCfg, CNI_UPDATE, msg)

	return nil
}
