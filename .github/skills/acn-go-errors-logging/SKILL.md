---
name: acn-go-errors-logging
description: "Go error string formatting, zap logging discipline, and adjacent observability naming for production services. Use when writing error messages, choosing log levels, shaping zap messages and fields, deciding between log-and-return or single-handling, or reviewing error/log quality. Trigger on capitalized error strings, metadata stuffed into message prefixes, zap.Any used for known types, vague metric/config names, bare root logger usage, or logs that describe code instead of outcomes. Complements samber/cc-skills-golang@golang-error-handling with stronger opinions on conciseness, field discipline, and level discipline."
user-invocable: true
license: MIT
compatibility: Designed for Claude Code or similar AI coding agents, and for projects using Golang.
metadata:
  author: rbtr
  version: "1.1.0"
allowed-tools: Read Edit Write Glob Grep Bash(go:*) Bash(golangci-lint:*) Bash(git:*) Agent
---

**Persona:** You are a Go SRE who reads thousands of log lines daily. Every unnecessary word in an error string or log message is noise that hides signal. You optimize for the person debugging at 3am with `grep` and structured log queries.

**Modes:**

- **Write mode** — writing error handling and logging for new code. Apply single-handling rule, format errors as lowercase fragments, choose log levels deliberately, keep messages stable, and put runtime metadata in typed zap fields.
- **Review mode** — reviewing error/log quality in PR diffs. Flag capitalized errors, verbose logs, log-and-return violations, expanded acronyms, metadata hidden in message text, `zap.Any` for known types, vague metric/config names, and logs that describe code flow instead of outcomes.

> **Complements** `samber/cc-skills-golang@golang-error-handling` (which covers error mechanics) and `samber/cc-skills-golang@golang-observability` (which covers observability infrastructure). This skill focuses on **error string quality, log conciseness, zap field discipline, and nearby metric/config naming that affects production debugging** specifically.

# Go Error & Logging Discipline

## Error String Rules

Error strings are fragments, not sentences. They get wrapped and embedded in other errors and logs. Sentence-casing reads wrong when embedded.

```go
// ❌ BAD — sentence-cased, reads wrong when wrapped
return errors.New("Connection string cannot be empty")
// produces: "failed to build appinsights client: Connection string cannot be empty"

// ✅ GOOD — lowercase fragment, composes naturally
return errors.New("connection string cannot be empty")
// produces: "failed to build appinsights client: connection string cannot be empty"
```

### Error String Principles

1. **Lowercase, no trailing punctuation** — errors are fragments that get composed with `fmt.Errorf("context: %w", err)`
2. **Concise** — describe what failed, not what to do about it
3. **Include the package name in sentinel errors** — `errors.New("apiclient: not found")` identifies origin
4. **Embed structured data in the error, not the string** — embed fields in custom error types or attach them as zap fields at the log site

```go
// ❌ BAD — verbose, prescriptive
return fmt.Errorf("Failed to retrieve network container version list from NMAgent")

// ✅ GOOD — concise, descriptive
return fmt.Errorf("failed to get nc version list from nmagent")
```

## Logging Philosophy

**Logs need to be concise and information dense. More to read is actually _worse_ for readability.**

### Don't Expand Well-Known Acronyms

| ❌ Verbose | ✅ Concise | Why |
| --- | --- | --- |
| "IP Address" | "IP" | "Address" is redundant — it's in the "A" |
| "Network Container" | "NC" | Standard domain term, everyone knows it |
| "Failed to synchronize host network container versions with NMAgent" | "sync host nc versions error" | Shorter, same information |

Writing out well-understood acronyms is not better. For logs, longer is not better.

### Log What Happened, Not What the Code Does

```go
// ❌ BAD — describes the code, not the outcome
logger.Debug("checking if primary IP should be processed")

// ✅ GOOD — tells us what happened and what decision was made
logger.Debug("primary IP processing decision",
    zap.Bool("processPrimary", processPrimary),
    zap.Bool("overlayMode", overlayMode),
)
```

A log that merely restates the code is useless — the reader can read the code. Logs should tell you _what happened_ at runtime: what values, what decisions, what outcomes.

Move logs inside or after conditionals to give information about the branch taken.

### Metadata Belongs in Fields, Not Message Prefixes

Keep the message about the event. Put variable metadata in fields so the message stays short, stable, and queryable.

```go
// ❌ BAD — metadata buried in message text
logger.Info("pod=" + podName + " ncID=" + ncID + " hostNC=true nc refresh succeeded")

// ✅ GOOD — stable message, structured metadata
logger.Info("nc refresh succeeded",
    zap.String("pod", podName),
    zap.String("ncID", ncID),
    zap.Bool("hostNC", true),
)
```

Do not build pseudo-key/value prefixes into messages. They make aggregation, deduping, and filtering harder than a stable message plus fields.

### Prefer Typed zap Fields Over `zap.Any`

If the type is known, encode it with the matching zap helper. `zap.Any` is for genuinely dynamic payloads or short-lived debugging, not routine production logs.

```go
// ❌ BAD — known types hidden behind zap.Any
logger.Debug("nc reconcile config",
    zap.Any("retryCount", retryCount),
    zap.Any("timeoutSeconds", cfg.TimeoutSeconds),
    zap.Any("reconcileInterval", cfg.ReconcileInterval),
)

// ✅ GOOD — preserve type information explicitly
logger.Debug("nc reconcile config",
    zap.Int("retryCount", retryCount),
    zap.Int("timeoutSeconds", cfg.TimeoutSeconds),
    zap.Duration("reconcileInterval", cfg.ReconcileInterval),
)
```

Prefer `zap.String`, `zap.Int`, `zap.Bool`, `zap.Duration`, `zap.Error`, `zap.Stringer`, etc. when the type is known.

### Derive Loggers, Don't Pass Root

```go
// ❌ BAD — bare root logger, no context
func NewService(logger *zap.Logger) *Service {
    return &Service{logger: logger}
}
// all logs say "msg=something" with no indication of source

// ✅ GOOD — derived logger with identifying fields
func NewService(logger *zap.Logger) *Service {
    return &Service{logger: logger.With(zap.String("component", "service"))}
}
```

### Don't Log Internal State on Hot Paths

```go
// ❌ BAD — dumps entire state map on every request
log.Printf("current state: %+v", internalState)

// ✅ GOOD — log at debug level, log only the relevant key
logger.Debug("ip config lookup", zap.String("ip", requestedIP))
```

## Single-Handling Rule

Errors MUST be either logged OR returned — **never both**. Duplicated log lines in aggregation services are noise that obscure the real signal.

```go
// ❌ BAD — log AND return (duplicates in aggregation)
if err != nil {
    logger.Error("failed to get NCs", zap.Error(err))
    return err
}

// ✅ GOOD — return with context, let the caller decide
if err != nil {
    return fmt.Errorf("getting NCs: %w", err)
}
```

The caller that ultimately handles the error can log it with full context from the error chain.

## Log Level Discipline

| Level | Use for | Stack traces? |
| --- | --- | --- |
| **Debug** | Internal state, decision points, useful context for troubleshooting | No |
| **Info** | Significant lifecycle events (started, stopped, config loaded) | No |
| **Warn** | Degraded but recoverable situations | No |
| **Error** | Failures requiring attention | Yes (Error+) |

Stack traces at Warn are noise. Reserve them for Error and above where you actually need the call site to diagnose.

```go
// ❌ BAD — stack at Warn level, not useful
logger.Warn("retrying connection", zap.Stack("stack"))

// ✅ GOOD — stack only at Error
logger.Error("connection permanently failed", zap.Error(err)) // zap adds stack at Error+
```

### Log Format Considerations

- **stdout (human-readable):** logfmt is more readable than JSON for `kubectl logs`
- **Machine-consumed outputs:** JSON for log aggregation services (Datadog, Loki, etc.)
- **AI/telemetry sinks:** JSON for structured parsing

## Observability Naming That Supports Debugging

These naming rules belong here because operators correlate logs, errors, metrics, and config when debugging the same incident. This is not a general observability design guide.

### Metric Names, Labels, and Help Must Be Explicit

Use explicit, `snake_case` metric names and label keys, plus help text that says exactly what is being counted or measured.

```go
// ❌ BAD — vague metric, vague label, vague help
refreshErrors := prometheus.NewCounterVec(prometheus.CounterOpts{
    Name: "Errors",
    Help: "Error count",
}, []string{"type"})

// ✅ GOOD — explicit, searchable, and consistent with the log/event name
refreshFailures := prometheus.NewCounterVec(prometheus.CounterOpts{
    Name: "nmagent_nc_refresh_failures_total",
    Help: "Total number of failed NMAgent network container refresh operations.",
}, []string{"operation", "failure_reason"})
```

If the error log says `"nc refresh failed"`, the related metric should not be named `Errors`. Make the signal self-explanatory.

### Config Names Must Carry Units

When config is numeric, the name must tell the operator the unit. If the type is `time.Duration`, the name still needs a clear duration/interval meaning.

```go
// ❌ BAD — unit is unclear
type RawNMAgentConfig struct {
    RequestTimeout int `json:"requestTimeout"`
    SyncPeriod     int `json:"syncPeriod"`
}

// ✅ GOOD — unit is obvious from the config name or type
type NMAgentConfig struct {
    RequestTimeoutSeconds int           `json:"requestTimeoutSeconds"`
    ReconcileInterval     time.Duration `json:"reconcileInterval"`
}
```

`timeout=30` is ambiguous. `timeoutSeconds=30` and `reconcileInterval=5s` are not.

## Useful Debug Logs, Not Deleted Logs

```go
// ❌ BAD — deleting logs entirely when they're noisy
// (just deleted the log lines)

// ✅ GOOD — change the level instead
logger.Debug("host nc version response", zap.Int("count", len(versions)))
```

Most noisy logs are useful _debug_ logs. Change the level instead of deleting them.

## Retry with Libraries, Not Hand-Rolled Loops

```go
// ❌ BAD — hand-rolled retry with sleep
go func() {
    for {
        if err := w.Start(ctx); err != nil {
            logger.Error("failed, will retry", zap.Error(err))
            time.Sleep(time.Minute)
            continue
        }
        return
    }
}()

// ✅ GOOD — use retry library with backoff and context
go func() {
    _ = retry.Do(func() error {
        w, err := fsnotify.New(path, logger)
        if err != nil {
            return errors.Wrap(err, "failed to create watcher")
        }
        return w.Start(ctx)
    }, retry.BackOffDelay, retry.Attempts(0), retry.Context(ctx))
}()
```

Use `avast/retry-go` (already in the codebase) instead of hand-rolled retry loops. It provides exponential backoff, context cancellation, and attempt limits for free.

### Closure-Returning Factory Functions for Retry

When retry logic needs setup, return a closure that `retry.Do` can call:

```go
func tryStopServiceFn(ctx context.Context, svc managedService) func() error {
    return func() error {
        status, err := svc.Query()
        if err != nil {
            return errors.Wrap(err, "could not query service")
        }
        if status.State == svc.Stopped {
            return nil
        }
        _, err = svc.Control(svc.Stop)
        return errors.Wrap(err, "could not stop service")
    }
}

// Usage: composable with retry
_ = retry.Do(tryStopServiceFn(ctx, service), retry.UntilSucceeded(), retry.Context(ctx))
```

## Common Mistakes

| Mistake | Fix |
| --- | --- |
| `errors.New("Connection string cannot be empty")` | Lowercase: `"connection string cannot be empty"` |
| `"Failed to retrieve network container version list from NMAgent"` | Concise: `"failed to get nc version list from nmagent"` |
| Logging AND returning the same error | Pick one. Return with context, or log and handle |
| `logger.Warn("...", zap.Stack("stack"))` | Reserve stacks for Error+ |
| Passing bare root `*zap.Logger` | Derive with `.With(zap.String("component", ...))` |
| `logger.Info("pod=... ncID=... refresh succeeded")` | Keep the message stable and move metadata into zap fields |
| `zap.Any("reconcileInterval", cfg.ReconcileInterval)` | Use `zap.Duration(...)` or another typed field when the type is known |
| Metric `Name: "Errors", Help: "Error count", []string{"type"}` | Use explicit `snake_case` names/labels and specific help text, e.g. `nmagent_nc_refresh_failures_total`, `operation`, `failure_reason` |
| Config `RequestTimeout int` | Encode units: `RequestTimeoutSeconds int` or `RequestTimeout time.Duration` with a clear external format |
| Deleting noisy logs | Change level to Debug instead |
| Log that describes code flow | Log what happened: values, decisions, outcomes |
| Expanding "NC" to "Network Container" in logs | Well-known acronyms are fine and preferred for density |
| Hand-rolled `for { ... time.Sleep ... }` retry | Use `retry.Do` with backoff and context |

## Cross-References

- → See `samber/cc-skills-golang@golang-error-handling` for error creation, wrapping, `errors.Is`/`errors.As`, and panic/recover
- → See `samber/cc-skills-golang@golang-observability` for structured logging setup and middleware
- → See `acn-go-types-parsing` skill for embedding structured data in error types
