package refresh

import "time"

// MockTickProvider is a mock implementation of the TickProvider interface
type MockTickProvider struct {
	tickChan        chan time.Time
	currentDuration time.Duration
}

// NewMockTickProvider creates a new MockTickProvider
func NewMockTickProvider() *MockTickProvider {
	return &MockTickProvider{
		tickChan: make(chan time.Time, 1),
	}
}

// C returns the channel on which ticks are delivered
func (m *MockTickProvider) C() <-chan time.Time {
	return m.tickChan
}

// Stop stops the ticker
func (m *MockTickProvider) Stop() {
	close(m.tickChan)
}

// Tick manually sends a tick to the channel
func (m *MockTickProvider) Tick() {
	m.tickChan <- time.Now()
}

func (m *MockTickProvider) Reset(d time.Duration) {
	m.currentDuration = d
}

func (m *MockTickProvider) GetCurrentDuration() time.Duration {
	return m.currentDuration
}

var _ TickProvider = &MockTickProvider{}
