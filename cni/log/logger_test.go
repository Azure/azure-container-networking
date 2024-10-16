package log

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var errInternal = errors.New("internal error")

func TestLoggerError(t *testing.T) {
	require := require.New(t) //nolint:gocritic

	var buf bytes.Buffer

	// Create a zap core that writes logs to the buffer
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(zapcore.EncoderConfig{}),
		zapcore.AddSync(&buf),
		zapcore.DebugLevel,
	)
	logger := zap.New(core)

	wrappedError := errors.Wrap(errInternal, "wrapped message")
	errorNoStack := NewErrorWithoutStackTrace(wrappedError)

	logger.Info("Error", zap.Error(wrappedError))
	require.Contains(buf.String(), "errorVerbose")
	fmt.Println(buf.String())
	buf.Reset()

	// Error verbose field should be omitted from the error without stack trace error
	logger.Info("ErrorWithoutStackTrace", zap.Error(errorNoStack))
	require.NotContains(buf.String(), "errorVerbose")
	require.Contains(buf.String(), "wrapped message")
	require.Contains(buf.String(), "internal error")
	fmt.Println(buf.String())
	buf.Reset()

	// Even if the embedded error is nil, the error should still display an empty string and not panic
	logger.Info("ErrorWithoutStackTrace nil internal error", zap.Error(NewErrorWithoutStackTrace(nil)))
	require.Contains(buf.String(), "\"error\":\"\"")
	fmt.Println(buf.String())
	buf.Reset()

	// should be able to unwrap the error without a stack trace
	require.ErrorIs(errorNoStack, errInternal)
	// Even if the embedded error is nil, should function properly
	require.NotErrorIs(&ErrorWithoutStackTrace{error: nil}, errorNoStack)
}
