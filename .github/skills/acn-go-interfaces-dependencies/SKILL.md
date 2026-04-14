---
name: acn-go-interfaces-dependencies
description: "Go interface design, dependency direction, and package coupling patterns. Use when defining interfaces, choosing where to place them, designing constructors, injecting dependencies, or reviewing import graphs. Trigger on exported interfaces in provider packages, interfaces matching a single type, backwards imports (server importing client), or functions accepting entire config structs when they need one field. Supersedes generic Go interface guidance with stronger opinions on consumer-side interfaces and coupling."
user-invocable: true
license: MIT
compatibility: Designed for Claude Code or similar AI coding agents, and for projects using Golang.
metadata:
  author: rbtr
  version: "1.0.0"
allowed-tools: Read Edit Write Glob Grep Bash(go:*) Bash(golangci-lint:*) Bash(git:*) Agent
---

**Persona:** You are a Go dependency architect. You believe interfaces belong to consumers, not providers. Every exported interface in a provider package and every function that accepts a whole config struct when it needs one field is a design smell that you will flag.

**Modes:**

- **Write mode** — designing new interfaces and package boundaries. Start concrete, extract interfaces only when a second consumer or test demands it.
- **Review mode** — reviewing PR diffs for coupling and interface violations. Flag exported interfaces in provider packages, backwards imports, and over-broad function parameters.
- **Audit mode** — auditing dependency graph health. Launch up to 3 parallel sub-agents: (1) find exported interfaces in non-consumer packages, (2) find functions accepting whole config structs, (3) trace import cycles and backwards dependencies.

> **Supersedes** `samber/cc-skills-golang@golang-structs-interfaces` on interface placement and coupling. The samber skill covers interface mechanics; this skill covers interface _strategy_.

# Go Interface Design & Dependency Direction

> "If something can do _this_, it can be used _here_." — Effective Go

Interfaces in Go are defined by the consumer, not the provider, to describe the behavior that an object must have to be passed in to them.

## Core Principles

1. **Interfaces belong to consumers** — define the interface where you _use_ it, not where you implement it. The consumer controls the contract.
2. **Don't predefine interfaces matching a type** — if your interface simply mirrors a concrete type's method set, delete the interface and use the concrete type until a second consumer appears.
3. **Accept interfaces, return structs** — always. Never return an interface from a constructor.
4. **Pass the minimum needed information** — if a function needs one boolean from a config, pass the boolean, not the config struct. Don't let low-level code depend on high-level config types.
5. **No backwards imports** — servers don't import clients. CNS doesn't import CNI. Higher-level code imports lower-level code, never the reverse.

## Consumer-Defined Interfaces

```go
// ❌ BAD — interface defined in the provider package
// package cns/service
type IPAMStateReconciler interface {
    ReconcileIPAMState(ncs []v1alpha.NetworkContainer, nnc *v1alpha.NodeNetworkConfig) types.ResponseCode
}

// ✅ GOOD — interface defined where consumed
// package nodesubnet (the consumer)
type ipamReconciler interface {
    ReconcileIPAMState(ncs []v1alpha.NetworkContainer, nnc *v1alpha.NodeNetworkConfig) types.ResponseCode
}

type Manager struct {
    reconciler ipamReconciler // accepts anything with this method
}
```

It usually doesn't make sense to have a public interface in a separate package. If `nodesubnet` needs IPAM reconciliation, it defines its own private interface describing exactly that behavior. Any type satisfying those methods works — including the existing concrete implementation.

Reference: [Preemptive Interface Anti-Pattern in Go](https://medium.com/@cep21/preemptive-interface-anti-pattern-in-go-54c18ac0668a)

## Pass Minimum Information

```go
// ❌ BAD — function depends on entire CNS config
func createNCRequest(cnsConfig *configuration.CNSConfig, nc NetworkContainer) Request {
    if cnsConfig.ChannelMode == "swift_v2" {  // leaks scenario knowledge
        // ...
    }
}

// ✅ GOOD — pass only what the function needs
func createNCRequest(processPrimaryIP bool, nc NetworkContainer) Request {
    if processPrimaryIP {
        // ...
    }
}
```

The caller already knows the scenario. The callee only needs to know the behavior. This makes the function reusable for any future scenario that wants the same behavior — without threading a new scenario name all the way down.

## Simplify Handler Surfaces

Handlers should expose one typed request/response surface per operation. Decode at the edge, filter the transport payload down to the fields the operation actually needs, call a narrow dependency, then encode the result. Don't keep a broad `Controller` or `Service` interface around just to mirror every CNS route and pass REST-shaped structs through unchanged.

```go
// ❌ BAD — pass-through controller that just mirrors the REST table
type ipamController interface {
    RequestIPConfig(context.Context, cns.IPConfigRequest) (*cns.IPConfigResponse, error)
    RequestIPConfigs(context.Context, cns.IPConfigsRequest) (*cns.IPConfigsResponse, error)
    ReleaseIPConfig(context.Context, cns.IPConfigsRequest) (*cns.IPConfigsResponse, error)
}

func (service *HTTPRestService) RequestIPConfigHandler(w http.ResponseWriter, r *http.Request) {
    var req cns.IPConfigRequest
    if err := common.Decode(w, r, &req); err != nil {
        return
    }

    resp, err := service.controller.RequestIPConfig(r.Context(), req)
    writeIPConfigResponse(w, resp, err)
}
```

```go
// ✅ GOOD — handler is decode-filter-encode over one operation surface
type requestIPConfigs interface {
    Run(context.Context, RequestIPConfigsInput) (RequestIPConfigsOutput, error)
}

type RequestIPConfigsInput struct {
    PodInterfaceID     string
    InfraContainerID   string
    Ifname             string
    DesiredIPAddresses []string
}

type RequestIPConfigsOutput struct {
    Response  cns.Response
    PodIPInfo []cns.PodIpInfo
}

func (service *HTTPRestService) RequestIPConfigHandler(w http.ResponseWriter, r *http.Request) {
    var req cns.IPConfigRequest
    if err := common.Decode(w, r, &req); err != nil {
        return
    }

    input := RequestIPConfigsInput{
        PodInterfaceID:   req.PodInterfaceID,
        InfraContainerID: req.InfraContainerID,
        Ifname:           req.Ifname,
    }
    if req.DesiredIPAddress != "" {
        input.DesiredIPAddresses = []string{req.DesiredIPAddress}
    }

    out, err := service.requestIPConfigs.Run(r.Context(), input)
    writeIPConfigResponse(w, &cns.IPConfigResponse{
        Response:  out.Response,
        PodIpInfo: out.PodIPInfo[0],
    }, err)
}
```

`RequestIPConfigHandler` can act as a migration adapter from the older one-IP REST surface to a richer multi-IP operation. That's fine. The adapter stays at the edge, stays thin, and gets deleted once callers can speak the direct operation surface. Don't fossilize that adapter into a permanent controller/service layer with dozens of pass-through methods.

## Import Direction

```
✅ GOOD dependency direction:
main → cns/service → cns/types
main → cni/network → cns/types (shared types)

❌ BAD — backwards:
cns/service → cni/network  (server imports client)
cns/restserver → crd/...    (core imports extension)
```

If you find yourself needing types from a "sibling" package, extract shared types to a lower-level package that both can import.

## The stdlib http.Client Pattern

The standard library's `http.Client` is a concrete type. It does not implement a predefined interface. Consumers define their own:

```go
// Your package defines only what it needs
type httpDoer interface {
    Do(req *http.Request) (*http.Response, error)
}

func registerNode(ctx context.Context, client httpDoer, endpoint string) error {
    // works with *http.Client, or any mock
}
```

This is the model. Don't create a global `HTTPClient` interface — each consumer defines its own minimal interface.

## Constructor Design

```go
// ❌ BAD — constructor for struct with no initialization logic
func NewIPTablesClient() *Client {
    return &Client{}
}
// Just use: c := &iptables.Client{}

// ✅ GOOD — constructor when initialization is needed (Poka-yoke)
func NewService(logger *zap.Logger, store UserStore) *Service {
    return &Service{logger: logger, store: store}
}
// Constructor prevents nil logger NPEs — can't forget required deps
```

If a struct's zero value is valid, a constructor is unnecessary boilerplate. Use constructors to enforce required dependencies (Poka-yoke) — if I let you construct directly and you forget the logger, everything NPEs.

## Function Types Over Single-Method Interfaces

When a dependency has only one method, a function type is lighter than an interface — no type to define, no struct to wrap, and it composes naturally with closures and method references.

```go
// ❌ BAD — interface for a single method
type nodeNetworkConfigListener interface {
    Update(*v1alpha.NodeNetworkConfig) error
}

type Reconciler struct {
    listener nodeNetworkConfigListener
}

// ✅ GOOD — function type, lighter and more composable
type nodenetworkconfigSink func(*v1alpha.NodeNetworkConfig) error

type Reconciler struct {
    sink nodenetworkconfigSink
}

// Construction: pass the method reference directly
r := NewReconciler(poolMonitor.Update)  // .Update satisfies the func type

// Or wrap with a closure for extra behavior:
initializer := func(nnc *v1alpha.NodeNetworkConfig) error {
    if err := reconcileState(nnc); err != nil {
        return err
    }
    hasInitialized.Set(1)
    return nil
}
r := NewReconciler(initializer)
```

Function types also make testing trivial — no mock struct needed:

```go
r := NewReconciler(func(nnc *v1alpha.NodeNetworkConfig) error {
    return nil // test stub
})
```

**When to use function types vs interfaces:**
- **1 method** → function type
- **2-3 methods that always travel together** → small interface
- **Methods with shared state** → interface backed by a struct

## Adapter Pattern for Incremental Migration

When replacing a subsystem (v1 → v2), use an adapter to bridge the new implementation back to existing consumers. This avoids flag-day migrations:

```go
// v2 Monitor has a completely different design (channel-driven, no interface)
// Adapter bridges it back to the v1 interface so existing code doesn't change
type v2Adapter struct {
    monitor *v2.Monitor
}

func (a *v2Adapter) Update(nnc *v1alpha.NodeNetworkConfig) error {
    a.monitor.Push(nnc) // adapts interface call to channel push
    return nil
}
```

The adapter can be deleted later when consuming code migrates to the v2 design directly.

## When NOT to Interface

- **One implementation, no tests needing mocks** → use the concrete type
- **Single method** → use a function type instead
- **The interface just mirrors the type** → delete it, use the type
- **You're creating it "for the future"** → YAGNI, extract when needed
- **It's in the provider package** → move it to the consumer

## Common Mistakes

| Mistake | Fix |
| --- | --- |
| Exported interface in provider package | Move to consumer, make private |
| Interface matching single concrete type | Delete interface, use concrete type |
| Returning interface from constructor | Return concrete type |
| Passing `*CNSConfig` to low-level function | Pass the specific bool/string needed |
| Server importing client package | Extract shared types to common package |
| Global `HTTPClient` interface | Each consumer defines its own `httpDoer` |
| Constructor for zero-value struct | Use `&Type{}` directly |
| Creating interface before second implementation | Start concrete, extract when needed |
| Single-method interface | Use a function type instead |
| Flag-day migration to new subsystem | Use adapter to bridge v2 back to v1 consumers |
| Controller/service interface mirrors REST routes | Give each operation one typed request/response surface |
| Adapter became permanent architecture | Keep it as a thin migration shim, then delete it |

## Cross-References

- → See `acn-go-design-boundaries` skill for behavioral config vs scenario config (closely related to pass-minimum-info)
- → See `acn-go-platform-abstraction` skill for OS-specific interface patterns
- → See `samber/cc-skills-golang@golang-structs-interfaces` for interface mechanics (type assertions, embedding, receivers)
- → See `samber/cc-skills-golang@golang-design-patterns` for functional options and constructor patterns
