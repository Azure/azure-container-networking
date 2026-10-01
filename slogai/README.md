# slogai

A Go package that integrates the standard `log/slog` structured logging library with Microsoft Azure Application Insights.

## Features

- Full `slog.Handler` interface implementation
- Automatic mapping of slog levels to Application Insights severity levels
- Field mappers to transform slog attribute names to Application Insights context tags
- Sensitive data redaction (PII filtering)
- Async buffered logging option for high-throughput scenarios
- Thread-safe for concurrent use
- Panic recovery in log handling

## Installation

```bash
go get github.com/Azure/azure-container-networking/slogai
```

## Quick Start

### Basic Usage (Synchronous)

```go
package main

import (
    "context"
    "log/slog"
    "os"

    "github.com/Azure/azure-container-networking/slogai"
)

func main() {
    // Create sink configuration
    cfg, err := slogai.NewSinkConfig(os.Getenv("APPLICATION_INSIGHTS_KEY"))
    if err != nil {
        panic(err)
    }

    // Create the sink
    sink := slogai.NewSink(cfg)
    defer sink.Close()

    // Create the handler
    handler := slogai.NewHandler(slog.LevelInfo, sink)

    // Create logger
    logger := slog.New(handler)

    // Log messages
    logger.Info("Application started", "version", "1.0.0")
}
```

### Async Usage (High Throughput)

For high-throughput scenarios, use `AsyncSink` to avoid blocking on network I/O:

```go
// Create sync sink first
cfg, _ := slogai.NewSinkConfig(instrumentationKey)
sink := slogai.NewSink(cfg)
defer sink.Close()

// Wrap with async sink
asyncCfg := &slogai.AsyncSinkConfig{
    BufferSize:   10000,                    // Buffer up to 10k logs
    DropPolicy:   slogai.DropNewest,        // Drop new logs if buffer full
    DrainTimeout: 5 * time.Second,          // Timeout on shutdown
    OnDropped: func(count int64) {
        fmt.Printf("Dropped %d logs\n", count)
    },
}
asyncSink := slogai.NewAsyncSink(sink, asyncCfg)
defer asyncSink.Close()

// Use with handler
handler := slogai.NewHandler(slog.LevelInfo, asyncSink)
logger := slog.New(handler)
```

## Configuration

### SinkConfig Options

| Option | Description | Default |
|--------|-------------|---------|
| `InstrumentationKey` | Application Insights instrumentation key | Required |
| `EndpointUrl` | Telemetry endpoint URL | Azure public endpoint |
| `MaxBatchSize` | Max telemetry items per batch | 1024 |
| `MaxBatchInterval` | Max time between batches | 10s |
| `GracePeriod` | Shutdown grace period for flushing | 10s |
| `OnError` | Error callback function | nil |

### AsyncSinkConfig Options

| Option | Description | Default |
|--------|-------------|---------|
| `BufferSize` | Async buffer channel size | 1000 |
| `DropPolicy` | Buffer overflow policy | DropNewest |
| `DrainTimeout` | Max drain time on Close | 5s |
| `OnDropped` | Callback when logs are dropped | nil |

### Drop Policies

- `DropNewest` - Drop incoming logs when buffer is full (default)
- `DropOldest` - Drop oldest logs in buffer to make room
- `Block` - Block until space is available (may cause backpressure)

## Field Mappers

Map slog attribute names to Application Insights context tags:

```go
handler := slogai.NewHandler(slog.LevelInfo, sink)
handler = handler.WithFieldMappers(
    slogai.DefaultMappers,           // All default mappers
    // Or specific mappers:
    slogai.UserContextMappers,       // user_id -> ai.user.authUserId
    slogai.OperationContextMappers,  // operation_id -> ai.operation.id
    slogai.CloudContextMappers,      // role -> ai.cloud.role
)

// Now these attributes are mapped to AI context tags
logger.Info("request", "user_id", "user123", "operation_id", "op456")
```

Available mapper sets:
- `ApplicationContextMappers` - version
- `DeviceContextMappers` - device_id, locale, model, os_version, etc.
- `LocationContextMappers` - ip
- `OperationContextMappers` - operation_id, operation_name, parent_id, etc.
- `SessionContextMappers` - session_id, session_is_first
- `UserContextMappers` - user_id, anonymous_user_id, account
- `CloudContextMappers` - role, role_instance
- `InternalContextMappers` - sdk_version, agent_version, node_name
- `DefaultMappers` - All of the above combined

## Sensitive Data Redaction

Redact sensitive fields before they are sent to Application Insights:

```go
handler := slogai.NewHandler(slog.LevelInfo, sink)
handler = handler.WithRedactFields("password", "token", "ssn", "credit_card")

// Sensitive fields are replaced with [REDACTED]
logger.Info("user login",
    "username", "john",       // Logged normally
    "password", "secret123",  // Logged as "[REDACTED]"
)

// Works with grouped fields too
handler = handler.WithRedactFields("user.ssn", "payment.card_number")
```

## Thread Safety

- The `Handler` is safe for concurrent use from multiple goroutines
- Each cloned handler (via `WithAttrs`, `WithGroup`, `WithFieldMappers`, `WithRedactFields`) has its own encoder and mutex to avoid contention
- `AsyncSink` is safe for concurrent writes

## Shutdown

Always close sinks on application shutdown to flush pending telemetry:

```go
// Synchronous sink
sink := slogai.NewSink(cfg)
defer sink.Close()

// Async sink
asyncSink := slogai.NewAsyncSink(sink, asyncCfg)
defer asyncSink.Close()

// For graceful shutdown with signals:
ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
defer cancel()

<-ctx.Done()
asyncSink.Close() // Will drain buffer within DrainTimeout
sink.Close()      // Will flush within GracePeriod
```

## Level Mapping

| slog Level | Application Insights Severity |
|------------|------------------------------|
| Debug | Verbose |
| Info | Information |
| Warn | Warning |
| Error | Error |

## Error Handling

Configure error callbacks to monitor telemetry failures:

```go
cfg, _ := slogai.NewSinkConfig(key)
cfg.OnError = func(err error) {
    // Log internally or send to alternate destination
    log.Printf("Telemetry error: %v", err)
}
```

## Performance

Benchmarks can be run with:

```bash
go test -bench=. -benchmem ./...
```

For high-throughput applications:
1. Use `AsyncSink` to avoid blocking
2. Configure appropriate `BufferSize` based on log volume
3. Monitor `DroppedCount()` to detect buffer overflow
4. Each cloned handler has independent locks for better parallelism

## Testing

```bash
# Run all tests
go test -v ./...

# Run with race detector (requires CGO)
CGO_ENABLED=1 go test -race ./...

# Check coverage
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

## License

See [LICENSE](LICENSE) file.
