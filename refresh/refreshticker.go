package refresh

import "time"

// TickProvider defines an interface for a type that provides a channel that ticks at a regular interval
type TickProvider interface {
	Stop()
	Reset(d time.Duration)
	C() <-chan time.Time
}

// TimedTickProvider wraps a time.Ticker to implement TickProvider
type TimedTickProvider struct {
	ticker *time.Ticker
}

var _ TickProvider = &TimedTickProvider{}

// NewTimedTickProvider creates a new TimedTickProvider
func NewTimedTickProvider(d time.Duration) *TimedTickProvider {
	return &TimedTickProvider{ticker: time.NewTicker(d)}
}

// Stop stops the ticker
func (tw *TimedTickProvider) Stop() {
	tw.ticker.Stop()
}

// Reset resets the ticker with a new duration
func (tw *TimedTickProvider) Reset(d time.Duration) {
	tw.ticker.Reset(d)
}

// C returns the ticker's channel
func (tw *TimedTickProvider) C() <-chan time.Time {
	return tw.ticker.C
}
