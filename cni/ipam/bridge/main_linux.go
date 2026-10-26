package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/ipam/ciliumpool"
	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var errMissingConfig = fmt.Errorf("bridge requires %s", common.CiliumPoolConfigPath)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	logger = logger.With(zap.String("component", "ipam-pool-bridge"))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err = run(ctx, logger)
	cancel()
	if err != nil {
		logger.Error("bridge stopped", zap.Error(err))
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *zap.Logger) error {
	cfg, err := common.LoadCiliumPoolConfig()
	if err != nil {
		return fmt.Errorf("loading bridge configuration: %w", err)
	}
	if cfg == nil {
		return errMissingConfig
	}
	var api *rest.Config
	if path := os.Getenv("KUBECONFIG"); path != "" {
		api, err = clientcmd.BuildConfigFromFlags("", path)
	} else {
		api, err = rest.InClusterConfig()
	}
	if err != nil {
		return fmt.Errorf("loading Kubernetes client configuration: %w", err)
	}
	api.Timeout = 10 * time.Second
	api.UserAgent = "azure-vnet-ipam-bridge"
	client, err := dynamic.NewForConfig(api)
	if err != nil {
		return fmt.Errorf("creating Kubernetes client: %w", err)
	}
	bridge := ciliumpool.Bridge{Config: cfg, Client: client, Discover: ciliumpool.DiscoverGuest, Logger: logger}
	if err := bridge.Run(ctx); err != nil {
		return fmt.Errorf("running bridge: %w", err)
	}
	return nil
}
