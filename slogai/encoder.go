package slogai

import (
	"bytes"
	"encoding/gob"
	"sync"

	"github.com/microsoft/ApplicationInsights-Go/appinsights"
	"github.com/pkg/errors"
)

type traceEncoder interface {
	encode(*appinsights.TraceTelemetry) ([]byte, error)
	setTraceTelemetry(*appinsights.TraceTelemetry)
}

type traceDecoder interface {
	decode([]byte) (*appinsights.TraceTelemetry, error)
}

// gobber is a synchronized encoder/decoder for appinsights.TraceTelemetry <-> []byte.
// Similar to the zapai implementation but simplified for slog use case.
type gobber struct {
	encoder        *gob.Encoder
	decoder        *gob.Decoder
	buffer         *bytes.Buffer
	traceTelemetry *appinsights.TraceTelemetry
	sync.Mutex
}

func (g *gobber) setTraceTelemetry(traceTelemetry *appinsights.TraceTelemetry) {
	g.traceTelemetry = traceTelemetry
}

// newTraceEncoder creates a gobber that can only encode.
func newTraceEncoder() traceEncoder {
	buf := &bytes.Buffer{}
	return &gobber{
		encoder: gob.NewEncoder(buf),
		buffer:  buf,
	}
}

// newTraceDecoder creates a gobber that can only decode.
func newTraceDecoder() traceDecoder {
	buf := &bytes.Buffer{}
	return &gobber{
		decoder: gob.NewDecoder(buf),
		buffer:  buf,
	}
}

// encode turns an appinsights.TraceTelemetry into a []byte gob.
// encode is safe for concurrent use.
func (g *gobber) encode(t *appinsights.TraceTelemetry) ([]byte, error) {
	g.Lock()
	defer g.Unlock()

	// Reset buffer
	g.buffer.Reset()

	if err := g.encoder.Encode(t); err != nil {
		return nil, errors.Wrap(err, "gobber failed to encode trace")
	}

	b := make([]byte, g.buffer.Len())
	if _, err := g.buffer.Read(b); err != nil {
		return nil, errors.Wrap(err, "gobber failed to read from buffer")
	}
	return b, nil
}

// decode turns a []byte gob into an appinsights.TraceTelemetry.
// decode is safe for concurrent use.
func (g *gobber) decode(b []byte) (*appinsights.TraceTelemetry, error) {
	g.Lock()
	defer g.Unlock()

	// Reset buffer and write new data
	g.buffer.Reset()
	if _, err := g.buffer.Write(b); err != nil {
		return nil, errors.Wrap(err, "gobber failed to write to buffer")
	}

	trace := appinsights.TraceTelemetry{}
	if err := g.decoder.Decode(&trace); err != nil {
		return nil, errors.Wrap(err, "gobber failed to decode trace")
	}
	return &trace, nil
}
