package validate

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var (
	errFirstObservation     = errors.New("first observation failed")
	errLastObservation      = errors.New("last observation failed")
	errTransientObservation = errors.New("transient observation failure")
)

func TestRunValidationAttempts(t *testing.T) {
	tests := []struct {
		name          string
		results       []bool
		errs          []error
		wantAttempts  int
		wantConverged bool
		wantErr       error
	}{
		{
			name:          "first observation converges",
			results:       []bool{true},
			errs:          []error{nil},
			wantAttempts:  1,
			wantConverged: true,
		},
		{
			name:          "transient observation failure",
			results:       []bool{false, true},
			errs:          []error{errFirstObservation, nil},
			wantAttempts:  2,
			wantConverged: true,
		},
		{
			name:          "transient mismatch",
			results:       []bool{false, true},
			errs:          []error{nil, nil},
			wantAttempts:  2,
			wantConverged: true,
		},
		{
			name:         "returns latest exhausted error",
			results:      []bool{false, false},
			errs:         []error{errFirstObservation, errLastObservation},
			wantAttempts: 2,
			wantErr:      errLastObservation,
		},
		{
			name:         "convergence does not hide error",
			results:      []bool{true},
			errs:         []error{errLastObservation},
			wantAttempts: 1,
			wantErr:      errLastObservation,
		},
		{
			name:         "exhausted mismatch",
			results:      []bool{false, false},
			errs:         []error{nil, nil},
			wantAttempts: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			call := 0
			attempts, converged, err := runValidationAttempts(t.Context(), len(test.results), 0, func() (bool, error) {
				result := test.results[call]
				err := test.errs[call]
				call++
				return result, err
			})
			require.ErrorIs(t, err, test.wantErr)
			require.Equal(t, test.wantAttempts, attempts)
			require.Equal(t, test.wantConverged, converged)
			require.Equal(t, test.wantAttempts, call)
		})
	}
}

func TestRunValidationAttemptsCanceled(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
	}{
		{
			name:     "during delayed retry",
			interval: time.Hour,
		},
		{
			name: "without retry delay",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			attempts, converged, err := runValidationAttempts(ctx, 2, test.interval, func() (bool, error) {
				return false, errTransientObservation
			})
			require.ErrorIs(t, err, context.Canceled)
			require.Equal(t, 1, attempts)
			require.False(t, converged)
		})
	}
}

func TestRunValidationAttemptsRejectsInvalidAttemptCount(t *testing.T) {
	attempts, converged, err := runValidationAttempts(t.Context(), 0, 0, func() (bool, error) {
		t.Fatal("validation callback must not run")
		return false, nil
	})
	require.EqualError(t, err, "validation attempts must be positive")
	require.Zero(t, attempts)
	require.False(t, converged)
}
