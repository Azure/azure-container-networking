package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/ipam/ciliumpool"
	"github.com/Azure/azure-container-networking/ipam/ciliumpool/persistence"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
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
	state, err := persistence.NewLocalState(cfg, logger)
	if err != nil {
		return fmt.Errorf("initializing pool state: %w", err)
	}
	address := os.Getenv("AZURE_VNET_POOL_HEALTH_ADDRESS")
	if address == "" {
		address = "127.0.0.1:10093"
	}
	listenConfig := net.ListenConfig{}
	listener, err := listenConfig.Listen(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("binding pool health listener: %w", err)
	}
	defer listener.Close()
	monitor := ciliumpool.NewMonitor(cfg.PreAllocate)
	state.Observe = func(operation string, duration time.Duration, err error) {
		monitor.Observe("database_"+operation, duration, err)
	}
	bridge := ciliumpool.Bridge{Config: cfg, Client: client, Discover: ciliumpool.DiscoverGuest, Logger: logger, State: state, Monitor: monitor}
	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error { return monitor.Serve(ctx, listener) })
	group.Go(func() error {
		return monitor.RunDiagnostics(ctx, cfg, func() (time.Time, error) { return ciliumpool.CredentialExpiry(api) }, logger)
	})
	group.Go(func() error { return bridge.Run(ctx) })
	if err := group.Wait(); err != nil {
		return fmt.Errorf("running pool bridge: %w", err)
	}
	return nil
}
