package nmagent

const (
	// GetNmAgentSupportedApiURLFmt Api endpoint to get supported Apis of NMAgent
	GetNmAgentSupportedApiURLFmt       = "http://%s/machine/plugins/?comp=nmagent&type=GetSupportedApis"
	GetNetworkContainerVersionURLFmt   = "http://%s/machine/plugins/?comp=nmagent&type=NetworkManagement/interfaces/%s/networkContainers/%s/version/authenticationToken/%s/api-version/1"
	GetNcVersionListWithOutTokenURLFmt = "http://%s/machine/plugins/?comp=nmagent&type=NetworkManagement/interfaces/api-version/%s"
	JoinNetworkURLFmt                  = "NetworkManagement/joinedVirtualNetworks/%s/api-version/1"
	PutNetworkValueFmt                 = "NetworkManagement/interfaces/%s/networkContainers/%s/authenticationToken/%s/api-version/1"
	DeleteNetworkContainerURLFmt       = "NetworkManagement/interfaces/%s/networkContainers/%s/authenticationToken/%s/api-version/1/method/DELETE"
)

// WireServerIP - wire server ip
var (
	WireserverIP                           = "168.63.129.16"
	WireServerPath                         = "machine/plugins"
	WireServerScheme                       = "http"
	getNcVersionListWithOutTokenURLVersion = "2"
)

// NetworkContainerResponse - NMAgent response.
type NetworkContainerResponse struct {
	ResponseCode       string `json:"httpStatusCode"`
	NetworkContainerID string `json:"networkContainerId"`
	Version            string `json:"version"`
}

type ContainerInfo struct {
	NetworkContainerID string `json:"networkContainerId"`
	Version            string `json:"version"`
}

type NetworkContainerListResponse struct {
	ResponseCode string          `json:"httpStatusCode"`
	Containers   []ContainerInfo `json:"networkContainers"`
}
