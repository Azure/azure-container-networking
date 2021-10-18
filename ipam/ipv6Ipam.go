package ipam

import (
	"context"
	"errors"
	"net"
	"os"
	"regexp"
	"runtime"
	"strings"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Masterminds/semver"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	defaultWindowsKubePath           = `c:\k\`
	defaultWindowsKubeConfigFilePath = defaultWindowsKubePath + `config`
	defaultLinuxKubeConfigFilePath   = "/var/lib/kubelet/kubeconfig"
	k8sMajorVerForNewPolicyDef       = "1"
	k8sMinorVerForNewPolicyDef       = "16"

	// by default, Kubernetes node is allocated a /64 for each node. That's too much state to preserve,
	// so instead we take the first /120 of that /64 as usable IP's.
	defaultIPv6SubnetMaskSizeLimit = "/120"
	lessThan                       = -1
	equalTo                        = 0
	greaterThan                    = 1
	comparisonError                = -2
)

// regex to get minor version
var re = regexp.MustCompile("[0-9]+")

type ipv6IpamSource struct {
	name                string
	nodeHostname        string
	subnetMaskSizeLimit string
	kubeConfigPath      string
	kubeClient          kubernetes.Interface
	kubeNode            *v1.Node
	isLoaded            bool
	sink                addressConfigSink
}

// creates a new IPv6 Ipam source
func newIPv6IpamSource(options map[string]interface{}, isLoaded bool) (*ipv6IpamSource, error) {
	var kubeConfigPath string
	name := options[common.OptEnvironment].(string)

	if runtime.GOOS == windows {
		kubeConfigPath = defaultWindowsKubeConfigFilePath
	} else {
		kubeConfigPath = defaultLinuxKubeConfigFilePath
	}

	nodeName, err := os.Hostname()
	if err != nil {
		return nil, err
	}

	return &ipv6IpamSource{
		name:                name,
		subnetMaskSizeLimit: defaultIPv6SubnetMaskSizeLimit,
		nodeHostname:        strings.ToLower(nodeName),
		kubeConfigPath:      kubeConfigPath,
		isLoaded:            isLoaded,
	}, nil
}

// Starts the MAS source.
func (source *ipv6IpamSource) start(sink addressConfigSink) error {
	source.sink = sink
	return nil
}

// Stops the MAS source.
func (source *ipv6IpamSource) stop() {
	source.sink = nil
}

// creates a KubernetesClientset using the Kubeconfig stored on each agent node
func (source *ipv6IpamSource) loadKubernetesConfig() (kubernetes.Interface, error) {
	config, err := clientcmd.BuildConfigFromFlags("", source.kubeConfigPath)
	if err != nil {
		log.Printf("[ipam] Failed to load Kubernetes config from disk: %+v", err)
		return nil, err
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("[ipam] Failed to create Kubernetes config: %+v", err)
		return nil, err
	}

	minimumVersion := &version.Info{
		Major: k8sMajorVerForNewPolicyDef,
		Minor: k8sMinorVerForNewPolicyDef,
	}

	serverVersion, err := client.ServerVersion()
	if err != nil {
		log.Printf("[ipam] Failed to retrieve Kubernetes version: %+v", err)
		return nil, err
	}

	comparison := CompareK8sVer(serverVersion, minimumVersion)
	if comparison == equalTo || comparison == lessThan {
		return nil, errors.New("Incompatible Kubernetes version for dual stack")
	} else if comparison == comparisonError {
		return nil, errors.New("Error comparing Kubernetes API versions")
	}

	return client, err
}

// CompareK8sVer compares two k8s versions.
// returns -1, 0, 1 if firstVer smaller, equals, bigger than secondVer respectively.
// returns -2 for error.
func CompareK8sVer(firstVer *version.Info, secondVer *version.Info) int {
	v1Minor := re.FindAllString(firstVer.Minor, -1)
	if len(v1Minor) < 1 {
		return comparisonError
	}

	v1, err := semver.NewVersion(firstVer.Major + "." + v1Minor[0])
	if err != nil {
		return comparisonError
	}

	v2Minor := re.FindAllString(secondVer.Minor, -1)
	if len(v2Minor) < 1 {
		return comparisonError
	}

	v2, err := semver.NewVersion(secondVer.Major + "." + v2Minor[0])
	if err != nil {
		return comparisonError
	}

	return v1.Compare(v2)
}

func (source *ipv6IpamSource) refresh() error {
	if source == nil {
		return errors.New("ipv6ipam is nil")
	}

	if source.isLoaded {
		log.Printf("ipv6 source already loaded")
		return nil
	}

	if source.kubeClient == nil {
		kubeClient, err := source.loadKubernetesConfig()
		if err != nil {
			log.Printf("[ipam] Failed to load Kubernetes config: %+v", err)
			return err
		}

		source.kubeClient = kubeClient
	}

	kubeNode, err := source.kubeClient.CoreV1().Nodes().Get(context.TODO(), source.nodeHostname, metav1.GetOptions{})
	if err != nil {
		log.Printf("[ipam] Failed to retrieve node using hostname: %+v with err %+v", source.nodeHostname, err)
		return err
	}

	source.kubeNode = kubeNode
	log.Printf("[ipam] Discovered CIDR's %v.", source.kubeNode.Spec.PodCIDRs)

	// Query the list of Kubernetes Pod IPs
	interfaceIPs, err := retrieveKubernetesPodIPs(source.kubeNode, source.subnetMaskSizeLimit)
	if err != nil {
		log.Printf("[ipam] Failed retrieve Kubernetes IP's from subnet: %v.", err)
		return err
	}

	// Configure the local default address space.
	local, err := source.sink.newAddressSpace(LocalDefaultAddressSpaceId, LocalScope)
	if err != nil {
		log.Printf("[ipam] Failed to configure local default address space: %v.", err)
		return err
	}

	for _, i := range interfaceIPs.Interfaces {
		for _, s := range i.IPSubnets {
			_, subnet, err := net.ParseCIDR(s.Prefix)
			ifaceName := ""
			priority := 0
			ap, err := local.newAddressPool(ifaceName, priority, subnet)

			for _, a := range s.IPAddresses {
				address := net.ParseIP(a.Address)
				_, err = ap.newAddressRecord(&address)
				if err != nil {
					log.Printf("[ipam] Failed to create address:%v err:%v.", address, err)
					continue
				}
			}
		}
	}

	// Set the local address space as active.
	if err = source.sink.setAddressSpace(local); err != nil {
		return err
	}

	source.isLoaded = true
	log.Printf("[ipam] Address space successfully populated from Kubernetes API Server")

	return err
}

// retrieves the allocated pod IP's, and populates the NetworkInterfaces struture
func retrieveKubernetesPodIPs(node *v1.Node, subnetMaskBitSize string) (*NetworkInterfaces, error) {
	var nodeCidr net.IP
	var ipnetv6 *net.IPNet

	// get IPv6 subnet allocated to node
	for _, cidr := range node.Spec.PodCIDRs {
		ipv6cidr, _, _ := net.ParseCIDR(cidr)
		if ipv6cidr.To4() == nil {
			nodeCidr = ipv6cidr
			break
		}
	}

	if nodeCidr == nil {
		return nil, errors.New("[ipam] Failed to retrieve subnet, node does an IPv6 subnet allocated from Kubernetes")
	}

	subnet := nodeCidr.String() + subnetMaskBitSize
	nodeCidr, ipnetv6, err := net.ParseCIDR(subnet)
	if err != nil {
		return nil, err
	}

	addresses := getIPsFromAddresses(nodeCidr, ipnetv6)
	networkSubnet := IPSubnet{
		Prefix: subnet,
	}

	// skip the first address, explicitly save all IP's from the given subnet
	for i := 2; i < len(addresses); i++ {
		ipaddress := IPAddress{
			IsPrimary: false,
			Address:   addresses[i].String(),
		}
		networkSubnet.IPAddresses = append(networkSubnet.IPAddresses, ipaddress)
	}

	return &NetworkInterfaces{
		Interfaces: []Interface{
			{
				IsPrimary: true,
				IPSubnets: []IPSubnet{
					networkSubnet,
				},
			},
		},
	}, nil
}

// retrieves all IP's from a given subnet
func getIPsFromAddresses(ip net.IP, ipnet *net.IPNet) []net.IP {
	ips := make([]net.IP, 0)
	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); incrementIP(ip) {
		ips = append(ips, duplicateIP(ip))
	}

	return ips
}

// increment the IP by one
func incrementIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

// Create a copy of a net.IP, used to populate an slice of IP's in a subnet
// net.IP is slice of bytes, slices are reference types, when calculating every
// ip in a subnet, need to save current net.IP slice in it's own memory.
func duplicateIP(ip net.IP) net.IP {
	dup := make(net.IP, len(ip))
	copy(dup, ip)
	return dup
}
