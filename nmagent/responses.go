package nmagent

import (
	"encoding/json"

	"github.com/pkg/errors"
)

type VirtualNetwork struct {
	CNetSpace      string   `json:"cnetSpace"`
	DefaultGateway string   `json:"defaultGateway"`
	DNSServers     []string `json:"dnsServers"`
	Subnets        []Subnet `json:"subnets"`
	VNetSpace      string   `json:"vnetSpace"`
	VNetVersion    string   `json:"vnetVersion"`
}

type Subnet struct {
	AddressPrefix string `json:"addressPrefix"`
	SubnetName    string `json:"subnetName"`
	Tags          []Tag  `json:"tags"`
}

type Tag struct {
	Name string `json:"name"`
	Type string `json:"type"` // the type of the tag (e.g. "System" or "Custom")
}

type SupportedAPIsResponseXML struct {
	SupportedApis []string `xml:"type"`
}

// NCVersion is a response produced from requests for a network container's
// version.
type NCVersion struct {
	NetworkContainerID string `json:"networkContainerId"`
	Version            string `json:"version"` // the current network container version
}

// NetworkContainerListResponse is a collection of network container IDs mapped
// to their current versions.
type NCVersionList struct {
	Containers []NCVersion `json:"networkContainers"`
}

// HomeAZFix is an indication that a particular bugfix has been applied to some
// HomeAZ.
type HomeAZFix int

func (h HomeAZFix) String() string {
	switch h {
	case HomeAZFixInvalid:
		return "HomeAZFixInvalid"
	case HomeAZFixIPv6:
		return "HomeAZFixIPv6"
	default:
		return "Unknown HomeAZ Fix"
	}
}

const (
	HomeAZFixInvalid HomeAZFix = iota
	HomeAZFixIPv6
)

type AzResponse struct {
	HomeAz       uint
	AppliedFixes []HomeAZFix
}

func (az *AzResponse) UnmarshalJSON(in []byte) error {
	type resp struct {
		HomeAz     uint `json:"homeAz"`
		APIVersion uint `json:"apiVersion"`
	}

	var rsp resp
	err := json.Unmarshal(in, &rsp)
	if err != nil {
		return errors.Wrap(err, "unmarshaling raw home az response")
	}

	if rsp.APIVersion != 0 && rsp.APIVersion != 2 {
		return HomeAzAPIVersionError{
			ReceivedAPIVersion: rsp.APIVersion,
		}
	}

	az.HomeAz = rsp.HomeAz

	if rsp.APIVersion == 2 { // nolint:gomnd // ignore magic number 2
		az.AppliedFixes = append(az.AppliedFixes, HomeAZFixIPv6)
	}

	return nil
}

// ContainsFixes reports whether all fixes requested are present in the
// AzResponse returned.
func (az AzResponse) ContainsFixes(requestedFixes ...HomeAZFix) bool {
	for _, requested := range requestedFixes {
		found := false
		for _, present := range az.AppliedFixes {
			if requested == present {
				found = true
			}
		}

		if !found {
			return false
		}
	}
	return true
}

type NodeIP struct {
	Address   IPAddress `xml:"Address,attr"`
	IsPrimary bool      `xml:"IsPrimary,attr"`
}

type InterfaceSubnet struct {
	IPAddress []NodeIP `xml:"IPAddress"`
	Prefix    string   `xml:"Prefix,attr"`
}

type Interface struct {
	InterfaceSubnets []InterfaceSubnet `xml:"IPSubnet"`
	MacAddress       MACAddress        `xml:"MacAddress,attr"`
	IsPrimary        bool              `xml:"IsPrimary,attr"`
}

// Response from NMAgent for getinterfaceinfov1 (interface IP information)
// If we change this name, we need to tell the XML encoder to look for
// "Interfaces" in the respose.
type Interfaces struct {
	Entries []Interface `xml:"Interface"`
}
