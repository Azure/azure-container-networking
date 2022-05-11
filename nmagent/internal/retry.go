package internal

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

// TemporaryError is an error that can indicate whether it may be resolved with
// another attempt.
type TemporaryError interface {
	error
	Temporary() bool
}

// Retrier is a construct for attempting some operation multiple times with a
// configurable backoff strategy.
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
				delay, err := cooldown() // nolint:govet // the shadow is intentional
				if err != nil {
					return pkgerrors.Wrap(err, "sleeping during retry")
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
// function can be invoked.
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
