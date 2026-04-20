---
name: acn-go-http-api-contracts
description: "HTTP and REST contract design for Azure/azure-container-networking. Use when designing or reviewing HTTP handlers, routes, request/response types, query parameters, and action semantics. Trigger on GET handlers with bodies, PUT vs PATCH questions, action-heavy URL designs, redundant wrapper responses, or APIs that split a single resource operation across mismatched request shapes."
user-invocable: true
license: MIT
compatibility: Designed for Claude Code or similar AI coding agents, and for projects using Golang with HTTP APIs.
metadata:
  author: rbtr
  version: "1.0.0"
allowed-tools: Read Edit Write Glob Grep Bash(go:*) Bash(golangci-lint:*) Bash(git:*) Agent
---

**Persona:** You are a Go API reviewer who treats HTTP shape as contract design, not routing trivia. A resource path, method, and payload must describe the same operation. If the URL says one thing and the body says another, the API is wrong.

**Modes:**

- **Write mode** — designing new HTTP APIs or reshaping handlers. Prefer resource-shaped URLs, typed request/response bodies, and method semantics that match the operation.
- **Review mode** — reviewing handlers, route wiring, and request/response contracts. Flag GET bodies, action-shaped URLs, redundant wrapper responses, and operations whose semantics belong in a different method or path.
- **Audit mode** — auditing an HTTP surface. Split work across: (1) URL/resource shape, (2) method/idempotency, (3) request/response contract clarity.

> **Repo-specific skill.** This captures repeated HTTP/API guidance from rbtr's ACN review history.

# Go HTTP/API Contracts

HTTP method, URL, query parameters, and body must all describe the same resource operation. Do not force callers to reverse-engineer your intent from mismatched pieces.

## Use when

- adding or reviewing HTTP handlers or routes
- choosing between GET / POST / PUT / PATCH
- designing request/response structs
- deciding whether something belongs in path, query, or body
- simplifying REST/controller handler surfaces

## Do not use when

- the change is internal-only and not exposed through HTTP
- the issue is only transport/client plumbing, not API contract shape

## Core Principles

1. **GET has no body** — use query parameters for filtering and selection.
2. **Choose PUT vs PATCH by semantics** — PUT replaces a resource representation; PATCH mutates part of it; POST performs creation or action semantics.
3. **Use resource-shaped URLs** — the path should identify the resource being operated on, not smuggle domain actions into arbitrary endpoints.
4. **Prefer HTTP status over extra app error-code layers** when the HTTP contract already expresses the state.
5. **One typed request/response surface per operation** — avoid redundant wrapper responses and mismatched body/path semantics.

## No GET Bodies

```go
// ❌ BAD — GET with body
GET /ibdevices
{
  "state": "assigned"
}

// ✅ GOOD — GET with query filters
GET /ibdevices?state=assigned
GET /ibdevices?state=unassigned
GET /ibdevices?by-pod=namespace%2Fname
```

GET retrieves a representation. If the caller is selecting or filtering, use query parameters.

## PUT vs PATCH vs POST

Choose the method based on what the operation means:

| Method | Use for | Example |
| --- | --- | --- |
| **GET** | read a resource or collection | `GET /ibdevices?state=assigned` |
| **PUT** | replace a full resource representation, idempotently | `PUT /nodes/{id}/status` |
| **PATCH** | partial mutation of an existing resource | `PATCH /pods/{id}` |
| **POST** | create a resource or invoke an action endpoint | `POST /ibdevices:assign?...` |

```go
// ❌ BAD — POST on a resource path that does not mean create/replace
POST /ibdevices/{id}
{
  "pod": "ns/name"
}

// ✅ GOOD — action is explicit when it is not whole-resource replacement
POST /ibdevices:assign?ibdevs=mac1,mac2&pod=ns%2Fname
```

If POSTing to `/resource/{id}` does **not** create or replace the whole resource, the API is lying about its contract.

## Resource-Shaped URLs

The path should identify the thing being operated on.

```go
// ❌ BAD — path and payload describe different resources
POST /ibdevices/{mac}
{
  "pod": "ns/name",
  "devices": ["mac1", "mac2"]
}

// ✅ GOOD — collection plus filter/action surfaces
GET  /ibdevices/{mac}
GET  /ibdevices?state=assigned
GET  /ibdevices?by-pod=ns%2Fname
POST /ibdevices:assign?ibdevs=mac1,mac2&pod=ns%2Fname
```

If the body is really about a mapping between resources, model the mapping explicitly. Do not pretend the request is “about” one thing while the payload mutates another.

## Path vs Query vs Body

Use each part of the request for what it is good at:

- **Path** — identifies the resource
- **Query** — selects, filters, or scopes the operation
- **Body** — supplies representation or mutation payload when the method semantics require one

```go
// ✅ GOOD — query carries the selection set
POST /ibdevices:assign?ibdevs=mac1,mac2&pod=ns%2Fname

// ✅ GOOD — body carries the representation being replaced
PUT /nodenetworkconfigs/{name}
{
  "spec": { ... }
}
```

Do not require a body when the input is just selectors that fit naturally in query params.

## Prefer HTTP Status Over Redundant App Error Layers

When HTTP already expresses the error class, use it.

```go
// ❌ BAD — HTTP 200 plus app-layer errorCode for a missing resource
{
  "errorCode": "DeviceNotFound",
  "message": "device not found"
}

// ✅ GOOD — HTTP status carries the contract
HTTP/1.1 404 Not Found
{
  "message": "device not found"
}
```

`409 Conflict`, `404 Not Found`, `400 Bad Request`, and friends already communicate most API contract failures. Avoid layering an extra parallel contract unless it is truly required across transports.

## Simplify Handler Surfaces

Handlers should read like: decode → validate → operate → encode.

```go
// ✅ GOOD — typed request/response surface per operation
type AssignIBDevicesRequest struct {
    Pod     string   `json:"pod"`
    Devices []string `json:"devices"`
}

func (s *Server) assignIBDevices(w http.ResponseWriter, r *http.Request) {
    var req AssignIBDevicesRequest
    if err := decode(r, &req); err != nil {
        http.Error(w, "invalid request", http.StatusBadRequest)
        return
    }
    // validate -> operate -> encode
}
```

Avoid broad pass-through wrappers or responses that exist only to restate HTTP status in another format.

## Common Mistakes

| Mistake | Fix |
| --- | --- |
| GET request with a body | Use query parameters |
| POST on `/resource/{id}` for a non-create/non-replace action | Use action endpoint or the correct method |
| URL path identifies one resource while body mutates another | Model the actual resource/mapping explicitly |
| Query-worthy selectors placed in JSON body | Put selectors in query params |
| Returning HTTP 200 with app-layer not-found/conflict code | Use HTTP 404/409/etc. |
| Handler response wrapped only to repeat status | Return one typed response shape or plain HTTP error |
| Splitting one logical operation across mismatched path/body shapes | Design one coherent typed request/response surface |

## Cross-References

- → See `acn-go-control-plane-contracts` for typed public response/status contracts and CRD/API surface hygiene
- → See `acn-go-interfaces-dependencies` for simplifying handler surfaces and migration shims
- → See `acn-go-errors-logging` for boundary error handling and concise API-facing messages
- → See `acn-go-design-boundaries` for keeping business behavior out of transport-specific plumbing
