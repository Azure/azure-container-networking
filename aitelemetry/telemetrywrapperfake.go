package aitelemetry

func NewFakeTelemtry() FakeTelemetryHandle {
	return FakeTelemetryHandle{}
}

type FakeTelemetryHandle struct{}

func (f FakeTelemetryHandle) TrackLog(report Report) {}

func (f FakeTelemetryHandle) TrackMetric(metric Metric) {}

func (f FakeTelemetryHandle) TrackEvent(aiEvent Event) {}

func (f FakeTelemetryHandle) Close(timeout int) {}
