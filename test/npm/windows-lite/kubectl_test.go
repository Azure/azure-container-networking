//go:build linux

//nolint:goconst // Keep wire fixtures independent of executor constants.
package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestBoundedCommandOutput(t *testing.T) {
	for _, size := range []int{outputLimit, outputLimit + 1} {
		var destination boundedBuffer
		// Hide WriterTo so io.Copy also exercises a destination's promoted ReadFrom.
		source := struct{ io.Reader }{strings.NewReader(strings.Repeat("x", size))}
		_, err := io.Copy(&destination, source)
		if size > outputLimit {
			if !errors.Is(err, errOutputLimit) || !destination.overflow || destination.buffer.Len() > outputLimit {
				t.Fatalf("output cap bypassed: length %d, error %v", destination.buffer.Len(), err)
			}
		} else if err != nil || destination.buffer.Len() != size {
			t.Fatalf("exact-limit output rejected: %v", err)
		}
	}
	if _, err := readBounded(strings.NewReader(strings.Repeat("x", outputLimit+1))); !errors.Is(err, errOutputLimit) {
		t.Fatalf("file cap: %v", err)
	}
}

func TestStrictProbeVerdicts(t *testing.T) {
	tests := []struct {
		name   string
		result commandResult
		want   verdict
	}{
		{"allow", commandResult{}, allow},
		{"timeout", commandResult{code: 1, stderr: []byte("TIMEOUT\ncommand terminated with exit code 1\n")}, deny},
		{"refused", commandResult{code: 1, stderr: []byte("REFUSED\ncommand terminated with exit code 1\n")}, deny},
		{"bare remote stderr", commandResult{code: 1, stderr: []byte("TIMEOUT\n")}, ""},
		{"missing remote stderr", commandResult{code: 1, stderr: []byte("command terminated with exit code 1\n")}, ""},
		{"authorization", commandResult{code: 1, stderr: []byte("Error from server (Forbidden): TIMEOUT\n")}, ""},
		{"transport", commandResult{code: 1, stderr: []byte("error: unable to upgrade connection: timeout\n")}, ""},
		{"unknown", commandResult{code: 1, stderr: []byte("UNKNOWN\ncommand terminated with exit code 1\n")}, ""},
		{"dns", commandResult{code: 1, stderr: []byte("DNS\ncommand terminated with exit code 1\n")}, ""},
		{"other", commandResult{code: 1, stderr: []byte("OTHER\ncommand terminated with exit code 1\n")}, ""},
		{"stdout", commandResult{code: 1, stdout: []byte("warning"), stderr: []byte("TIMEOUT\ncommand terminated with exit code 1\n")}, ""},
		{"extra stderr", commandResult{code: 1, stderr: []byte("warning\nTIMEOUT\ncommand terminated with exit code 1\n")}, ""},
		{"wrong exit", commandResult{code: 2, stderr: []byte("TIMEOUT\ncommand terminated with exit code 1\n")}, ""},
		{"success warning", commandResult{stderr: []byte("warning")}, ""},
		{"oversized diagnostic", commandResult{stderr: []byte(strings.Repeat("x", outputLimit))}, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseProbe(test.result)
			if got != test.want || (err == nil) != (test.want != "") {
				t.Fatalf("got %q, %v; want %q", got, err, test.want)
			}
			if err != nil && len(err.Error()) > 1200 {
				t.Fatal("unbounded diagnostic")
			}
		})
	}
}

func TestConvergenceRequiresConsecutiveRounds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		results := []bool{true, true, false, true, true, true}
		calls := 0
		start := time.Now()
		err := converge(t.Context(), convergenceLimit, consecutiveRounds, func(context.Context) (bool, error) {
			got := results[calls]
			calls++
			return got, nil
		})
		if err != nil || calls != 6 || time.Since(start) != 25*time.Second {
			t.Fatalf("convergence: calls %d, elapsed %s, error %v", calls, time.Since(start), err)
		}
	})
}

func TestConvergenceWaitsAfterSlowRound(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var previous time.Time
		calls := 0
		err := converge(t.Context(), convergenceLimit, consecutiveRounds, func(context.Context) (bool, error) {
			if !previous.IsZero() && time.Since(previous) < pollInterval {
				t.Fatal("consecutive rounds were not separated by the polling interval")
			}
			if calls == 0 {
				time.Sleep(9 * time.Second)
			}
			previous = time.Now()
			calls++
			return true, nil
		})
		if err != nil || calls != consecutiveRounds {
			t.Fatalf("slow-round convergence: calls %d, error %v", calls, err)
		}
	})
}

func TestConvergenceDeadlineAndCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		err := converge(t.Context(), convergenceLimit, 1, func(context.Context) (bool, error) { return false, nil })
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) != convergenceLimit {
			t.Fatalf("deadline: elapsed %s, error %v", time.Since(start), err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		err = converge(ctx, convergenceLimit, 1, func(context.Context) (bool, error) {
			cancel()
			return true, nil
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("accepted success after cancellation: %v", err)
		}
	})
}
