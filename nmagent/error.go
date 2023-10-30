package nmagent

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"

	"github.com/Azure/azure-container-networking/nmagent/internal"
	pkgerrors "github.com/pkg/errors"
)

var deleteNetworkPattern = regexp.MustCompile(`/NetworkManagement/joinedVirtualNetworks/[^/]+/api-version/\d+/method/DELETE`)

// ContentError is encountered when an unexpected content type is obtained from
// NMAgent.
type ContentError struct {
	Type string // the mime type of the content received
	Body []byte // the received body
}

func (c ContentError) Error() string {
	if c.Type == internal.MimeOctetStream {
		return fmt.Sprintf("unexpected content type %q: body length: %d", c.Type, len(c.Body))
	}
	return fmt.Sprintf("unexpected content type %q: body: %s", c.Type, c.Body)
}

// NewContentError creates a ContentError from a provided reader and limit.
func NewContentError(contentType string, in io.Reader, limit int64) error {
	out := ContentError{
		Type: contentType,
		Body: make([]byte, limit),
	}

	bodyReader := io.LimitReader(in, limit)

	read, err := io.ReadFull(bodyReader, out.Body)
	earlyEOF := errors.Is(err, io.ErrUnexpectedEOF)
	if err != nil && !earlyEOF {
		return pkgerrors.Wrap(err, "reading unexpected content body")
	}

	if earlyEOF {
		out.Body = out.Body[:read]
	}

	return out
}

// Error is a aberrent condition encountered when interacting with the NMAgent
// API.
type Error struct {
	Code   int    // the HTTP status code received
	Source string // the component responsible for producing the error
	Body   []byte // the body of the error returned
	Path   string // the path of the request that produced the error
}

// Error constructs a string representation of this error in accordance with
// the error interface.
func (e Error) Error() string {
	return fmt.Sprintf("nmagent: %s: http status %d: %s: body: %s", e.source(), e.Code, e.Message(), string(e.Body))
}

func (e Error) source() string {
	source := "not provided"
	if e.Source != "" {
		source = e.Source
	}
	return fmt.Sprintf("source: %s", source)
}

// Message interprets the HTTP Status code from NMAgent and returns the
// corresponding explanation from the documentation.
func (e Error) Message() string {
	switch e.Code {
	case http.StatusProcessing:
		return "the request is taking time to process. the caller should try the request again"
	case http.StatusUnauthorized:
		return "the request did not originate from an interface with an OwningServiceInstanceId property"
	case http.StatusInternalServerError:
		return "error occurred during nmagent's request processing"
	default:
		return "undocumented error"
	}
}

// Temporary reports whether the error encountered from NMAgent should be
// considered temporary, and thus retriable.
func (e Error) Temporary() bool {
	// NMAgent will return a 102 (Processing) if the request is taking time to
	// complete. These should be attempted again.
	return e.Code == http.StatusProcessing
}

// StatusCode returns the HTTP status associated with this error.
func (e Error) StatusCode() int {
	return e.Code
}

// Unauthorized reports whether the error was produced as a result of
// submitting the request from an interface without an OwningServiceInstanceId
// property. In some cases, this can be a transient condition that could be
// retried.
func (e Error) Unauthorized() bool {
	return e.Code == http.StatusUnauthorized
}

// NotFound reports whether the error was produced as a result of the resource
// not existing.
func (e Error) NotFound() bool {
	if deleteNetworkPattern.MatchString(e.Path) {
		return e.Code == http.StatusBadRequest || e.Code == http.StatusNotFound
	}
	return e.Code == http.StatusNotFound
}
