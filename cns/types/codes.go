package types

type ResponseCode int

// ResponseCode definitions
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
	FailedToAllocateIPConfig               ResponseCode = 32
	EmptyOrchestratorContext               ResponseCode = 33
	UnsupportedOrchestratorContext         ResponseCode = 34
	NetworkContainerVfpProgramComplete     ResponseCode = 35
	NetworkContainerVfpProgramCheckSkipped ResponseCode = 36
	NmAgentSupportedApisError              ResponseCode = 37
	UnsupportedNCVersion                   ResponseCode = 38
	FailedToRunIPTableCmd                  ResponseCode = 39
	NilEndpointStateStore                  ResponseCode = 40
	UnexpectedError                        ResponseCode = 99
)

// nolint:gocyclo
func (c ResponseCode) String() string {
	switch c {
	case AddressUnavailable:
		return "AddressUnavailable"
	case CallToHostFailed:
		return "CallToHostFailed"
	case DockerContainerNotSpecified:
		return "DockerContainerNotSpecified"
	case EmptyOrchestratorContext:
		return "EmptyOrchestratorContext"
	case FailedToAllocateIPConfig:
		return "FailedToAllocateIpConfig"
	case InconsistentIPConfigState:
		return "InconsistentIPConfigState"
	case InvalidParameter:
		return "InvalidParameter"
	case InvalidPrimaryIPConfig:
		return "InvalidPrimaryIPConfig"
	case InvalidRequest:
		return "InvalidRequest"
	case InvalidSecondaryIPConfig:
		return "InvalidSecondaryIPConfig"
	case MalformedSubnet:
		return "MalformedSubnet"
	case NetworkContainerNotSpecified:
		return "NetworkContainerNotSpecified"
	case NetworkContainerPublishFailed:
		return "NetworkContainerPublishFailed"
	case NetworkContainerUnpublishFailed:
		return "NetworkContainerUnpublishFailed"
	case NetworkContainerVfpProgramCheckSkipped:
		return "NetworkContainerVfpProgramCheckSkipped"
	case NetworkContainerVfpProgramComplete:
		return "NetworkContainerVfpProgramComplete"
	case NetworkContainerVfpProgramPending:
		return "NetworkContainerVfpProgramPending"
	case NetworkJoinFailed:
		return "NetworkJoinFailed"
	case NmAgentSupportedApisError:
		return "NmAgentSupportedApisError"
	case NotFound:
		return "NotFound"
	case PrimaryCANotSame:
		return "PrimaryCANotSame"
	case ReservationNotFound:
		return "ReservationNotFound"
	case Success:
		return "Success"
	case UnexpectedError:
		return "UnexpectedError"
	case UnknownContainerID:
		return "UnknownContainerID"
	case UnreachableDockerDaemon:
		return "UnreachableDockerDaemon"
	case UnreachableHost:
		return "UnreachableHost"
	case UnspecifiedNetworkName:
		return "UnspecifiedNetworkName"
	case UnsupportedEnvironment:
		return "UnsupportedEnvironment"
	case UnsupportedNCVersion:
		return "UnsupportedNCVersion"
	case UnsupportedNetworkContainerType:
		return "UnsupportedNetworkContainerType"
	case UnsupportedNetworkType:
		return "UnsupportedNetworkType"
	case UnsupportedOrchestratorContext:
		return "UnsupportedOrchestratorContext"
	case UnsupportedOrchestratorType:
		return "UnsupportedOrchestratorType"
	case UnsupportedVerb:
		return "UnsupportedVerb"
	default:
		return "UnknownError"
	}
}
