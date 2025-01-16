package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	pkgerrors "github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

var errTest = errors.New("mock error")

type TestError struct{}

func (t TestError) Error() string {
	return "oh no!"
}

func (t TestError) Temporary() bool {
	return true
}

func TestBackoffRetry(t *testing.T) {
	got := 0
	exp := 10

	ctx := context.Background()

	rt := Retrier{
		Cooldown: AsFastAsPossible(),
	}

	err := rt.Do(ctx, func() error {
		if got < exp {
			got++
			return TestError{}
		}
		return nil
	})
	if err != nil {
		t.Fatal("unexpected error: err:", err)
	}

	if got < exp {
		t.Error("unexpected number of invocations: got:", got, "exp:", exp)
	}
}

func TestBackoffRetryWithCancel(t *testing.T) {
	got := 0
	exp := 5
	total := 10

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt := Retrier{
		Cooldown: AsFastAsPossible(),
	}

	err := rt.Do(ctx, func() error {
		got++
		if got >= exp {
			cancel()
		}

		if got < total {
			return TestError{}
		}
		return nil
	})

	if err == nil {
		t.Error("expected context cancellation error, but received none")
	}

	if got != exp {
		t.Error("unexpected number of iterations: exp:", exp, "got:", got)
	}
}

func TestBackoffRetryUnretriableError(t *testing.T) {
	rt := Retrier{
		Cooldown: AsFastAsPossible(),
	}

	err := rt.Do(context.Background(), func() error {
		return errors.New("boom") // nolint:goerr113 // it's just a test
	})

	if err == nil {
		t.Fatal("expected an error, but none was returned")
	}
}

func TestFixed(t *testing.T) {
	exp := 20 * time.Millisecond

	cooldown := Fixed(exp)()

	got, err := cooldown()
	if err != nil {
		t.Fatal("unexpected error invoking cooldown: err:", err)
	}

	if got != exp {
		t.Fatal("unexpected sleep duration: exp:", exp, "got:", got)
	}
}

func TestExp(t *testing.T) {
	exp := 10 * time.Millisecond
	base := 2

	cooldown := Exponential(exp, base)()

	first, err := cooldown()
	if err != nil {
		t.Fatal("unexpected error invoking cooldown: err:", err)
	}

	if first != exp {
		t.Fatal("unexpected sleep during first cooldown: exp:", exp, "got:", first)
	}

	// ensure that the sleep increases
	second, err := cooldown()
	if err != nil {
		t.Fatal("unexpected error on second invocation of cooldown: err:", err)
	}

	if second < first {
		t.Fatal("unexpected sleep during first cooldown: exp:", exp, "got:", second)
	}
}

func TestMax(t *testing.T) {
	exp := 10
	got := 0

	// create a test sleep function
	fn := func() CooldownFunc {
		return func() (time.Duration, error) {
			got++
			return 0 * time.Nanosecond, nil
		}
	}

	cooldown := Max(10, fn)()

	for i := 0; i < exp; i++ {
		_, err := cooldown()
		if err != nil {
			t.Fatal("unexpected error from cooldown: err:", err)
		}
	}

	if exp != got {
		t.Error("unexpected number of cooldown invocations: exp:", exp, "got:", got)
	}

	// attempt one more, we expect an error
	_, err := cooldown()
	if err == nil {
		t.Errorf("expected an error after %d invocations but received none", exp+1)
	}
}

func TestRetriableError(t *testing.T) {
	// wrapping nil returns a nil
	require.NoError(t, WrapTemporaryError(nil))

	wrappedMockError := WrapTemporaryError(pkgerrors.Wrap(errTest, "nested"))

	// temporary errors should still be able to be unwrapped
	require.ErrorIs(t, wrappedMockError, errTest)

	var temporaryError TemporaryError
	require.ErrorAs(t, wrappedMockError, &temporaryError)
	require.True(t, temporaryError.Temporary(), "errors returned from wrap temporary error should have temporary set to true")
}

func createFunctionWithFailurePattern(errorPattern []error) func() error {
	s := 0
	return func() error {
		if s >= len(errorPattern) {
			return nil
		}
		result := errorPattern[s]
		s++
		return result
	}
}

func TestRunWithRetries(t *testing.T) {
	errMock := WrapTemporaryError(errTest)
	retries := 3 // runs 4 times, then errors before the 5th
	retrier := Retrier{
		Cooldown: Max(retries, Fixed(100*time.Millisecond)),
	}

	tests := []struct {
		name    string
		wantErr bool
		f       func() error
	}{
		{
			name:    "Succeed on first try",
			f:       createFunctionWithFailurePattern([]error{}),
			wantErr: false,
		},
		{
			name:    "Succeed on first try do not check again",
			f:       createFunctionWithFailurePattern([]error{nil, errMock, errMock, errMock}),
			wantErr: false,
		},
		{
			name:    "Succeed on last try",
			f:       createFunctionWithFailurePattern([]error{errMock, errMock, errMock, nil, errMock}),
			wantErr: false,
		},
		{
			name:    "Fail after too many attempts",
			f:       createFunctionWithFailurePattern([]error{errMock, errMock, errMock, errMock, nil, nil}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			err := retrier.Do(context.Background(), tt.f)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
