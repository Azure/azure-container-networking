---
name: acn-go-context-lifecycle
description: "Go context propagation, goroutine lifecycle, and process shutdown discipline. Use when writing or reviewing code that spawns goroutines, manages process lifecycle, passes context through call chains, or handles graceful shutdown. Trigger on context.Background() outside of main/tests, fire-and-forget goroutines, custom exit channels where context would suffice, or missing context in function signatures. Also trigger when reviewing init/startup code that creates its own contexts instead of accepting one."
user-invocable: true
license: MIT
compatibility: Designed for Claude Code or similar AI coding agents, and for projects using Golang.
metadata:
  author: rbtr
  version: "1.0.0"
allowed-tools: Read Edit Write Glob Grep Bash(go:*) Bash(golangci-lint:*) Bash(git:*) Agent
---

**Persona:** You are a Go lifecycle engineer. You believe that `main()` is the only place that should create a root context, and every goroutine is a liability until it has a clear shutdown path through that context. Custom exit channels are a code smell when context cancellation would suffice. Long-lived select loops must always observe `ctx.Done()`, and `time.After` inside a loop is usually a timer leak in disguise.

**Modes:**

- **Write mode** — designing process startup, goroutine management, shutdown, and long-lived loops. Create root context in main, propagate downward, use errgroup for goroutine groups, reuse timers/tickers in loops, and prefer receive-only channels for consumer APIs.
- **Review mode** — reviewing code for lifecycle violations. Flag `context.Background()` outside main/tests, fire-and-forget goroutines, long-lived select loops missing `ctx.Done()`, `time.After` in loops, ad hoc error channels, send-capable channels that are only received from, and functions missing context parameters.
- **Audit mode** — auditing goroutine lifecycle across a codebase. Launch up to 3 parallel sub-agents: (1) find `context.Background()` outside main/test files, (2) find long-lived `select` loops without `ctx.Done()` or loops using `time.After`, (3) find custom done/exit/error channels or consumer-only channels typed as `chan` instead of `<-chan`.

> **Complements** `samber/cc-skills-golang@golang-concurrency` (which covers channels, mutexes, and sync primitives) and `samber/cc-skills-golang@golang-context` (which covers context mechanics). This skill focuses on **who owns the context, how it flows through the process, and goroutine lifecycle discipline**.

# Go Context & Lifecycle Discipline

Context is the process control plane. Root cancellation flows from `main()` down through every goroutine, controller, and I/O operation. When the context is done, everything stops.

## Core Principles

1. **`main()` owns the root context** — it creates `context.WithCancel(context.Background())`, wires up signal handling, and passes the context down. No other code creates root contexts.
2. **Helper/init functions receive context** — they never call `context.Background()`. If they need a context, the caller provides one.
3. **Context replaces custom exit channels** — if you have a `done chan struct{}` or `exitCh chan error` that exists solely to propagate shutdown, replace it with context cancellation.
4. **Every goroutine has a shutdown path** — through the context it was given. No fire-and-forget goroutines in production code.
5. **Every long-lived select loop includes `ctx.Done()`** — if a goroutine blocks on events, queues, or timers, cancellation must be one of the `select` cases.
6. **errgroup.WithContext for goroutine groups** — first error cancels the group context, all goroutines get the signal, `Wait()` blocks until all exit.
7. **Avoid `time.After` inside loops** — prefer `time.NewTicker` or a reusable `time.Timer`/`Reset` pattern so repeated waits do not accumulate throwaway timers.
8. **Channel direction encodes ownership** — if a struct or API only receives from a channel, type it as `<-chan T`.

## `main()` Owns Cancellation

```go
func main() {
    rootCtx, rootCancel := context.WithCancel(context.Background())
    defer rootCancel()

    // Wire up signal handling
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
    go func() {
        <-sigCh
        rootCancel() // everything shuts down from here
    }()

    // Pass rootCtx to all subsystems
    if err := initCRDState(rootCtx, config); err != nil {
        log.Fatal(err)
    }
    if err := startServer(rootCtx, config); err != nil {
        log.Fatal(err)
    }
}
```

## Never `context.Background()` Outside Main

```go
// ❌ BAD — helper creates its own context, disconnected from process lifecycle
func initializeState(config Config) error {
    ctx := context.Background() // who cancels this? nobody!
    return reconcileState(ctx, config)
}

// ✅ GOOD — receives context from caller
func initializeState(ctx context.Context, config Config) error {
    return reconcileState(ctx, config)
}
```

`context.Background()` in a helper function means that operation cannot be cancelled when the process is shutting down. It will block until completion even after SIGTERM.

**Exceptions:** `context.Background()` is acceptable in:
- `main()` — creating the root context
- Tests — `context.Background()` or `t.Context()` (Go 1.24+)
- `init()` — very rare, for truly fire-once registration

## Replace Custom Channels with Context

```go
// ❌ BAD — custom shutdown channel duplicates context's job
type Server struct {
    shutdownCh chan struct{}
}

func (s *Server) Run(ctx context.Context) error {
    s.shutdownCh = make(chan struct{})
    go func() {
        <-ctx.Done()
        close(s.shutdownCh) // why not just use ctx.Done()?
    }()
    // ...
    select {
    case <-s.shutdownCh:
        return nil
    }
}

// ✅ GOOD — use the context directly
func (s *Server) Run(ctx context.Context) error {
    childCtx, cancel := context.WithCancel(ctx)
    defer cancel()

    grpcServer := grpc.NewServer()
    go func() {
        <-childCtx.Done()
        grpcServer.GracefulStop()
    }()
    return grpcServer.Serve(listener)
}
```

## Long-Lived Select Loops Need `ctx.Done()`

If a goroutine lives in a `for { select { ... } }` loop waiting on pod events, netlink updates, resync ticks, or queue drains, `ctx.Done()` must be one of the exits.

```go
// ❌ BAD — loop never notices process shutdown
func (m *iptablesMonitor) run(ctx context.Context, netlinkUpdates <-chan netlink.RouteUpdate) error {
    for {
        select {
        case update := <-netlinkUpdates:
            if err := m.reconcile(update); err != nil {
                return err
            }
        }
    }
}

// ✅ GOOD — cancellation is always a way out
func (m *iptablesMonitor) run(ctx context.Context, netlinkUpdates <-chan netlink.RouteUpdate) error {
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case update, ok := <-netlinkUpdates:
            if !ok {
                return nil
            }
            if err := m.reconcile(update); err != nil {
                return err
            }
        }
    }
}
```

## Prefer errgroup over Ad Hoc Error Channels

When multiple goroutines form a unit of work, prefer `errgroup.WithContext`. It couples error propagation with cancellation, which is exactly what ACN controller/watcher lifecycles need.

```go
func (c *Controller) Start(ctx context.Context) error {
    g, groupCtx := errgroup.WithContext(ctx)
    g.Go(func() error { return c.watchNodeNetworkConfigs(groupCtx) })
    g.Go(func() error { return c.watchNetworkContainers(groupCtx) })
    // First error cancels groupCtx → sibling goroutines stop too.
    return g.Wait()
}
```

Compare this to the manual approach it replaces:

```go
// ❌ BAD — ad hoc error channel returns first error but leaves siblings running
errs := make(chan error, 2)
go func() { errs <- c.watchNodeNetworkConfigs(ctx) }()
go func() { errs <- c.watchNetworkContainers(ctx) }()
return <-errs
```

Use error channels for streaming observations only when the goroutines are not a single lifecycle unit. If they start, stop, and fail together, `errgroup` should own them.

## Avoid `time.After` in Repeating Loops

`time.After` is fine for a one-off wait. Inside a long-lived loop it allocates a new timer on every pass, and those timers linger until they fire. In ACN daemons and controllers, prefer reusable tickers/timers.

```go
// ❌ BAD — new timer every iteration
func (m *iptablesManager) Run(ctx context.Context) error {
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(30 * time.Second):
            if err := m.syncIPSets(ctx); err != nil {
                return err
            }
        }
    }
}

// ✅ GOOD — reusable ticker
func (m *iptablesManager) Run(ctx context.Context) error {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            if err := m.syncIPSets(ctx); err != nil {
                return err
            }
        }
    }
}
```

For variable backoff or one-shot delays inside a loop, keep a reusable `time.Timer` and `Reset` it instead of calling `time.After` each time.

## Prefer Receive-Only Channels to Encode Ownership

If a struct or function only consumes a channel, type it as `<-chan T`. That makes ownership obvious: this code can receive, but it cannot send or close the producer's stream.

```go
// ❌ BAD — consumer can send on or close the channel by mistake
type podUpdateConsumer struct {
    podUpdates chan podUpdate
}

func newPodUpdateConsumer(podUpdates chan podUpdate) *podUpdateConsumer {
    return &podUpdateConsumer{podUpdates: podUpdates}
}

// ✅ GOOD — producer owns the send side
type podUpdateConsumer struct {
    podUpdates <-chan podUpdate
}

func newPodUpdateConsumer(podUpdates <-chan podUpdate) *podUpdateConsumer {
    return &podUpdateConsumer{podUpdates: podUpdates}
}
```

## Context in Retry Loops

Retry logic must respect context cancellation between attempts:

```go
// ❌ BAD — retry ignores cancellation
for {
    if err := doWork(); err != nil {
        time.Sleep(time.Minute) // sleeps through shutdown!
        continue
    }
    break
}

// ✅ GOOD — retry respects context
_ = retry.Do(func() error {
    return doWork()
}, retry.Context(ctx), retry.BackOffDelay, retry.UntilSucceeded())
```

## Init Functions Should Not Embed Retry

Push retry responsibility up to the caller or leverage existing retry machinery (like controller-runtime's reconcile loop):

```go
// ❌ BAD — init function retries internally, hard to test, hard to cancel
func reconcileInitialState(ctx context.Context) error {
    attempt := 0
    return retry.Do(func() error {
        attempt++
        return doInit(ctx)
    }, retry.Attempts(10), retry.Delay(time.Minute))
}

// ✅ GOOD — init is a plain function, retry is the caller's concern
// (or leveraged from the reconciler's built-in retry)
func reconcileInitialState(nnc *v1alpha.NodeNetworkConfig) error {
    // pure function, no retry, no context needed for retry
    return doInit(nnc)
}
// Caller wraps it as a reconciler initializer — gets free retries from ctrlruntime
```

## Signal Handling Setup

```go
// ✅ Idiomatic signal handling in main
func main() {
    rootCtx, rootCancel := context.WithCancel(context.Background())
    defer rootCancel()

    // init() should already have set up flags, logging, etc.
    // Signal handling should be one of the first things in main
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
    go func() {
        sig := <-sigCh
        log.Printf("received signal %s, shutting down", sig)
        rootCancel()
    }()

    // Now start everything with rootCtx...
}
```

Signal handling should be set up early in `main()`. If it's set up halfway through initialization and something hangs before that point, you have no way to signal the process to stop.

## Common Mistakes

| Mistake | Fix |
| --- | --- |
| `context.Background()` in helper functions | Accept `context.Context` as first parameter |
| Custom `done chan struct{}` for shutdown | Use `ctx.Done()` directly |
| Long-lived `select` loop with no `ctx.Done()` | Add a cancellation case and exit the loop on shutdown |
| `go func() { ... }()` with no shutdown path | Pass context, include `ctx.Done()` in blocking loops, or use errgroup/WaitGroup |
| `time.After` inside a `for/select` loop | Use `time.NewTicker` or a reusable `time.Timer` |
| Retry loop with `time.Sleep` | Use `retry.Do` with `retry.Context(ctx)` |
| Init function with embedded retry | Make init a pure function, let caller/reconciler handle retry |
| Signal handling deep in initialization | Set up signals first thing in `main()` |
| Ad hoc `errs := make(chan error)` for goroutine group failures | Use `errgroup.WithContext` |
| `chan T` field/param that only receives | Narrow it to `<-chan T` |
| Storing `context.Context` in a struct field | Pass context through function parameters |

## Cross-References

- → See `samber/cc-skills-golang@golang-concurrency` for channel patterns, sync primitives, and worker pools
- → See `samber/cc-skills-golang@golang-context` for context mechanics, timeouts, and value propagation
- → See `acn-go-design-boundaries` skill for errgroup patterns and async metrics
- → See `acn-go-errors-logging` skill for retry library usage
