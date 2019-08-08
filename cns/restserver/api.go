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
	UnexpectedError                 = 99
)

func ReturnCodeToString(returnCode int) (s string) {
	switch returnCode {
	case Success:
		s = "Success"
	case UnsupportedNetworkType:
		s = "UnsupportedNetworkType"
	case InvalidParameter:
		s = "InvalidParameter"
	case UnreachableHost:
		s = "UnreachableHost"
	case ReservationNotFound:
		s = "ReservationNotFound"
	case MalformedSubnet:
		s = "MalformedSubnet"
	case UnreachableDockerDaemon:
		s = "UnreachableDockerDaemon"
	case UnspecifiedNetworkName:
		s = "UnspecifiedNetworkName"
	case NotFound:
		s = "NotFound"
	case AddressUnavailable:
		s = "AddressUnavailable"
	case NetworkContainerNotSpecified:
		s = "NetworkContainerNotSpecified"
	case CallToHostFailed:
		s = "CallToHostFailed"
	case UnknownContainerID:
		s = "UnknownContainerID"
	case UnsupportedOrchestratorType:
		s = "UnsupportedOrchestratorType"
	case UnexpectedError:
		s = "UnexpectedError"
	case DockerContainerNotSpecified:
		s = "DockerContainerNotSpecified"
	default:
		s = "UnknownError"
	}

	return
}
