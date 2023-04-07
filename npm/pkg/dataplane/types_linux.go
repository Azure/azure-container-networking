package dataplane

// npmEndpoint holds info relevant for endpoints in windows
type npmEndpoint struct {
	netPolReference map[string]struct{}
}

type endpointQuery struct{}
