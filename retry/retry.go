package retry

import (
	"context"
	"errors"
	"math"
	"time"

	pkgerrors "github.com/pkg/errors"
)

const (
	noDelay = 0 * time.Nanosecond
)

const (
	ErrMaxAttempts = Error("maximum attempts reached")
)

// Error represents an internal sentinal error which can be defined as a
// constant.
type Error string

func (e Error) Error() string {
	return string(e)
}

// RetriableError is an implementation of TemporaryError that is always retriable
type RetriableError struct {
	err error
}

func (r RetriableError) Error() string {
	if r.err == nil {
		return ""
	}
	return r.err.Error()
}

func (r RetriableError) Unwrap() error {
	return r.err
}

func (r RetriableError) Temporary() bool {
	return true
}

// Forces the error to be retriable, returns nil if error is nil
func WrapTemporaryError(err error) error {
	if err == nil {
		return nil
	}
	return RetriableError{err: err}
}

// TemporaryError is an error that can indicate whether it may be resolved with
// another attempt.
type TemporaryError interface {
	error
	Temporary() bool
}

// Retrier is a construct for attempting some operation multiple times with a
// configurable backoff strategy. To retry, a returned error must implement the
// TemporaryError interface and return true
type Retrier struct {
	Cooldown CooldownFactory
}

// Do repeatedly invokes the provided run function while the context remains
// active. It waits in between invocations of the provided functions by
// delegating to the provided Cooldown function.
func (r Retrier) Do(ctx context.Context, run func() error) error {
	cooldown := r.Cooldown()

	for {
		if err := ctx.Err(); err != nil {
			// nolint:wrapcheck // no meaningful information can be added to this error
			return err
		}

		err := run()
		if err != nil {
			// check to see if it's temporary.
			var tempErr TemporaryError
			if ok := errors.As(err, &tempErr); ok && tempErr.Temporary() {
				delay, cooldownErr := cooldown()
				if cooldownErr != nil {
					return pkgerrors.Wrap(cooldownErr, "sleeping during retry, last error:"+err.Error())
				}
				time.Sleep(delay)
				continue
			}

			// since it's not temporary, it can't be retried, so...
			return err
		}
		return nil
	}
}

// CooldownFunc is a function that will block when called. It is intended for
// use with retry logic.
type CooldownFunc func() (time.Duration, error)

// CooldownFactory is a function that returns CooldownFuncs. It helps
// CooldownFuncs dispose of any accumulated state so that they function
// correctly upon successive uses.
type CooldownFactory func() CooldownFunc

// Max provides a fixed limit for the number of times a subordinate cooldown
// function can be invoked. Note if we set the limit to 0, we still
// invoke the target method once in the retrier, but do not retry
// Read this as the max number of *retries*
func Max(limit int, factory CooldownFactory) CooldownFactory {
	return func() CooldownFunc {
		cooldown := factory()
		count := 0
		return func() (time.Duration, error) {
			if count >= limit {
				return noDelay, ErrMaxAttempts
			}

			delay, err := cooldown()
			if err != nil {
				return noDelay, err
			}
			count++
			return delay, nil
		}
	}
}

// AsFastAsPossible is a Cooldown strategy that does not block, allowing retry
// logic to proceed as fast as possible. This is particularly useful in tests.
func AsFastAsPossible() CooldownFactory {
	return func() CooldownFunc {
		return func() (time.Duration, error) {
			return noDelay, nil
		}
	}
}

// Exponential provides an exponential increase the the base interval provided.
func Exponential(interval time.Duration, base int) CooldownFactory {
	return func() CooldownFunc {
		count := 0
		return func() (time.Duration, error) {
			increment := math.Pow(float64(base), float64(count))
			delay := interval.Nanoseconds() * int64(increment)
			count++
			return time.Duration(delay), nil
		}
	}
}

// Fixed produced the same delay value upon each invocation.
func Fixed(delay time.Duration) CooldownFactory {
	return func() CooldownFunc {
		return func() (time.Duration, error) {
			return delay, nil
		}
	}
}
