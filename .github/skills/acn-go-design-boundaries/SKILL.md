---
name: acn-go-design-boundaries
description: "Go code design boundaries — behavioral configuration vs scenario coupling, side effects in functions, function parameter minimality, abstraction timing, control-loop hysteresis, validator purity, and preflight-before-mutation. Use when designing config-driven behavior, reviewing functions that accept whole structs when they need one field, refactoring code that branches on scenario names deep in the stack, stabilizing reconcile loops that request/release on every pass, separating validation from mutation, or evaluating whether a helper function or constructor adds value. Trigger on scenario names (Swift, MTPNC, overlay) appearing in low-level functions, validators that patch inputs while checking them, batch mutations that can partially apply before failing, functions mutating passed references instead of returning values, flapping threshold decisions in reconcile loops, or unnecessary abstractions."
user-invocable: true
license: MIT
compatibility: Designed for Claude Code or similar AI coding agents, and for projects using Golang.
metadata:
  author: rbtr
  version: "1.0.0"
allowed-tools: Read Edit Write Glob Grep Bash(go:*) Bash(golangci-lint:*) Bash(git:*) Agent
---

**Persona:** You are a Go minimalist architect. You believe that every function should accept the least information it needs, return values instead of mutating inputs, keep validators pure, and never know which product scenario invoked it. When you see a scenario name deep in the call stack, a validator that secretly mutates, or a batch mutation that fails halfway through, you know the boundary is wrong.

**Modes:**

- **Write mode** — designing new functionality. Push scenario knowledge to the caller, pass behavioral parameters, keep validators/read-model helpers pure, preflight batches before mutation, and keep reconcile policies stable.
- **Review mode** — reviewing code for boundary violations. Flag scenario names in low-level code, validators that mutate passed references, batch mutations that can partially apply, over-broad parameters, flapping reconcile decisions, and premature abstraction.

> **Complements** `samber/cc-skills-golang@golang-design-patterns` (which covers constructor and options patterns) with stronger opinions on parameter minimality and scenario decoupling.

# Go Design Boundaries

## Configure Behavior, Not Scenarios

Low-level functions must not know which product scenario called them. The scenario is the caller's concern. The callee only needs to know **what behavior is desired**.

```go
// ❌ BAD — scenario name leaks deep into the stack
func createNCRequestFromStaticNCHelper(config *CNSConfig, nc NetworkContainer) Request {
    if config.ChannelMode == "SwiftV2" {
        // process primary prefix IP
    }
}
// Problems:
// 1. Not reusable if a new scenario wants the same behavior
// 2. Requires threading a whole scenario name through the stack
// 3. Low-level code depends on high-level config types

// ✅ GOOD — behavioral parameter
func createNCRequestFromStaticNCHelper(processPrimaryIP bool, nc NetworkContainer) Request {
    if processPrimaryIP {
        // process primary prefix IP
    }
}
// The caller already knows the scenario and passes the bool
// Any future scenario that needs this behavior works without code changes here
```

This seems minor but is actually a **huge architectural tenet**. As implemented with scenario names, the behavior is not reusable — you have to thread a new scenario name all the way down for every new feature. With a behavioral parameter, the caller just passes the bool.

### Applied: Config-Driven Behavior

```go
// ❌ BAD — function needs the whole CNS config to check one thing
func setupHealthCheck(config *configuration.CNSConfig) error {
    if config.ChannelMode == "CRD" {
        // enable NNC health check
    }
}

// ✅ GOOD — pass the behavioral decision
func setupHealthCheck(enableNNCCheck bool) error {
    if enableNNCCheck {
        // enable NNC health check
    }
}
// The caller (main/init) makes the scenario→behavior mapping
```

Initialize behavior in `main` based on scenario config. From that point on, nothing downstream knows the scenario name.

## Side Effects Are Bad

Passing a reference and mutating it is a bad pattern. It's impossible to tell from the calling code that the input is modified. Even if you suspect it, it's impossible to tell _how_ it changes.

```go
// ❌ BAD — mutates input, side effects, nil-pointer risk
func addACLPolicies(podIPInfo *cns.PodIpInfo, policies []policy.Policy) {
    // impossible to tell from the call site that podIPInfo is modified
    // also: nil pointer exception if podIPInfo is nil
    podIPInfo.EndpointPolicies = append(podIPInfo.EndpointPolicies, policies...)
}

// ✅ GOOD — pure function, returns result, no side effects
func buildACLPolicies(existingPolicies []policy.Policy, newPolicies []policy.Policy) []policy.Policy {
    return append(existingPolicies, newPolicies...)
}
// Caller explicitly assigns: info.EndpointPolicies = buildACLPolicies(info.EndpointPolicies, new)
```

When a function doesn't even need the full struct — it only appends to one field — it should be a stateless pure function that returns the result. This is more flexible, more testable, and has no nil-pointer risk.

**Also worth noting:** Go is not C. Passing refs is not always more performant than passing values, because the GC must track pointer references.

## Validators Validate; Mutators Mutate

Validators belong on the pure side of the boundary. Their job is to answer "is this valid?" or "what validation result should the caller act on?" by returning `bool`, `error`, or a small result struct. They must not patch the request, append defaults, reserve addresses, or otherwise mutate inputs as a hidden side effect.

```go
// ❌ BAD — "validator" mutates the request while checking it
func validateReleaseRequest(req *IPConfigsRequest) error {
    if req == nil {
        return fmt.Errorf("request must not be nil")
    }
    if req.NetworkContainerID == "" {
        req.NetworkContainerID = deriveNCID(req.PodInfo) // hidden mutation
    }
    if len(req.IPsToRelease) == 0 {
        req.IPsToRelease = append(req.IPsToRelease, req.PodInfo.PodIP) // more hidden mutation
    }
    return nil
}

// ✅ GOOD — validator only reports validity
func validateReleaseRequest(req IPConfigsRequest) error {
    if req.NetworkContainerID == "" {
        return fmt.Errorf("networkContainerID is required")
    }
    if len(req.IPsToRelease) == 0 {
        return fmt.Errorf("at least one IP must be released")
    }
    return nil
}
```

If you need defaulting or construction, make that a separate, explicitly named step. This is just the validator form of the same pure-function rule: callers should be able to see when mutation happens.

## Preflight the Whole Batch Before Mutating

When applying a batch mutation, do not validate and mutate one item at a time if the operation can fail halfway through. Preflight/validate the full batch first when possible, then perform the mutation. This is the batch form of the same boundary rule: decide first, mutate second.

```go
// ❌ BAD — validates and mutates one IP at a time; partial release is possible
func (m *IPAMManager) releaseSecondaryIPs(ctx context.Context, req IPConfigsRequest) error {
    for _, ip := range req.IPsToRelease {
        if err := validateReleaseIP(req.NetworkContainerID, ip, m.assignedIPsByNC); err != nil {
            return err
        }
        if err := m.client.ReleaseIPs(ctx, IPConfigsRequest{
            NetworkContainerID: req.NetworkContainerID,
            IPsToRelease:       []string{ip},
        }); err != nil {
            return err
        }
    }
    return nil
}
// If the third IP is invalid, the first two may already be gone.

// ✅ GOOD — preflight the whole batch, then perform one mutation step
type releasePlan struct {
    ncID string
    ips  []string
}

func buildReleasePlan(ncID string, requested []string, assigned map[string]struct{}) (releasePlan, error) {
    seen := map[string]struct{}{}
    for _, ip := range requested {
        if _, ok := assigned[ip]; !ok {
            return releasePlan{}, fmt.Errorf("ip %s is not assigned to %s", ip, ncID)
        }
        if _, dup := seen[ip]; dup {
            return releasePlan{}, fmt.Errorf("duplicate IP %s in release request", ip)
        }
        seen[ip] = struct{}{}
    }
    return releasePlan{ncID: ncID, ips: requested}, nil
}

func (m *IPAMManager) releaseSecondaryIPs(ctx context.Context, req IPConfigsRequest) error {
    plan, err := buildReleasePlan(req.NetworkContainerID, req.IPsToRelease, m.assignedIPsByNC[req.NetworkContainerID])
    if err != nil {
        return err
    }
    return m.client.ReleaseIPs(ctx, IPConfigsRequest{
        NetworkContainerID: plan.ncID,
        IPsToRelease:       plan.ips,
    })
}
```

If the downstream API forces one-by-one mutation, still preflight the entire batch first and stop before the first mutation when any item fails validation.

## Minimal Constructors

```go
// ❌ BAD — constructor for struct with no initialization logic
func NewIPTablesClient() *Client {
    return &Client{}
}
// Adds a function that does nothing. Just use &iptables.Client{}

// ✅ GOOD — constructor enforces required dependencies
func NewService(logger *zap.Logger, store UserStore) *Service {
    if logger == nil {
        panic("logger must not be nil") // fail fast, not at first log call
    }
    return &Service{logger: logger, store: store}
}
// If I let you construct directly and you forget the logger, everything NPEs
```

Use constructors when they enforce invariants (Poka-yoke). Don't add constructors whose only job is `return &Type{}`.

## Inline Over Unnecessary Helpers

```go
// ❌ BAD — helper takes more space than inline and obscures what happens
func formatError(operation string, err error) error {
    return fmt.Errorf("failed to %s: %w", operation, err)
}
// ... 
return formatError("create endpoint", err)

// ✅ GOOD — inline is shorter and self-documenting
return fmt.Errorf("creating endpoint: %w", err)
```

If an indirection makes it less obvious what's happening and the helper takes up more space than having the code inline — get rid of it.

**Exception:** centralize formatters for identifiers/keys where consistency matters (see `acn-go-types-parsing` skill).

## Nil as Configuration is Bad

```go
// ❌ BAD — passing nil to configure behavior
func ReconcileIPAMState(nnc *v1alpha.NodeNetworkConfig) {
    if nnc == nil {
        // different behavior when nil
        return
    }
    // normal behavior
}
// Problems:
// 1. Hard to discover — how do you know nil means "skip"?
// 2. In normal scenarios, nil NNC is a fatal error we should catch
// 3. Accidentally passing nil gives wrong behavior instead of an error

// ✅ GOOD — validate inputs immediately, use explicit flags for behavior
func ReconcileIPAMState(nnc *v1alpha.NodeNetworkConfig, skipReconcile bool) error {
    if skipReconcile {
        return nil
    }
    if nnc == nil {
        // this is now clearly an error, not a feature
        return fmt.Errorf("nnc must not be nil")
    }
    // ...
    return nil
}
```

Passing a nil ref is not a sustainable strategy for configuring behavior. It's hard to discover, and it prevents catching actual nil bugs.

## The Decorator/Middleware Pattern

When behavior varies by scenario, compose it at initialization instead of branching deep in the stack:

```go
// ❌ BAD — scenario branching deep in the stack
func (w *Watcher) ReleaseIPs(ctx context.Context, req IPConfigsRequest) error {
    if w.isStatelessCNIWindows {  // scenario knowledge leaked!
        w.deleteHNSEndpoint(req)
    }
    return w.client.ReleaseIPs(ctx, req)
}

// ✅ GOOD — compose behavior at initialization
// In main:
var releaseIPs ReleaseIPsFunc = cnsClient.ReleaseIPs
if isStatelessCNIWindows(config) {
    releaseIPs = withHNSCleanup(cnsClient) // decorator wraps the base
}
watcher := fsnotify.NewWatcher(releaseIPs)
// Watcher has no knowledge of scenarios — zero changes to the fsnotify package
```

## Hysteresis Beats Thrash

If a reconcile/control-loop decision can flap, the boundary is wrong when every pass immediately mutates external state. The loop should own the stability policy; the helper should only execute a clear action.

```go
// ❌ BAD — single threshold, churns external state on every loop
func (m *Manager) reconcilePool(ctx context.Context, freeIPs int) error {
    if freeIPs < m.targetFree {
        return m.client.RequestIPs(ctx, m.batchSize)
    }
    if freeIPs > m.targetFree {
        return m.client.ReleaseIPs(ctx, freeIPs-m.targetFree)
    }
    return nil
}
// One pod create/delete around targetFree can alternate RequestIPs/ReleaseIPs forever.

// ✅ GOOD — hysteresis + sticky/in-flight state
type poolControl struct {
    requestInFlight bool
    releaseInFlight bool
}

func (m *Manager) reconcilePool(ctx context.Context, freeIPs int, ctl *poolControl) error {
    switch {
    case ctl.requestInFlight || ctl.releaseInFlight:
        return nil // wait for the previous external change to settle
    case freeIPs <= m.lowWatermark:
        ctl.requestInFlight = true
        return m.client.RequestIPs(ctx, m.batchSize)
    case freeIPs >= m.highWatermark:
        ctl.releaseInFlight = true
        return m.client.ReleaseIPs(ctx, freeIPs-m.targetFree)
    default:
        return nil // stay sticky inside the band
    }
}
```

Use enter/exit thresholds, sticky state, or in-flight tracking whenever a pool-scaling or release decision can oscillate. Don't bury "should I request or release?" inside a helper that gets called every reconcile. Keep the stability policy at the boundary that owns the loop, and keep the action function dumb.

## errgroup for Goroutine Lifecycle

When multiple goroutines must run concurrently and any failure should tear down the group:

```go
// ❌ BAD — manual channel-based error collection
errs := make(chan error)
go func() { errs <- watchPendingDelete(ctx) }()
go func() { errs <- watchFS(ctx) }()
err := <-errs // only gets first error, leaks the other goroutine

// ✅ GOOD — errgroup.WithContext
g, groupCtx := errgroup.WithContext(ctx)
g.Go(func() error { return w.watchPendingDelete(groupCtx) })
g.Go(func() error { return w.watchFS(groupCtx) })
return g.Wait() // cancels groupCtx on first error, waits for all
```

The first error cancels `groupCtx`, signaling all goroutines to stop. `Wait()` blocks until all have exited. No leaked goroutines, no manual channel management.

## Constructors Must Return Errors

If construction can fail, return the error — don't silently log and return a half-initialized struct:

```go
// ❌ BAD — silently swallows error, returns broken watcher
func New(path string, logger *zap.Logger) *watcher {
    if err := os.Mkdir(path, 0o755); err != nil {
        logger.Error("error making directory", zap.Error(err))
        // continues with broken state!
    }
    return &watcher{path: path}
}

// ✅ GOOD — caller can handle the failure
func New(path string, logger *zap.Logger) (*watcher, error) {
    if err := os.Mkdir(path, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
        return nil, errors.Wrapf(err, "failed to create dir %s", path)
    }
    return &watcher{path: path}, nil
}
```

## Async Metrics Must Not Block Hot Paths

Telemetry collection must never add latency to request handling:

```go
// ❌ BAD — synchronous metrics in request handler
func (s *Service) RequestIPConfig(ctx context.Context, req Request) Response {
    resp := s.allocateIP(req)
    s.publishMetrics(ctx)  // blocks on metrics publish!
    return resp
}

// ✅ GOOD — defer async recording
func (s *Service) RequestIPConfig(ctx context.Context, req Request) Response {
    defer s.recorder.Record()  // triggers async channel-based worker
    return s.allocateIP(req)
}

// Recorder uses sync.Once + channel worker pattern
type Recorder struct {
    once sync.Once
    ch   chan struct{}
}

func (r *Recorder) Record() {
    r.once.Do(func() { go r.worker() })
    select {
    case r.ch <- struct{}{}:
    default: // drop if worker is busy — metrics are best-effort
    }
}
```

## Fail Fast on Invariant Violations

When internal state is inconsistent — like duplicate IPs in the pool — treat it as unrecoverable. Don't silently last-write-wins.

```go
// ❌ BAD — silently overwrites duplicate, hides corruption
for _, ip := range ips {
    state[ip.Address] = ip // duplicate? too bad, last write wins
}

// ✅ GOOD — detect and fail immediately
var ErrDuplicateIP = errors.New("duplicate IP in state")

for _, ip := range ips {
    if _, exists := state[ip.Address]; exists {
        return fmt.Errorf("%w: %s", ErrDuplicateIP, ip.Address)
    }
    state[ip.Address] = ip
}
```

If this state corruption is truly unrecoverable, crash CNS so it restarts cleanly rather than operating with corrupted state. A crash is observable (CrashLoopBackoff, alerts); silent corruption is invisible.

## Decision Checklist

Before adding a parameter, helper, or branch:

| Question | If yes... |
| --- | --- |
| Does this function need to know the scenario name? | Pass a behavioral bool/enum instead |
| Does this function mutate a passed reference? | Return a value instead |
| Is this validator mutating or defaulting while it checks? | Return `bool`/result/`error`; keep mutation in a separate step |
| Can this batch mutation fail halfway through? | Preflight the whole batch first, then mutate |
| Does this helper save code? | If not, inline it |
| Does this constructor do anything? | If only `&Type{}`, delete it |
| Is nil used to mean "skip"? | Use an explicit flag |
| Does this low-level package import high-level config? | Pass only the needed values |
| Can this reconcile decision flap around a threshold? | Add enter/exit thresholds or sticky/in-flight state |

## Common Mistakes

| Mistake | Fix |
| --- | --- |
| `if config.ChannelMode == "SwiftV2"` in helper | Pass `processPrimaryIP bool` |
| Mutating `*PodIpInfo` in-place via helper | Return `[]policy.Policy`, let caller assign |
| `validateReleaseRequest(req *IPConfigsRequest)` fills in NC IDs or IPs while checking | Return `error`/result only; do defaulting or building separately |
| `for _, ip := range ips { validateReleaseIP(ip); ReleaseIPs([]string{ip}) }` | Validate/build the full release plan first, then mutate |
| `NewClient()` that just does `return &Client{}` | Use `&Client{}` directly |
| Helper function larger than the inline code | Inline it |
| `if nnc == nil { /* different behavior */ }` | Validate nil as error, use explicit flags |
| `cnsConfig.ManageEndpointState && runtime.GOOS == "windows"` deep in stack | Resolve in main, pass result |
| `if freeIPs < targetFree { RequestIPs } else if freeIPs > targetFree { ReleaseIPs }` every reconcile | Use low/high watermarks and in-flight tracking |
| Creating a whole wrapper package for one function | Inline or use anonymous func |

## Cross-References

- → See `acn-go-interfaces-dependencies` skill for passing minimum info and dependency direction
- → See `acn-go-platform-abstraction` skill for OS-specific behavior boundaries
- → See `samber/cc-skills-golang@golang-design-patterns` for functional options, builders, and constructor patterns
- → See `samber/cc-skills-golang@golang-code-style` for function parameter and control flow rules
