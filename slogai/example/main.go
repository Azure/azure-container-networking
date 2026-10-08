// slogai Example Program
//
// This example demonstrates how to use the slogai library to send structured
// logs to Azure Application Insights using Go's standard slog package.
//
// # Prerequisites
//
// You need an Azure Application Insights instance. Create one using Azure CLI:
//
//	# Install Azure CLI (if not already installed)
//	# macOS:   brew install azure-cli
//	# Ubuntu:  curl -sL https://aka.ms/InstallAzureCLIDeb | sudo bash
//	# Windows: winget install Microsoft.AzureCLI
//
//	# Login to Azure
//	az login
//
//	# Create a resource group (if you don't have one)
//	az group create --name myResourceGroup --location eastus
//
//	# Create an Application Insights instance (workspace-based, recommended)
//	# First, create a Log Analytics workspace
//	az monitor log-analytics workspace create \
//	    --resource-group myResourceGroup \
//	    --workspace-name myLogAnalyticsWorkspace
//
//	# Then create Application Insights linked to the workspace
//	az monitor app-insights component create \
//	    --app myAppInsights \
//	    --location eastus \
//	    --resource-group myResourceGroup \
//	    --workspace myLogAnalyticsWorkspace
//
//	# Get the instrumentation key
//	az monitor app-insights component show \
//	    --app myAppInsights \
//	    --resource-group myResourceGroup \
//	    --query instrumentationKey -o tsv
//
// # Running the Example
//
//	export APPLICATION_INSIGHTS_INSTRUMENTATION_KEY="your-instrumentation-key"
//	go run main.go
//
// # Viewing Logs
//
// After running, view your logs in the Azure Portal:
//  1. Go to portal.azure.com
//  2. Navigate to your Application Insights resource
//  3. Select "Logs" under Monitoring
//  4. Query traces: traces | order by timestamp desc | take 50
//
// Or use Azure CLI:
//
//	az monitor app-insights query \
//	    --app myAppInsights \
//	    --resource-group myResourceGroup \
//	    --analytics-query "traces | order by timestamp desc | take 10"
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/Azure/azure-container-networking/slogai"
)

// multiHandler fans out log records to multiple handlers.
type multiHandler struct {
	handlers []slog.Handler
}

func newMultiHandler(handlers ...slog.Handler) *multiHandler {
	return &multiHandler{handlers: handlers}
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m.handlers {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r.Clone()); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{handlers: handlers}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithGroup(name)
	}
	return &multiHandler{handlers: handlers}
}

func main() {
	instrumentationKey := os.Getenv("APPLICATION_INSIGHTS_INSTRUMENTATION_KEY")
	if instrumentationKey == "" {
		panic("APPLICATION_INSIGHTS_INSTRUMENTATION_KEY environment variable is required")
	}

	fmt.Println("=== slogai Library Examples ===")

	// Run all examples
	runSyncExample(instrumentationKey)
	fmt.Println()

	runAsyncExample(instrumentationKey)
	fmt.Println()

	runRedactExample(instrumentationKey)

	fmt.Println("\n=== All examples completed ===")
}

// runSyncExample demonstrates CNI/CNS operations with synchronous logging.
func runSyncExample(instrumentationKey string) {
	fmt.Println("--- Synchronous Sink Example (CNI/CNS Operations) ---")

	cfg, err := slogai.NewSinkConfig(instrumentationKey)
	if err != nil {
		panic(err)
	}
	cfg.MaxBatchInterval = 10 * time.Second
	cfg.MaxBatchSize = 1024

	// Configure error callback to handle telemetry failures
	cfg.OnError = func(err error) {
		fmt.Fprintf(os.Stderr, "Telemetry error: %v\n", err)
	}

	sink := slogai.NewSink(cfg)
	defer sink.Close()

	// Create slogai handler with DEBUG level to see all logs
	aiHandler := slogai.NewHandler(slog.LevelDebug, sink)

	// Use DefaultMappers for convenience (includes all context mappers)
	aiHandler = aiHandler.WithFieldMappers(slogai.DefaultMappers)

	// Create a standard text handler to also log to the terminal
	terminalHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Combine both handlers using a multi-handler
	handler := newMultiHandler(aiHandler, terminalHandler)

	logger := slog.New(handler)
	ctx := context.Background()

	// CNS Service Initialization
	logger.InfoContext(ctx, "[Azure CNS] Starting HTTP REST service",
		slog.String("component", "cns"),
		slog.String("version", "1.5.32"),
		slog.String("node", "aks-nodepool1-12345678-vmss000000"),
	)

	logger.DebugContext(ctx, "[Azure CNS] Registering node with infrastructure network",
		slog.String("infraNetwork", "azure"),
		slog.String("nodeIP", "10.240.0.4"),
	)

	// CNI ADD operation - pod network attachment
	cniLogger := logger.With(
		slog.String("component", "cni"),
		slog.String("containerID", "a1b2c3d4e5f6"),
	)

	cniLogger.InfoContext(ctx, "Processing CNI ADD command",
		slog.String("netNS", "/var/run/netns/cni-12345678-abcd"),
		slog.String("ifName", "eth0"),
		slog.String("podName", "nginx-deployment-5d6d8f7b9c-x2k4m"),
		slog.String("namespace", "default"),
	)

	cniLogger.InfoContext(ctx, "Requesting IP configuration from CNS",
		slog.String("podName", "nginx-deployment-5d6d8f7b9c-x2k4m"),
		slog.String("namespace", "default"),
		slog.String("infraContainerID", "a1b2c3d4e5f6"),
	)

	cniLogger.InfoContext(ctx, "Successfully allocated IP address",
		slog.String("podIP", "10.244.0.15"),
		slog.String("subnet", "10.244.0.0/16"),
		slog.String("gateway", "10.244.0.1"),
		slog.String("macAddress", "00:0d:3a:1b:2c:3d"),
	)

	// Demonstrate grouped attributes for endpoint info
	logger.InfoContext(ctx, "Generated endpoint configuration",
		slog.Group("endpoint",
			slog.String("id", "ep-a1b2c3d4"),
			slog.String("ifName", "eth0"),
			slog.String("ipAddress", "10.244.0.15/16"),
			slog.String("gateway", "10.244.0.1"),
		),
		slog.Group("pod",
			slog.String("name", "nginx-deployment-5d6d8f7b9c-x2k4m"),
			slog.String("namespace", "default"),
			slog.String("uid", "12345678-abcd-efgh-ijkl-mnopqrstuvwx"),
		),
	)

	// Warning scenario - high IP utilization
	logger.WarnContext(ctx, "[Azure CNS] IP pool utilization high",
		slog.Int("allocatedIPs", 245),
		slog.Int("totalIPs", 256),
		slog.Float64("utilizationPercent", 95.7),
		slog.String("subnet", "10.244.0.0/24"),
	)

	// Error scenario - CNS communication failure
	logger.ErrorContext(ctx, "Failed to request IP address from CNS",
		slog.String("podName", "myapp-7f8d9c6b5a-j3k2l"),
		slog.String("namespace", "production"),
		slog.Any("error", fmt.Errorf("connection refused: CNS service unavailable")),
		slog.Int("retryCount", 3),
		slog.Duration("timeout", 30*time.Second),
	)

	// CNI DEL operation
	cniLogger.InfoContext(ctx, "Processing CNI DEL command",
		slog.String("podName", "old-pod-to-delete"),
		slog.String("namespace", "default"),
	)

	cniLogger.InfoContext(ctx, "Successfully released IP address",
		slog.String("releasedIP", "10.244.0.8"),
	)

	fmt.Println("Flushing synchronous sink...")
	time.Sleep(1 * time.Second)
}

// runAsyncExample demonstrates NPM (Network Policy Manager) high-throughput logging.
func runAsyncExample(instrumentationKey string) {
	fmt.Println("--- Asynchronous Sink Example (NPM Policy Operations) ---")

	cfg, err := slogai.NewSinkConfig(instrumentationKey)
	if err != nil {
		panic(err)
	}

	sink := slogai.NewSink(cfg)

	// Configure async sink with custom settings
	asyncCfg := &slogai.AsyncSinkConfig{
		BufferSize:   500,
		DropPolicy:   slogai.DropOldest, // Drop oldest logs when buffer is full
		DrainTimeout: 10 * time.Second,
		OnDropped: func(count int64) {
			fmt.Fprintf(os.Stderr, "Warning: %d logs dropped due to buffer overflow\n", count)
		},
	}

	asyncSink := slogai.NewAsyncSink(sink, asyncCfg)
	defer asyncSink.Close()

	// Create slogai handler
	aiHandler := slogai.NewHandler(slog.LevelInfo, asyncSink)
	aiHandler = aiHandler.WithFieldMappers(slogai.DefaultMappers)

	// Create a standard text handler to also log to the terminal
	terminalHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	// Combine both handlers using a multi-handler
	handler := newMultiHandler(aiHandler, terminalHandler)

	logger := slog.New(handler)
	ctx := context.Background()

	// NPM initialization
	npmLogger := logger.With(slog.String("component", "npm"))

	npmLogger.InfoContext(ctx, "[DataPlane] Initializing network policy manager",
		slog.String("mode", "v2"),
		slog.String("platform", "linux"),
	)

	// Simulate high-throughput pod update events (common during deployments)
	namespaces := []string{"default", "kube-system", "monitoring", "production"}
	apps := []string{"nginx", "redis", "api-server", "worker", "frontend"}

	fmt.Println("Simulating 100 pod network policy events...")
	for i := range 100 {
		ns := namespaces[i%len(namespaces)]
		app := apps[i%len(apps)]
		podName := fmt.Sprintf("%s-%d-pod-%d", app, i/10, i%10)
		podKey := fmt.Sprintf("%s/%s", ns, podName)
		podIP := fmt.Sprintf("10.244.%d.%d", i/256, i%256)

		npmLogger.InfoContext(ctx, "[DataPlane] Updating pod network policies",
			slog.String("podKey", podKey),
			slog.String("podIP", podIP),
			slog.String("endpointID", fmt.Sprintf("ep-%08x", i)),
		)

		// Simulate policy application
		if i%10 == 0 {
			npmLogger.InfoContext(ctx, "[DataPlane] Applying network policy to pod",
				slog.String("podKey", podKey),
				slog.String("policy", fmt.Sprintf("%s-network-policy", ns)),
				slog.String("action", "allow-ingress"),
				slog.Int("rulesApplied", 3+i%5),
			)
		}

		// Simulate IPSet updates
		if i%5 == 0 {
			npmLogger.InfoContext(ctx, "Adding pod to IPSet",
				slog.String("podKey", podKey),
				slog.String("podIP", podIP),
				slog.String("ipset", fmt.Sprintf("azure-npm-%s", ns)),
			)
		}
	}

	// Monitor buffer status
	fmt.Printf("Buffer length: %d, Dropped count: %d\n", asyncSink.BufferLen(), asyncSink.DroppedCount())

	fmt.Println("Flushing async sink...")
	time.Sleep(2 * time.Second)
	fmt.Printf("Final dropped count: %d\n", asyncSink.DroppedCount())
}

// runRedactExample demonstrates sensitive field redaction for network credentials.
func runRedactExample(instrumentationKey string) {
	fmt.Println("--- Field Redaction Example (Network Credentials) ---")

	cfg, err := slogai.NewSinkConfig(instrumentationKey)
	if err != nil {
		panic(err)
	}

	sink := slogai.NewSink(cfg)
	defer sink.Close()

	// Create slogai handler with redaction
	aiHandler := slogai.NewHandler(slog.LevelInfo, sink)
	aiHandler = aiHandler.WithFieldMappers(slogai.DefaultMappers)

	// Configure sensitive fields to be redacted (common in ACN context)
	aiHandler = aiHandler.WithRedactFields(
		"aadToken",
		"bearerToken",
		"connectionString",
		"sasToken",
		"clientSecret",
		"serviceAccountToken",
		"kubeconfig",
	)

	// Create a standard text handler to also log to the terminal
	// Note: Terminal output will show original values (no redaction)
	terminalHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	// Combine both handlers using a multi-handler
	handler := newMultiHandler(aiHandler, terminalHandler)

	logger := slog.New(handler)
	ctx := context.Background()

	// NMAgent authentication - AAD token would be redacted
	logger.InfoContext(ctx, "[Azure CNS] Authenticating with NMAgent",
		slog.String("endpoint", "http://168.63.129.16/machine/plugins"),
		slog.String("aadToken", "eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiIsIng1dCI6..."), // Will be redacted
		slog.String("nodeIP", "10.240.0.4"),
	)

	// Kubernetes API authentication
	logger.InfoContext(ctx, "Connecting to Kubernetes API server",
		slog.String("apiServer", "https://myaks-dns-12345678.hcp.eastus.azmk8s.io:443"),
		slog.String("bearerToken", "eyJhbGciOiJSUzI1NiIsImtpZCI6IkNnYz..."), // Will be redacted
		slog.String("namespace", "kube-system"),
	)

	// Azure Storage connection for state management
	logger.InfoContext(ctx, "[Azure CNS] Initializing state store",
		slog.String("storageAccount", "acnstatestorage"),
		slog.String("container", "cns-state"),
		slog.String("connectionString", "DefaultEndpointsProtocol=https;AccountName=acnstatestorage;AccountKey=abc123..."), // Will be redacted
	)

	// Service principal for IPAM
	logger.InfoContext(ctx, "Configuring Azure IPAM client",
		slog.String("subscriptionID", "12345678-abcd-efgh-ijkl-mnopqrstuvwx"),
		slog.String("resourceGroup", "aks-nodepool-rg"),
		slog.String("clientID", "spn-12345678"),
		slog.String("clientSecret", "super-secret-spn-password-here"), // Will be redacted
	)

	// Pod with mounted service account token
	logger.InfoContext(ctx, "[DataPlane] Pod accessing Kubernetes API",
		slog.String("podKey", "kube-system/azure-cns-12345"),
		slog.String("serviceAccountToken", "eyJhbGciOiJSUzI1NiIsImtpZCI6..."), // Will be redacted
		slog.String("serviceAccount", "azure-cns"),
	)

	fmt.Println("Flushing redaction example...")
	fmt.Println("(Sensitive fields appear as [REDACTED] in Application Insights)")
	time.Sleep(1 * time.Second)
}
