package nmagent_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/nmagent"
)

func TestErrorTemp(t *testing.T) {
	errorTests := []struct {
		name       string
		err        nmagent.Error
		shouldTemp bool
	}{
		{
			"regular",
			nmagent.Error{
				Code: http.StatusInternalServerError,
			},
			false,
		},
		{
			"processing",
			nmagent.Error{
				Code: http.StatusProcessing,
			},
			true,
		},
		{
			"unauthorized",
			nmagent.Error{
				Code: http.StatusUnauthorized,
			},
			false,
		},
	}

	for _, test := range errorTests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if test.err.Temporary() && !test.shouldTemp {
				t.Fatal("test was temporary and not expected to be")
			}

			if !test.err.Temporary() && test.shouldTemp {
				t.Fatal("test was not temporary but expected to be")
			}
		})
	}
}

func TestContentErrorNew(t *testing.T) {
	errTests := []struct {
		name          string
		body          io.Reader
		limit         int64
		contentType   string
		exp           string
		shouldMakeErr bool
	}{
		{
			"empty",
			strings.NewReader(""),
			0,
			"text/plain",
			"unexpected content type \"text/plain\": body: ",
			true,
		},
		{
			"happy path",
			strings.NewReader("random text"),
			11,
			"text/plain",
			"unexpected content type \"text/plain\": body: random text",
			true,
		},
		{
			// if the body is an octet stream, it's entirely possible that it's
			// unprintable garbage. This ensures that we just print the length
			"octets",
			bytes.NewReader([]byte{0xde, 0xad, 0xbe, 0xef}),
			4,
			"application/octet-stream",
			"unexpected content type \"application/octet-stream\": body length: 4",
			true,
		},
		{
			// even if the length is wrong, we still want to return as much data as
			// we can for debugging
			"wrong len",
			bytes.NewReader([]byte{0xde, 0xad, 0xbe, 0xef}),
			8,
			"application/octet-stream",
			"unexpected content type \"application/octet-stream\": body length: 4",
			true,
		},
	}

	for _, test := range errTests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := nmagent.NewContentError(test.contentType, test.body, test.limit)

			var e nmagent.ContentError
			wasContentErr := errors.As(err, &e)
			if !wasContentErr && test.shouldMakeErr {
				t.Fatalf("error was not a ContentError")
			}

			if wasContentErr && !test.shouldMakeErr {
				t.Fatalf("received a ContentError when it was not expected")
			}

			got := err.Error()
			if got != test.exp {
				t.Error("unexpected error message: got:", got, "exp:", test.exp)
			}
		})
	}
}

// testContext creates a context from the provided testing.T that will be
// canceled if the test suite is terminated.
func testContext(t *testing.T) (context.Context, context.CancelFunc) {
	if deadline, ok := t.Deadline(); ok {
		return context.WithDeadline(context.Background(), deadline)
	}
	return context.WithCancel(context.Background())
}

// checkErr is an assertion of the presence or absence of an error
func checkErr(t *testing.T, err error, shouldErr bool) {
	t.Helper()
	if err != nil && !shouldErr {
		t.Fatal("unexpected error: err:", err)
	}

	if err == nil && shouldErr {
		t.Fatal("expected error but received none")
	}
}
