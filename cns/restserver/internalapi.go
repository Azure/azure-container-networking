// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package restserver

// This file contains the internal functions called by either HTTP APIs (api.go) or
// internal APIs (definde in internalapi.go).
// This will be used internally (say by RequestController in case of AKS)

// GetPartitionKey - Get dnc/service partition key
func (service *HTTPRestService) GetPartitionKey() (dncPartitionKey string) {
	service.RLock()
	dncPartitionKey = service.dncPartitionKey
	service.RUnlock()
	return
}
