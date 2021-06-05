package types

type ResponseCode int

const (
	Success                                ResponseCode = 0
	UnsupportedNetworkType                 ResponseCode = 1
	InvalidParameter                       ResponseCode = 2
	UnsupportedEnvironment                 ResponseCode = 3
	UnreachableHost                        ResponseCode = 4
	ReservationNotFound                    ResponseCode = 5
	MalformedSubnet                        ResponseCode = 8
	UnreachableDockerDaemon                ResponseCode = 9
	UnspecifiedNetworkName                 ResponseCode = 10
	NotFound                               ResponseCode = 14
	AddressUnavailable                     ResponseCode = 15
	NetworkContainerNotSpecified           ResponseCode = 16
	CallToHostFailed                       ResponseCode = 17
	UnknownContainerID                     ResponseCode = 18
	UnsupportedOrchestratorType            ResponseCode = 19
	DockerContainerNotSpecified            ResponseCode = 20
	UnsupportedVerb                        ResponseCode = 21
	UnsupportedNetworkContainerType        ResponseCode = 22
	InvalidRequest                         ResponseCode = 23
	NetworkJoinFailed                      ResponseCode = 24
	NetworkContainerPublishFailed          ResponseCode = 25
	NetworkContainerUnpublishFailed        ResponseCode = 26
	InvalidPrimaryIPConfig                 ResponseCode = 27
	PrimaryCANotSame                       ResponseCode = 28
	InconsistentIPConfigState              ResponseCode = 29
	InvalidSecondaryIPConfig               ResponseCode = 30
	NetworkContainerVfpProgramPending      ResponseCode = 31
	FailedToAllocateIpConfig               ResponseCode = 32
	EmptyOrchestratorContext               ResponseCode = 33
	UnsupportedOrchestratorContext         ResponseCode = 34
	NetworkContainerVfpProgramComplete     ResponseCode = 35
	NetworkContainerVfpProgramCheckSkipped ResponseCode = 36
	NmAgentSupportedApisError              ResponseCode = 37
	UnsupportedNCVersion                   ResponseCode = 38
	UnexpectedError                        ResponseCode = 99
)

func (c ResponseCode) String() string {
	switch c {
	case Success:
		return "Success"
	case UnsupportedNetworkType:
		return "UnsupportedNetworkType"
	case InvalidParameter:
		return "InvalidParameter"
	case UnreachableHost:
		return "UnreachableHost"
	case ReservationNotFound:
		return "ReservationNotFound"
	case MalformedSubnet:
		return "MalformedSubnet"
	case UnreachableDockerDaemon:
		return "UnreachableDockerDaemon"
	case UnspecifiedNetworkName:
		return "UnspecifiedNetworkName"
	case NotFound:
		return "NotFound"
	case AddressUnavailable:
		return "AddressUnavailable"
	case NetworkContainerNotSpecified:
		return "NetworkContainerNotSpecified"
	case CallToHostFailed:
		return "CallToHostFailed"
	case UnknownContainerID:
		return "UnknownContainerID"
	case UnsupportedOrchestratorType:
		return "UnsupportedOrchestratorType"
	case UnexpectedError:
		return "UnexpectedError"
	case DockerContainerNotSpecified:
		return "DockerContainerNotSpecified"
	case NetworkContainerVfpProgramPending:
		return "NetworkContainerVfpProgramPending"
	default:
		return "UnknownError"
	}
}
