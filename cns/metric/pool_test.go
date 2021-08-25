package metric

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

var _ prometheus.Observer = (*mockObserver)(nil)

type mockObserver struct {
	called   int
	labels   []string
	observed float64
	prometheus.Observer
}

func (m *mockObserver) Observe(o float64) {
	m.observed = o
	m.called++
}

var _ prometheus.ObserverVec = (*mockObserverVec)(nil)

type mockObserverVec struct {
	observer *mockObserver
	prometheus.ObserverVec
}

func (m *mockObserverVec) WithLabelValues(vals ...string) prometheus.Observer {
	m.observer.labels = vals
	return m.observer
}

func newMockObserverVec() *mockObserverVec {
	return &mockObserverVec{
		observer: &mockObserver{},
	}
}

func TestObservePoolScaleLatency(t *testing.T) {
	mockAllocLatency := newMockObserverVec()
	mockDeallocLatency := newMockObserverVec()
	incLatency = mockAllocLatency
	decLatency = mockDeallocLatency

	start := time.Now()
	StartPoolIncreaseTimer(1)
	StartPoolDecreaseTimer(1)
	ObserverPoolScaleLatency()
	elapsed := time.Since(start)

	assert.ElementsMatch(t, []string{"1"}, mockAllocLatency.observer.labels)
	assert.ElementsMatch(t, []string{"1"}, mockDeallocLatency.observer.labels)
	assert.GreaterOrEqual(t, elapsed.Seconds(), mockAllocLatency.observer.observed)
	assert.GreaterOrEqual(t, elapsed.Seconds(), mockDeallocLatency.observer.observed)

	StartPoolIncreaseTimer(2)
	StartPoolDecreaseTimer(2)
	start = time.Now()
	elapsed = time.Since(start)
	ObserverPoolScaleLatency()

	assert.ElementsMatch(t, []string{"2"}, mockAllocLatency.observer.labels)
	assert.ElementsMatch(t, []string{"2"}, mockDeallocLatency.observer.labels)
	assert.LessOrEqual(t, elapsed.Seconds(), mockAllocLatency.observer.observed)
	assert.LessOrEqual(t, elapsed.Seconds(), mockDeallocLatency.observer.observed)
}

func TestNonBlocking(t *testing.T) {
	mockAllocLatency := newMockObserverVec()
	mockDeallocLatency := newMockObserverVec()
	incLatency = mockAllocLatency
	decLatency = mockDeallocLatency
	StartPoolIncreaseTimer(1)
	StartPoolDecreaseTimer(1)
	StartPoolIncreaseTimer(2)
	StartPoolDecreaseTimer(2)
	ObserverPoolScaleLatency()
	ObserverPoolScaleLatency()
	assert.Equal(t, 1, mockAllocLatency.observer.called)
	assert.Equal(t, 1, mockDeallocLatency.observer.called)
	assert.ElementsMatch(t, []string{"1"}, mockAllocLatency.observer.labels)
	assert.ElementsMatch(t, []string{"1"}, mockDeallocLatency.observer.labels)
}
