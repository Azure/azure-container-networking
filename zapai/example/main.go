package main

import (
	"fmt"
	"github.com/Azure/azure-container-networking/zapai"
	logfmt "github.com/jsternberg/zap-logfmt"
	"github.com/microsoft/ApplicationInsights-Go/appinsights"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"net/http"
	"os"
	"runtime"
	"time"
)

const version = "1.2.3"

type Example struct {
	NetworkContainerID string
	NetworkID          string
	ReservationID      string
	Sub                Sub
}

type Sub struct {
	subnet string
	num    int
}

func (s Sub) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	encoder.AddString("subnet", s.subnet)
	encoder.AddInt("num", s.num)
	return nil
}

func (e Example) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	encoder.AddString("ncId", e.NetworkContainerID)
	encoder.AddString("vnetId", e.NetworkID)
	encoder.AddObject("sub", e.Sub)
	return nil
}

func main() {
	// stdoutcore logs to stdout with a default JSON encoding
	stdoutcore := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), os.Stdout, zapcore.DebugLevel)
	log := zap.New(stdoutcore)
	log.Info("starting...")

	// logfmtcore *also* logs to stdout with a more human-readable logfmt encoding
	logfmtcore := zapcore.NewCore(logfmt.NewEncoder(zap.NewProductionEncoderConfig()), os.Stdout, zapcore.DebugLevel)
	log = zap.New(logfmtcore) // reassign log
	log.Error("subnet failed to join", zap.String("subnet", "podnet"), zap.String("prefix", "10.0.0.0/8"))
	jsoncore := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), os.Stdout, zapcore.DebugLevel)
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
	teecore := zapcore.NewTee(logfmtcore, jsoncore, aicore)

	// reassign log using the teecore
	log = zap.New(teecore, zap.AddCaller())

	//(optional): add normalized fields for the built-in AI Tags
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

	subn := Sub{
		subnet: "123.222.222",
		num:    123,
	}
	ex1 := Example{
		NetworkID:          "vetId-1",
		NetworkContainerID: "nc-1",
		Sub:                subn,
	}

	ex2 := Example{
		NetworkID:          "vetId-2",
		NetworkContainerID: "nc-2",
		Sub:                subn,
	}

	// log with zap.Object
	log.Debug("testing message-1", zap.Object("ex1", &ex1))
	log.Debug("testing message-2", zap.Object("ex2", &ex2))

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
