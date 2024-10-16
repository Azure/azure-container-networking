package refresh

import (
	"context"
	"time"
)

const (
	DefaultMinInterval = 4 * time.Second
	DefaultMaxInterval = 1024 * time.Second
)

// Fetcher fetches data at regular intervals. The interval will vary within the range of minInterval and
// maxInterval. When no diff is observed after a fetch, the interval doubles (subject to the maximum interval).
// When a diff is observed, the interval resets to the minimum. The interval can be made unchanging by setting
// minInterval and maxInterval to the same desired value.

type Fetcher[T equaler[T]] struct {
	fetchFunc       func(context.Context) (T, error)
	cache           T
	minInterval     time.Duration
	maxInterval     time.Duration
	currentInterval time.Duration
	ticker          TickProvider
	consumeFunc     func(T) error
	logger          Logger
}

// NewFetcher creates a new Fetcher. If minInterval is 0, it will default to 4 seconds.
func NewFetcher[T equaler[T]](
	fetchFunc func(context.Context) (T, error),
	minInterval time.Duration,
	maxInterval time.Duration,
	consumeFunc func(T) error,
	logger Logger,
) *Fetcher[T] {
	if minInterval == 0 {
		minInterval = DefaultMinInterval
	}

	if maxInterval == 0 {
		maxInterval = DefaultMaxInterval
	}

	maxInterval = max(minInterval, maxInterval)

	return &Fetcher[T]{
		fetchFunc:       fetchFunc,
		minInterval:     minInterval,
		maxInterval:     maxInterval,
		currentInterval: minInterval,
		consumeFunc:     consumeFunc,
		logger:          logger,
	}
}

func (f *Fetcher[T]) Start(ctx context.Context) {
	go func() {
		// do an initial fetch
		res, err := f.fetchFunc(ctx)
		if err != nil {
			f.logger.Printf("Error invoking fetch: %v", err)
		}

		f.cache = res
		if f.consumeFunc != nil {
			if err := f.consumeFunc(res); err != nil {
				f.logger.Errorf("Error consuming data: %v", err)
			}
		}

		if f.ticker == nil {
			f.ticker = NewTimedTickProvider(f.currentInterval)
		}

		defer f.ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				f.logger.Printf("Fetcher stopped")
				return
			case <-f.ticker.C():
				result, err := f.fetchFunc(ctx)
				if err != nil {
					f.logger.Errorf("Error fetching data: %v", err)
				} else {
					if result.Equal(f.cache) {
						f.updateFetchIntervalForNoObservedDiff()
						f.logger.Printf("No diff observed in fetch, not invoking the consumer")
					} else {
						f.cache = result
						f.updateFetchIntervalForObservedDiff()
						if f.consumeFunc != nil {
							if err := f.consumeFunc(result); err != nil {
								f.logger.Errorf("Error consuming data: %v", err)
							}
						}
					}
				}

				f.ticker.Reset(f.currentInterval)
			}
		}
	}()
}

func (f *Fetcher[T]) updateFetchIntervalForNoObservedDiff() {
	f.currentInterval = min(f.currentInterval*2, f.maxInterval) // nolint:gomnd // doubling logic
}

func (f *Fetcher[T]) updateFetchIntervalForObservedDiff() {
	f.currentInterval = f.minInterval
}
