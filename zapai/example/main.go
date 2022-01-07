package main

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/Azure/azure-container-networking/zapai"
	logfmt "github.com/jsternberg/zap-logfmt"
	"github.com/microsoft/ApplicationInsights-Go/appinsights"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const version = "1.2.3"

func main() {
	// stdoutcore logs to stdout with a default JSON encoding
	stdoutcore := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), os.Stdout, zapcore.DebugLevel)
	log := zap.New(stdoutcore)
	log.Info("starting...")

	// logfmtcore *also* logs to stdout with a more human-readable logfmt encoding
	logfmtcore := zapcore.NewCore(logfmt.NewEncoder(zap.NewProductionEncoderConfig()), os.Stdout, zapcore.DebugLevel)
	log = zap.New(logfmtcore) // reassign log
	log.Error("subnet failed to join", zap.String("subnet", "podnet"), zap.String("prefix", "10.0.0.0/8"))

	// build the AI config
	sinkcfg := zapai.SinkConfig{
		GracePeriod:            30 * time.Second, //nolint:gomnd // ignore
		TelemetryConfiguration: *appinsights.NewTelemetryConfiguration("key123"),
	}
	sinkcfg.MaxBatchSize = 32000
	sinkcfg.MaxBatchInterval = 30 * time.Second //nolint:gomnd // ignore

	// open the AI zap sink
	aisink, aiclose, err := zap.Open(sinkcfg.URI()) //nolint:govet // intentional shadow
	if err != nil {
		fmt.Println(err)
		return
	}
	defer aiclose()

	// build the AI core
	aicore := zapai.NewCore(zapcore.DebugLevel, aisink)
	// (optional): add the zap Field to AI Tag mappers
	aicore = aicore.WithFieldMappers(zapai.DefaultMappers)

	// compose the logfmt and aicore in to a virtual tee core so they both receive all log events
	teecore := zapcore.NewTee(logfmtcore, aicore)

	// reassign log using the teecore
	log = zap.New(teecore)

	// (optional): add normalized fields for the built-in AI Tags
	log = log.With(
		zap.String("user_id", runtime.GOOS),
		zap.String("operation_id", ""),
		zap.String("parent_id", version),
		zap.String("version", version),
		zap.String("account", "SubscriptionID"),
		zap.String("anonymous_user_id", "VMName"),
		zap.String("session_id", "VMID"),
		zap.String("AppName", "name"),
		zap.String("Region", "Location"),
		zap.String("ResourceGroup", "ResourceGroupName"),
		zap.String("VMSize", "VMSize"),
		zap.String("OSVersion", "OSVersion"),
		zap.String("VMID", "VMID"),
	)

	// muxlog adds a component=mux field to every log that it writes
	muxlog := log.With(zap.String("component", "mux"))
	m := &mux{log: muxlog}

	http.Handle("/info", m.loggerHandlerMiddleware(infoHandler))
	if err := http.ListenAndServe(":8080", http.DefaultServeMux); err != nil {
		log.Sugar().Fatal(err)
	}
}

type mux struct {
	log *zap.Logger
}

type loggedHandler func(http.ResponseWriter, *http.Request, *zap.Logger)

func (m *mux) loggerHandlerMiddleware(l loggedHandler) http.HandlerFunc {
	handlerLogger := m.log.With(zap.String("span-id", "guid"))
	return func(w http.ResponseWriter, r *http.Request) {
		l(w, r, handlerLogger)
	}
}

func infoHandler(w http.ResponseWriter, r *http.Request, l *zap.Logger) {
	// do some stuff
	// write some logs
	l.Info("some stuff with metadata like span")
}
