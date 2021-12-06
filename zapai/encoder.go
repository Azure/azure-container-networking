package zapai

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"sync"

	"github.com/microsoft/ApplicationInsights-Go/appinsights"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

type traceEncoder interface {
	encode(*appinsights.TraceTelemetry) ([]byte, error)
}

type traceDecoder interface {
	decode([]byte) (*appinsights.TraceTelemetry, error)
}

// gobber is a synchronized encoder/decoder for appinsights.TraceTelemetry <-> []byte.
//
// A thread-safe object is necessary because, for efficiency, we reuse the gob.Enc/Decoder objects, and they must be
// attached to a common buffer (per gobber) to stream data in and out.
//
// This impl lets consumers deal with the gobber.enc/decode methods synchronously without having to synchronize a
// pipe or buffer and the gob.Enc/Decoders directly.
//
// Encoders and Decoders also need to be matched up 1:1, as the first thing an Encoder sends (once!) is type data, and
// it is an error for a Decoder to receive the same type data from its stream more than once.
type gobber struct {
	encoder *gob.Encoder
	decoder *gob.Decoder
	buffer  *bytes.Buffer
	sync.Mutex
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
	if err := g.encoder.Encode(t); err != nil {
		return nil, errors.Wrap(err, "gobber failed to encode trace")
	}
	b := make([]byte, g.buffer.Len())
	if _, err := g.buffer.Read(b); err != nil {
		return nil, errors.Wrap(err, "gobber failed to read from buffer")
	}
	return b, nil
}

// decode turns a []byte gob in to an appinsights.TraceTelemetry.
// decode is safe for concurrent use.
func (g *gobber) decode(b []byte) (*appinsights.TraceTelemetry, error) {
	g.Lock()
	defer g.Unlock()
	if _, err := g.buffer.Write(b); err != nil {
		return nil, errors.Wrap(err, "gobber failed to write to buffer")
	}
	trace := appinsights.TraceTelemetry{}
	if err := g.decoder.Decode(&trace); err != nil {
		return nil, errors.Wrap(err, "gobber failed to decode trace")
	}
	return &trace, nil
}

// fieldStringer evaluates a zapcore.Field in to a best-effort string.
func fieldStringer(f *zapcore.Field) string {
	if f.Type == zapcore.StringType {
		return f.String
	}
	return fmt.Sprintf("%v", f)
}
