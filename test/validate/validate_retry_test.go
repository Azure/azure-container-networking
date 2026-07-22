package validate

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunValidationAttempts(t *testing.T) {
	firstErr := errors.New("first observation failed")
	lastErr := errors.New("last observation failed")
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
			errs:          []error{firstErr, nil},
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
			errs:         []error{firstErr, lastErr},
			wantAttempts: 2,
			wantErr:      lastErr,
		},
		{
			name:         "convergence does not hide error",
			results:      []bool{true},
			errs:         []error{lastErr},
			wantAttempts: 1,
			wantErr:      lastErr,
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
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	attempts, converged, err := runValidationAttempts(ctx, 2, time.Hour, func() (bool, error) {
		return false, errors.New("transient observation failure")
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, attempts)
	require.False(t, converged)
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
