package cns

// SupportedAPIsPath is the CNS capability discovery endpoint.
const SupportedAPIsPath = "/supportedapis"

// SupportedAPIsResponse advertises curated capabilities, not registered routes.
type SupportedAPIsResponse struct {
	SupportedAPIs []string `json:"supportedApis"`
}
