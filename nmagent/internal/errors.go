package internal

import "net/http"

// Error represents an internal sentinal error which can be defined as a
// constant.
type Error string

func (e Error) Error() string {
	return string(e)
}

// ErrorSource is an indicator used as a header value to indicate the source of
// non-2xx status codes.
type ErrorSource int

const (
	ErrorSourceInvalid ErrorSource = iota
	ErrorSourceWireserver
	ErrorSourceNMAgent
)

// String produces the string equivalent for the ErrorSource type.
func (e ErrorSource) String() string {
	switch e {
	case ErrorSourceWireserver:
		return "wireserver"
	case ErrorSourceNMAgent:
		return "nmagent"
	case ErrorSourceInvalid:
		return ""
	default:
		return ""
	}
}

// NewErrorSource produces an ErrorSource value from the provided string. Any
// unrecognized values will become the invalid type.
func NewErrorSource(es string) ErrorSource {
	switch es {
	case "wireserver":
		return ErrorSourceWireserver
	case "nmagent":
		return ErrorSourceNMAgent
	default:
		return ErrorSourceInvalid
	}
}

const (
	HeaderErrorSource = "X-Error-Source"
)

// GetErrorSource retrieves the error source from the provided HTTP headers.
func GetErrorSource(head http.Header) ErrorSource {
	return NewErrorSource(head.Get(HeaderErrorSource))
}

// SetErrorSource sets the header value necessary for communicating the error
// source.
func SetErrorSource(head *http.Header, es ErrorSource) {
	head.Set(HeaderErrorSource, es.String())
}
