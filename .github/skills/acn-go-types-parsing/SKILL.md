---
name: acn-go-types-parsing
description: "Go type safety, standard library type preferences, and the 'parse don't validate' principle. Use when handling IP addresses, MAC addresses, network types, or any string-typed domain data. Trigger on net.IP usage (prefer netip.Addr), string MAC addresses (prefer net.HardwareAddr), stringly-typed variables that should be custom types, or reflect.DeepEqual where == suffices. Also trigger on 'parse don't validate' discussions or type conversion at boundaries."
user-invocable: true
license: MIT
compatibility: Designed for Claude Code or similar AI coding agents, and for projects using Golang.
metadata:
  author: rbtr
  version: "1.0.0"
allowed-tools: Read Edit Write Glob Grep Bash(go:*) Bash(golangci-lint:*) Bash(git:*) Agent
---

**Persona:** You are a Go type system advocate. You believe that if data passes through your code as a `string` when a richer type exists, every consumer must re-parse and re-validate it — multiplying bugs. Parse once at the boundary, use types everywhere else.

**Modes:**

- **Write mode** — writing new code that handles network types, identifiers, or domain data. Parse to types at the entry point, use typed values throughout.
- **Review mode** — reviewing code for type safety. Flag string IPs, string MACs, stringly-typed config, and unnecessary reflection. Sequential.
- **Audit mode** — auditing type safety across a codebase. Launch up to 3 parallel sub-agents: (1) find `net.IP` usage that should be `netip.Addr`, (2) find string MAC addresses, (3) find stringly-typed domain identifiers.

> **Complements** `samber/cc-skills-golang@golang-safety` with domain-specific type guidance for network services.

# Go Type Safety: Parse, Don't Validate

> [Parse, don't validate](https://lexi-lambda.github.io/blog/2019/11/05/parse-don-t-validate/) — convert data to types at the boundary so that the rest of your code cannot represent invalid states.

## Core Principle

Convert raw data (strings, bytes) to typed values at the system boundary. Once parsed, the type carries the validation with it — downstream code cannot accidentally misuse it.

```go
// ❌ BAD — string IP passed through multiple functions, re-parsed each time
func configureEndpoint(ipStr string) error {
    // every consumer must validate: is this v4? v6? valid at all?
    ip := net.ParseIP(ipStr)
    if ip == nil {
        return fmt.Errorf("invalid IP: %s", ipStr)
    }
    // ...
}

// ✅ GOOD — parse at boundary, use typed value everywhere
func configureEndpoint(addr netip.Addr) error {
    // addr is guaranteed valid, can query .Is4()/.Is6() directly
    if addr.Is6() {
        return configureV6(addr)
    }
    return configureV4(addr)
}
```

## Prefer `netip` Over `net` for IP Handling

The `net/netip` package (Go 1.18+) provides value types that are comparable, more efficient, and safer than the `net` package equivalents.

| Old (`net`) | New (`netip`) | Why prefer `netip` |
| --- | --- | --- |
| `net.IP` | `netip.Addr` | Value type, comparable with `==`, `.Is4()`/`.Is6()` methods |
| `net.IPNet` | `netip.Prefix` | Value type, `.Overlaps()` method, no nil footgun |
| `net.ParseIP()` | `netip.ParseAddr()` | Returns error (not nil), forces error handling |
| `net.ParseCIDR()` | `netip.ParsePrefix()` | Returns error, cleaner API |

```go
// ❌ BAD — old net package, nil return on error
ip := net.ParseIP(ipStr)
if ip == nil { // easy to forget this check
    // ...
}

// ✅ GOOD — netip returns error, forces handling
addr, err := netip.ParseAddr(ipStr)
if err != nil {
    return fmt.Errorf("parsing IP %q: %w", ipStr, err)
}
// addr.Is4(), addr.Is6() — no ambiguity
```

**Conversion at boundaries:** You will have to convert at existing type boundaries (APIs, structs using `net.IP`), but everything in between gets improved.

### Subnet Overlap with `netip.Prefix`

```go
// ✅ Use Prefix.Overlaps() instead of manual bitmask comparison
subnet1, _ := netip.ParsePrefix("10.0.0.0/8")
subnet2, _ := netip.ParsePrefix("10.1.0.0/16")
if subnet1.Overlaps(subnet2) {
    // subnet1 fully contains subnet2
}
```

`Overlaps` normalizes to the network address even if the CIDR uses a different base address, so it is sufficient alone.

## Use `net.HardwareAddr` for MAC Addresses

```go
// ❌ BAD — MAC as string, format inconsistencies everywhere
type Device struct {
    MAC string // "00:11:22:33:44:55" or "00-11-22-33-44-55" or "001122334455"?
}

// ✅ GOOD — stdlib type, consistent representation
type Device struct {
    MAC net.HardwareAddr
}
```

There is [a stdlib MAC type](https://pkg.go.dev/net#HardwareAddr). Use it. If the source is a `net.HardwareAddr` already, you don't have to parse it again — compare it with `bytes.Equal(a, b)` rather than `==` because `net.HardwareAddr` is a `[]byte` slice type. Comparing `string(a)`/`string(b)` or `a.String()` values can work, but be explicit about the normalization and formatting tradeoffs. If MAC comparison rules matter in multiple places, centralize them in a helper such as `EqualMAC(a, b net.HardwareAddr) bool`. Consolidate any special handling of MAC parsing into a single place.

## Custom Types Over String Typing

```go
// ❌ BAD — stringly typed, easy to mix up
func CreateNC(channelMode string, ncID string, nodeID string) error {
    // which string is which? typos compile fine
}

// ✅ GOOD — custom types prevent misuse
type ChannelMode string
const (
    ChannelModeDirect ChannelMode = "direct"
    ChannelModeManaged ChannelMode = "managed"
    ChannelModeCRD ChannelMode = "crd"
)

type NCID string
type NodeID string

func CreateNC(mode ChannelMode, ncID NCID, nodeID NodeID) error {
    // compiler prevents passing nodeID where ncID is expected
}
```

Stop kicking the can down the road — fix stringly-typed variables instead of adding more special handling for them.

## Struct Comparison

```go
// ❌ BAD — reflect.DeepEqual or cmp.Equal for comparable structs
if reflect.DeepEqual(oldState, newState) { ... }

// ✅ GOOD — if all fields are comparable, use ==
if oldState == newState { ... }
```

From the [Go spec](https://go.dev/ref/spec#Comparison_operators): "Struct types are comparable if all their field types are comparable."

**Never use `cmp.Equal` in production code.** From its [documentation](https://pkg.go.dev/github.com/google/go-cmp/cmp): "It is intended to only be used in tests, as performance is not a goal and it may panic if it cannot compare the values."

## Centralize Formatters

When an identifier has a canonical string format, implement it as a method on the type:

```go
// ❌ BAD — magic key prefixes scattered across codebase
key := fmt.Sprintf("NIC_%s_%s", deviceType, uid)
// ... elsewhere:
key := fmt.Sprintf("NIC-%s-%s", deviceType, uid) // inconsistent!

// ✅ GOOD — centralized as String()/Key()/ID() method
func (d Device) Key() string {
    return fmt.Sprintf("NIC_%s_%s", d.Type, d.UID)
}
```

These formatters MUST be centralized as `Key()`, `ID()`, or `String()` methods on the type so that they are _always_ used consistently.

## Typed State Enums and Predicate Filters

Define state values as typed string constants, then build reusable predicate functions. The same rule applies to public contract surfaces such as `Status`, `Condition`, `Reason`, and `ResponseCode` — if callers can observe it, model it as a named type with shared constants and helpers.

```go
// ❌ BAD — bare strings for state, ad-hoc filtering everywhere
if state == "Available" || state == "PendingProgramming" {
    // ...
}

// ✅ GOOD — typed enum + predicate function
type IPConfigState string

const (
    Available          IPConfigState = "Available"
    Allocated          IPConfigState = "Allocated"
    PendingRelease     IPConfigState = "PendingRelease"
    PendingProgramming IPConfigState = "PendingProgramming"
)

// Reusable filter predicate
func IsAvailableForAllocation(state IPConfigState) bool {
    return state == Available || state == PendingProgramming
}

// Used consistently across codebase
for _, ip := range pool {
    if IsAvailableForAllocation(ip.State) {
        available = append(available, ip)
    }
}
```

This moves state semantics into shared types. Consumers use predicates instead of repeating conditions with bare strings.

For public contract fields, typed constants and predicate helpers are mandatory. Never scatter raw `"Failed"`, `"Timeout"`, or `4091` literals through handlers, and never infer a public `Reason` or `ResponseCode` by matching `err.Error()`. Map internal errors to typed public values once with `errors.Is`/`errors.As`, then return that typed surface everywhere else.

```go
type Status string
type Condition string
type Reason string
type ResponseCode int

const (
    StatusSucceeded Status = "Succeeded"
    StatusFailed    Status = "Failed"

    ConditionReady    Condition = "Ready"
    ConditionDegraded Condition = "Degraded"

    ReasonNone         Reason = "None"
    ReasonInvalidInput Reason = "InvalidInput"

    ResponseCodeOK           ResponseCode = 0
    ResponseCodeInvalidInput ResponseCode = 4001
)

func IsFailureStatus(status Status) bool {
    return status == StatusFailed
}

type PublicResponse struct {
    Status       Status
    Condition    Condition
    Reason       Reason
    ResponseCode ResponseCode
}

func mapPublicResponse(err error) PublicResponse {
    switch {
    case errors.Is(err, ErrInvalidIP):
        return PublicResponse{
            Status:       StatusFailed,
            Condition:    ConditionDegraded,
            Reason:       ReasonInvalidInput,
            ResponseCode: ResponseCodeInvalidInput,
        }
    default:
        return PublicResponse{
            Status:       StatusSucceeded,
            Condition:    ConditionReady,
            Reason:       ReasonNone,
            ResponseCode: ResponseCodeOK,
        }
    }
}
```

## Common Mistakes

| Mistake | Fix |
| --- | --- |
| `net.IP` in new code | Use `netip.Addr` — value type, comparable, returns errors |
| `net.ParseIP` (returns nil) | Use `netip.ParseAddr` (returns error) |
| String MAC addresses | Use `net.HardwareAddr` |
| `channelMode string` parameter | Define `type ChannelMode string` with const values |
| `reflect.DeepEqual` on comparable structs | Use `==` directly |
| `cmp.Equal` in production | Only in tests — it panics by design |
| Format string duplicated in multiple places | Centralize as `Key()`/`ID()`/`String()` method |
| Re-parsing IP string in every function | Parse at boundary, pass `netip.Addr` through |
| Public `Status`/`Condition`/`Reason`/`ResponseCode` handled as raw literals | Define typed constants + predicate helpers, then map internal errors with `errors.Is`/`errors.As` instead of `err.Error()` matching |

## Cross-References

- → See `samber/cc-skills-golang@golang-safety` for nil interface trap and numeric safety
- → See `samber/cc-skills-golang@golang-naming` for type and constant naming conventions
- → See `acn-go-errors-logging` skill for embedding structured data in errors
- → See `acn-go-interfaces-dependencies` skill for type design at package boundaries
