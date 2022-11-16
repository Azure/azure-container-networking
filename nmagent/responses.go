package nmagent

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

type AzResponse struct {
	HomeAz uint `json:"homeAz"`
}
