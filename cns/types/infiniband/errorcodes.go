package infiniband

type ErrorCode int

const (
	Success                  ErrorCode = 0
	PodNotFound              ErrorCode = 1
	DeviceUnavailable        ErrorCode = 2
	DeviceNotFound           ErrorCode = 3
	AnnotationNotFound       ErrorCode = 4
	PodAlreadyAllocated      ErrorCode = 5
	InternalProgrammingError ErrorCode = 6
)
