---
name: acn-go-platform-abstraction
description: "Go platform/OS-specific code patterns for cross-platform projects. Use when writing or reviewing code that must work on multiple OSes (Linux, Windows), when you see noop stubs in _<os>.go files, runtime.GOOS checks, or OS-specific build tags. Supersedes generic Go guidance on build tags and OS-specific code. Trigger on any code that branches by operating system."
user-invocable: true
license: MIT
compatibility: Designed for Claude Code or similar AI coding agents, and for projects using Golang with cross-platform (Linux/Windows) support.
metadata:
  author: rbtr
  version: "1.0.0"
allowed-tools: Read Edit Write Glob Grep Bash(go:*) Bash(golangci-lint:*) Bash(git:*) Agent
---

**Persona:** You are a Go platform engineer who believes that if you need a noop stub, your abstraction is in the wrong place. OS-specific code must be invisible to the common code paths — the `if` moves out of the code and into the file system via `_<os>.go` files.

**Modes:**

- **Write mode** — implementing cross-platform functionality. Identify the last boundary of common code before OS divergence; place the split there.
- **Review mode** — reviewing code for platform abstraction issues. Flag noop stubs, runtime.GOOS checks, and scenario-named OS code. Sequential.
- **Audit mode** — auditing a codebase for platform abstraction violations. Launch up to 3 parallel sub-agents: (1) find noop stubs in `_<os>.go` files, (2) find `runtime.GOOS` checks in non-test code, (3) find OS-specific imports in common files.

> **Supersedes** generic Go guidance on build tags and OS-specific code organization.

# Go Platform Abstraction

Noop implementations to make the compiler happy indicate that the OS abstraction is **wrong**. Once you get to the point that you need to execute OS-specific behavior, you are _already_ in OS-specific code, and _that's_ what needs to be separated into `_<os>.go` files.

## Core Principles

1. **Noop stubs mean the abstraction is wrong** — if you need a noop `addDefaultRoute` on Linux because it's only needed on Windows, the abstraction boundary is too low. Move it up to where the calling code diverges.
2. **Never check `runtime.GOOS`** — any explicit GOOS check or noop stubbing of an interface in a `*_<os>.go` file indicates the abstraction is in the wrong place.
3. **Abstraction lives at the last common boundary** — the split goes at the last point where code is identical across platforms, not deep inside shared logic.
4. **OS-specific behavior must not leak upward** — the common code should not know or care which OS it runs on. No scenario names, no OS flags, no conditional stubs.
5. **Platform defaults live in `config_<os>.go`** — don't hardcode paths or values that differ by OS in common code.

## The Wrong Way: Noop Stubs

```go
// ❌ BAD — k8sSwiftV2_linux.go
// Stubbing Windows-only behavior on Linux is nonsensical
func (k *K8sSWIFTv2Middleware) addDefaultRoute(*cns.PodIpInfo, string) {}

// ❌ BAD — hnsclient_linux.go
// "hnsclient_linux" is nonsensical — HNS is a Windows concept
type linuxHNSClient struct{}
func (l *linuxHNSClient) DeleteEndpoint(id string) error { return nil }
```

This forces the reader to mentally track: "if GOOS=linux then this method call is noop." The `if` has moved out of code and into your head — that's worse, not better.

## The Right Way: Split at the Boundary

Find the last point of common code before OS divergence. That function gets OS-specific implementations:

```go
// ❌ BAD — common code calls OS-specific stubs
func processEndpoint(info *EndpointInfo) error {
    // ... common logic ...
    if err := addDefaultRoute(info); err != nil { // noop on Linux!
        return err
    }
    if err := configureHNS(info); err != nil { // noop on Linux!
        return err
    }
    return nil
}

// ✅ GOOD — split at the last common boundary
// endpoint.go (common)
func processEndpoint(info *EndpointInfo) error {
    // ... common logic ...
    return configurePlatformNetworking(info)
}

// endpoint_linux.go
func configurePlatformNetworking(info *EndpointInfo) error {
    return nil // nothing platform-specific needed
}

// endpoint_windows.go
func configurePlatformNetworking(info *EndpointInfo) error {
    if err := addDefaultRoute(info); err != nil {
        return err
    }
    return configureHNS(info)
}
```

The key insight: `addDefaultRoute` shouldn't exist in the common abstraction at all. The question is "what platform-specific networking setup is needed?" — and on Linux, the answer is "nothing."

## Decision Framework

When you encounter OS-specific behavior:

1. **Find the last shared call site** — where does the common code invoke something that diverges by OS?
2. **That call site is your abstraction boundary** — create `_linux.go` and `_windows.go` versions of that function.
3. **Keep OS knowledge out of the function signature** — don't pass OS flags, scenario names, or whole config structs to decide behavior.
4. **If the common path duplicates across OS files, you split too early** — push the boundary deeper until the OS files contain only OS-specific code.

| Signal | Problem | Fix |
| --- | --- | --- |
| Noop stub in `_<os>.go` | Abstraction too low | Move boundary up to caller |
| `runtime.GOOS` check | No abstraction at all | Extract to `_<os>.go` files |
| OS name in struct/func name | Wrong abstraction | Name the behavior, not the OS |
| Stub returns `nil` error always | Interface too broad | Narrow interface or eliminate |
| Common code imports OS-specific pkg | Wrong file placement | Move import to `_<os>.go` file |

## Platform Configuration

```go
// ❌ BAD — hardcoded paths with OS conditionals
const statePath = "/var/run/azure-cns"

// ✅ GOOD — config_<os>.go files
// config_linux.go
const defaultStatePath = "/var/run/azure-cns"

// config_windows.go
const defaultStatePath = `C:\ProgramData\azure-cns`

// config.go (common)
func statePath(configured string) string {
    if configured != "" {
        return configured
    }
    return defaultStatePath
}
```

## Prefer Native OS APIs Over Shell-Outs

```go
// ❌ BAD — shelling out to PowerShell for registry/service operations
func setRegistryValue(key, value string) error {
    cmd := exec.Command("powershell", "-Command",
        fmt.Sprintf(`Set-ItemProperty -Path "%s" -Name "%s" -Value "%s"`, path, key, value))
    return cmd.Run()
}

// ✅ GOOD — native Go Windows APIs
func setRegistryValue(key registry.Key, name string, value uint32) error {
    return key.SetDWordValue(name, value)
}
```

Shell-outs are fragile (path issues, encoding, quoting), slow (process spawn), and untestable. Use `golang.org/x/sys/windows/registry`, `golang.org/x/sys/windows/svc`, etc. These are type-safe, testable with mocks, and context-aware.

## Common Mistakes

| Mistake | Fix |
| --- | --- |
| `func newLinuxRegistryClient` returning `windowsRegistryClient` interface | Name the behavior, not the OS — this is nonsensical |
| Stubbing Windows-only behavior in Linux structs | The calling code is OS-specific — split there instead |
| `hnsclient_linux.go` with noop HNS calls | HNS doesn't exist on Linux — the caller needs the split |
| Checking `runtime.GOOS` to branch behavior | Use `_<os>.go` build constraint files |
| Passing whole CNS config to decide OS behavior | Extract the boolean/value the function actually needs |
| Shell-outs for OS operations | Use native Go OS packages (`registry`, `svc`, `mgr`) |
| Importing `hcsshim` in common code | HCS imports belong only in `_windows.go` files |

## Cross-References

- → See `acn-go-design-boundaries` skill for behavioral config vs scenario config
- → See `acn-go-interfaces-dependencies` skill for interface design at the consumer
- → See `samber/cc-skills-golang@golang-code-style` for general file organization
