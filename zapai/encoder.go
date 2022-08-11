package zapai

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/microsoft/ApplicationInsights-Go/appinsights"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

type traceEncoder interface {
	encode(*appinsights.TraceTelemetry) ([]byte, error)
	AddArray(string, zapcore.ArrayMarshaler) error
	AddObject(string, zapcore.ObjectMarshaler) error
	AddBinary(string, []byte)
	AddByteString(string, []byte)
	AddBool(string, bool)
	AddComplex128(string, complex128)
	AddComplex64(string, complex64)
	AddDuration(string, time.Duration)
	AddFloat64(string, float64)
	AddFloat32(string, float32)
	AddInt(string, int)
	AddInt64(string, int64)
	AddInt32(string, int32)
	AddInt16(string, int16)
	AddInt8(string, int8)
	AddString(string, string)
	AddTime(string, time.Time)
	AddUint(string, uint)
	AddUint64(string, uint64)
	AddUint32(string, uint32)
	AddUint16(string, uint16)
	AddUint8(string, uint8)
	AddUintptr(string, uintptr)
	AddReflected(string, interface{}) error
	OpenNamespace(string)
	cloneEncoder(*appinsights.TraceTelemetry) traceEncoder
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
	encoder        *gob.Encoder
	decoder        *gob.Decoder
	buffer         *bytes.Buffer
	traceTelemetry *appinsights.TraceTelemetry
	lock           sync.Mutex
}

var _tracePool = sync.Pool{New: func() interface{} {
	return &gobber{}
}}

func (g *gobber) AddObject(_ string, marshaler zapcore.ObjectMarshaler) error {
	marshaler.MarshalLogObject(g)
	return nil
}

func (g *gobber) AddString(key, value string) {
	g.traceTelemetry.Properties[key] = value
}

func (g *gobber) AddArray(_ string, _ zapcore.ArrayMarshaler) error { return nil }

func (g *gobber) AddBinary(_ string, _ []byte) {}

func (g *gobber) AddByteString(_ string, _ []byte) {}

func (g *gobber) AddBool(_ string, _ bool) {}

func (g *gobber) AddComplex128(_ string, _ complex128) {}

func (g *gobber) AddComplex64(_ string, _ complex64) {}

func (g *gobber) AddDuration(_ string, _ time.Duration) {}

func (g *gobber) AddFloat64(_ string, _ float64) {}

func (g *gobber) AddFloat32(_ string, _ float32) {}

func (g *gobber) AddInt(_ string, _ int) {}

func (g *gobber) AddInt64(_ string, _ int64) {}

func (g *gobber) AddInt32(_ string, _ int32) {}

func (g *gobber) AddInt16(_ string, _ int16) {}

func (g *gobber) AddInt8(_ string, _ int8) {}

func (g *gobber) AddTime(_ string, _ time.Time) {}

func (g *gobber) AddUint(_ string, _ uint) {}

func (g *gobber) AddUint64(_ string, _ uint64) {}

func (g *gobber) AddUint32(_ string, _ uint32) {}

func (g *gobber) AddUint16(_ string, _ uint16) {}

func (g *gobber) AddUint8(_ string, _ uint8) {}

func (g *gobber) AddUintptr(_ string, _ uintptr) {}

func (g *gobber) AddReflected(_ string, _ interface{}) error { return nil }

func (g *gobber) OpenNamespace(_ string) {}

func (g *gobber) cloneEncoder(traceTelemetry *appinsights.TraceTelemetry) traceEncoder {
	clone := _tracePool.Get().(*gobber)
	buf := &bytes.Buffer{}
	clone.encoder = gob.NewEncoder(buf)
	clone.buffer = buf
	clone.lock = g.lock
	clone.traceTelemetry = traceTelemetry
	return clone
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
	g.lock.Lock()
	defer g.lock.Unlock()
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
	g.lock.Lock()
	defer g.lock.Unlock()
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
	switch f.Type {
	case zapcore.StringType:
		return f.String
	case zapcore.Int64Type:
		return strconv.FormatInt(f.Integer, 10)
	case zapcore.Uint16Type:
		return strconv.FormatInt(f.Integer, 10)
	case zapcore.ErrorType:
		return f.Interface.(error).Error()
	case zapcore.BoolType:
		return strconv.FormatBool(f.Integer == 1)
	default:
		return fmt.Sprintf("%v", f)
	}
}
