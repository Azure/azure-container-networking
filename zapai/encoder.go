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
	zapcore.ObjectEncoder
	encode(*appinsights.TraceTelemetry) ([]byte, error)
	setTraceTelemetry(*appinsights.TraceTelemetry)
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
	keyPrefix      string
	sync.Mutex
}

func (g *gobber) AddObject(key string, marshaler zapcore.ObjectMarshaler) error {
	curPrefix := g.keyPrefix
	if len(g.keyPrefix) == 0 {
		g.keyPrefix = key
	} else {
		g.keyPrefix = g.keyPrefix + "_" + key
	}
	marshaler.MarshalLogObject(g)
	g.keyPrefix = curPrefix
	return nil
}

func (g *gobber) AddString(key, value string) {
	g.traceTelemetry.Properties[g.keyPrefix+"_"+key] = value
}

func (g *gobber) AddBool(key string, value bool) {
	g.traceTelemetry.Properties[g.keyPrefix+"_"+key] = strconv.FormatBool(value)
}

func (g *gobber) AddInt(key string, value int) {
	g.traceTelemetry.Properties[g.keyPrefix+"_"+key] = strconv.Itoa(value)
}

func (g *gobber) AddInt64(key string, value int64) {
	g.traceTelemetry.Properties[g.keyPrefix+"_"+key] = strconv.FormatInt(value, 10)
}

func (g *gobber) AddUint16(key string, value uint16) {
	g.traceTelemetry.Properties[g.keyPrefix+"_"+key] = strconv.FormatUint(uint64(value), 10)
}

func (g *gobber) AddArray(_ string, _ zapcore.ArrayMarshaler) error {
	// TODO to be implemented
	return nil
}

func (g *gobber) AddBinary(_ string, _ []byte) {
	// TODO to be implemented
}

func (g *gobber) AddByteString(_ string, _ []byte) {
	// TODO to be implemented
}

func (g *gobber) AddComplex128(_ string, _ complex128) {
	// TODO to be implemented
}

func (g *gobber) AddComplex64(_ string, _ complex64) {
	// TODO to be implemented
}

func (g *gobber) AddDuration(_ string, _ time.Duration) {
	// TODO to be implemented
}

func (g *gobber) AddFloat64(_ string, _ float64) {
	// TODO to be implemented
}

func (g *gobber) AddFloat32(_ string, _ float32) {
	// TODO to be implemented
}

func (g *gobber) AddInt32(_ string, _ int32) {
	// TODO to be implemented
}

func (g *gobber) AddInt16(_ string, _ int16) {
	// TODO to be implemented
}

func (g *gobber) AddInt8(_ string, _ int8) {
	// TODO to be implemented
}

func (g *gobber) AddTime(_ string, _ time.Time) {
	// TODO to be implemented
}

func (g *gobber) AddUint(_ string, _ uint) {
	// TODO to be implemented
}

func (g *gobber) AddUint64(_ string, _ uint64) {
	// TODO to be implemented
}

func (g *gobber) AddUint32(_ string, _ uint32) {
	// TODO to be implemented
}

func (g *gobber) AddUint8(_ string, _ uint8) {
	// TODO to be implemented
}

func (g *gobber) AddUintptr(_ string, _ uintptr) {
	// TODO to be implemented
}

func (g *gobber) AddReflected(_ string, _ interface{}) error {
	// TODO to be implemented
	return nil
}

func (g *gobber) OpenNamespace(_ string) {
	// TODO to be implemented
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
