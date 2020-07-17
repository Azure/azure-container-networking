// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package restserver

// Container Network Service remote API Contract.
const (
	Success                         = 0
	UnsupportedNetworkType          = 1
	InvalidParameter                = 2
	UnsupportedEnvironment          = 3
	UnreachableHost                 = 4
	ReservationNotFound             = 5
	MalformedSubnet                 = 8
	UnreachableDockerDaemon         = 9
	UnspecifiedNetworkName          = 10
	NotFound                        = 14
	AddressUnavailable              = 15
	NetworkContainerNotSpecified    = 16
	CallToHostFailed                = 17
	UnknownContainerID              = 18
	UnsupportedOrchestratorType     = 19
	DockerContainerNotSpecified     = 20
	UnsupportedVerb                 = 21
	UnsupportedNetworkContainerType = 22
	InvalidRequest                  = 23
	NetworkJoinFailed               = 24
	NetworkContainerPublishFailed   = 25
	NetworkContainerUnpublishFailed = 26
	UnexpectedError                 = 99
)

const (
	// Key against which CNS state is persisted.
	storeKey        = "ContainerNetworkService"
	swiftAPIVersion = "1"
	attach          = "Attach"
	detach          = "Detach"
	// Rest service state identifier for named lock
	stateJoinedNetworks = "JoinedNetworks"
)
