package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	zaplogfmt "github.com/jsternberg/zap-logfmt"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	ctx       context.Context
	z         *zap.Logger
	levelFlag string
	leveler   = zap.NewAtomicLevel()
)

// root represent the base invocation.
var root = &cobra.Command{
	Use:          "dropgz",
	SilenceUsage: true,
}

func init() {
	// set up signal handlers
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(context.Background())

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		cancel()
		fmt.Println("exiting")
		os.Exit(1)
	}()

	// build root logger
	zcfg := zap.NewProductionEncoderConfig()
	z = zap.New(zapcore.NewCore(
		zaplogfmt.NewEncoder(zcfg),
		os.Stdout,
		leveler,
	))

	// bind root flags
	root.PersistentFlags().StringVarP(&levelFlag, "log-level", "v", "info", "log level [trace,debug,info,warn,error]")
}

func Execute() {
	if err := root.ExecuteContext(ctx); err != nil {
		z.Fatal("exiting due to error", zap.Error(err))
	}
}

func setLogLevel() error {
	level, err := zapcore.ParseLevel(levelFlag)
	if err != nil {
		return errors.Wrapf(err, "failed to parse log level '%s'", levelFlag)
	}
	leveler.SetLevel(level)
	return nil
}
