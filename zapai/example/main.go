package main

import (
	"net/http"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	stdoutcore := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), os.Stdout, zapcore.DebugLevel)
	log := zap.New(stdoutcore)
	log.Info("starting...")

	log.Info("subnet failed to join", zap.String("subnet", "podnet"), zap.String("prefix", "10.0.0.0/8"))

	// multiple subnets

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
