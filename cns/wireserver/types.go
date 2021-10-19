package wireserver

// InterfaceInfo specifies the information about an interface as returned by Host Agent.
type InterfaceInfo struct {
	Subnet       string
	Gateway      string
	IsPrimary    bool
	PrimaryIP    string
	SecondaryIPs []string
}

type Address struct {
	Address   string `xml:"Address,attr"`
	IsPrimary bool   `xml:"IsPrimary,attr"`
}

type Subnet struct {
	Prefix    string `xml:"Prefix,attr"`
	IPAddress []Address
}

type Interface struct {
	MacAddress string `xml:"MacAddress,attr"`
	IsPrimary  bool   `xml:"IsPrimary,attr"`
	IPSubnet   []Subnet
}

// GetInterfacesResult is the xml mapped response of the getInterfacesQuery
type GetInterfacesResult struct {
	Interface []Interface
}
