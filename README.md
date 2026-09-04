# Agent Dependency Preflight

Agent Dependency Preflight is a read-only MCP server for checking the readiness of operator-approved service dependencies without giving an AI agent unrestricted shell or network access.

The operator owns the dependency catalog. MCP clients identify dependencies by logical name rather than supplying arbitrary network destinations such as hostnames, IP addresses, ports, URL schemes, HTTP paths, headers, credentials, request bodies, or redirect destinations.

The MVP is implemented with the official MCP Go SDK `v1.7.0` and stdio transport.

## Quick start

The recommended development workflow uses [`just`](https://github.com/casey/just) as a thin wrapper around the project's Go tooling.

### Windows

Install `just`:

```powershell
winget install --id Casey.Just --exact
```

Then, from the repository root:

```powershell
just setup-windows
just check-race
```

`just setup-windows` performs the complete Windows developer/reviewer setup: Go module setup, the MSYS2 UCRT64 GCC toolchain used by Go's race detector, and Node.js LTS plus `npx` for the MCP Inspector demo. Node.js is not required by the MCP server itself.

If Node.js is installed for the first time, open a new PowerShell session before running `just inspector-demo` so the parent shell picks up the updated `PATH`.

### macOS

```bash
brew install just
just setup
just check
```

### Other platforms

If `just` is already installed:

```bash
just setup
just check
```

Run `just --list` to see all project commands. `just` is optional; the underlying commands are standard Go tooling.

## Two-minute local demo

The demo uses only loopback networking. It does not depend on a public service.

The demo catalog contains three logical dependencies:

| Name | Expected observation |
| --- | --- |
| `demo_ready` | HTTP `/ready` returns `200`, so the dependency is `ready` |
| `demo_unhealthy` | HTTP `/unhealthy` returns `503`, so the dependency is `not_ready` at the HTTP stage |
| `demo_closed` | nothing listens on port `18081`, so the dependency is `not_ready` at the TCP stage |

An unknown name demonstrates the fail-closed catalog boundary and never reaches the network checker.

### 1. Start the deterministic upstream

In terminal 1:

```bash
just demo-upstream
```

Expected startup output:

```text
demo upstream listening on http://127.0.0.1:18080
  GET /ready     -> 200
  GET /unhealthy -> 503
```

### 2. Open the MCP Inspector

The reference MCP Inspector requires Node.js 22.19.0 or newer and runs through `npx` without a global Inspector install. On Windows, `just setup-windows` installs or updates Node.js LTS automatically. If you only need the Inspector prerequisite, run:

```powershell
just setup-inspector-windows
```

If Node.js was newly installed, open a new PowerShell session before continuing. Verify the Inspector runtime with:

```bash
just verify-inspector
```

Then, in terminal 2:

```bash
just inspector-demo
```

The Inspector will launch the MCP server over stdio with `examples/demo/catalog.json`.

From the Inspector:

1. call `list_dependencies`;
2. call `check_dependency` with `{"name":"demo_ready"}`;
3. call `check_dependency` with `{"name":"demo_unhealthy"}`;
4. call `check_dependency` with `{"name":"demo_closed"}`;
5. call `check_dependency` with `{"name":"not_configured"}`.

The first three configured checks are successful MCP tool executions even when the dependency is `not_ready`. The unknown logical name is a tool error because the request itself is outside the operator-approved catalog.

To run the stdio server directly for another MCP client:

```bash
just mcp-demo
```

The process then reserves stdout for MCP protocol traffic and writes diagnostics only to stderr.

## Architecture

```text
MCP client
    |
    | logical dependency name only
    v
internal/mcpserver
    |
    v
internal/preflight.Service
    |
    | exact catalog lookup
    +---- unknown name ----> tool error, no network access
    |
    v
validated catalog.Dependency
    |
    v
internal/preflight.Checker
    |
    +--> DNS
    +--> TCP
    +--> optional TLS
    +--> optional HTTP GET
```

Package responsibilities are intentionally narrow:

- `internal/catalog` owns strict parsing, validation, immutable lookup, and safe dependency summaries.
- `internal/preflight` owns the application service, layered readiness checks, result contracts, and safe error classification.
- `internal/mcpserver` is the only package coupled to the MCP SDK.
- `cmd/agent-dependency-preflight` owns process composition, configuration loading, signals, stderr logging, and stdio transport.
- `cmd/demo-upstream` is a deterministic reviewer fixture, not production checking logic.

## Security boundary

The trusted operator controls the catalog file and therefore the set of destinations the server may contact. The MCP caller controls only a logical dependency name.

The MCP tools do **not** accept caller-supplied:

- hostnames or IP addresses;
- ports;
- URL schemes or paths;
- HTTP methods, headers, credentials, or request bodies;
- redirect destinations;
- shell commands or SSH parameters.

Additional boundaries in the MVP:

- unknown logical names fail before network access;
- configuration is parsed once at startup with unknown JSON fields rejected;
- DNS, TCP, TLS, HTTP, and total execution have bounded deadlines;
- TCP uses addresses returned by the explicit DNS stage rather than performing a second hostname lookup;
- TLS performs normal certificate verification with the operating-system trust store and requires TLS 1.2 or newer;
- HTTP readiness uses a fixed `GET`, does not use environment proxies, does not follow redirects, does not send credentials or a body, limits response headers, and does not consume response bodies;
- public tool results contain safe categories rather than raw operating-system/network errors;
- stdout is reserved for MCP protocol traffic when running over stdio.

A result proves only what this process observed from this machine at the reported time. It does not claim a root cause beyond the stage and safe failure category actually observed.

## MCP tools

### `list_dependencies`

Input: empty object.

Returns the approved logical dependency names and descriptions. It deliberately does not expose configured network destinations.

Example structured result:

```json
{
  "dependencies": [
    {
      "name": "demo_ready",
      "description": "Local demo HTTP dependency that returns 200 from /ready."
    }
  ]
}
```

### `check_dependency`

Input:

```json
{
  "name": "demo_ready"
}
```

The input schema accepts only the logical name. Host, port, scheme, path, and all other network parameters remain operator-owned configuration.

A healthy dependency returns a normal tool result with `status: "ready"`.

An operational failure also returns a normal tool result, but with `status: "not_ready"`, the failed stage, stage-by-stage evidence, and a safe failure category. Later stages are marked `not_attempted` with a reason when an earlier stage fails.

An unknown dependency is different: it is a tool-level error with category `unknown_dependency`, because the requested name is not authorized by the catalog.

Both tools are annotated read-only, non-destructive, and idempotent. `check_dependency` is marked open-world because it observes external service state; `list_dependencies` is closed-world because it only reads the startup catalog snapshot.

## Dependency catalog

A catalog is strict JSON and is loaded once at startup. See [`examples/demo/catalog.json`](examples/demo/catalog.json) for a complete example.

```json
{
  "version": 1,
  "timeouts": {
    "total": "3s",
    "dns": "500ms",
    "tcp": "500ms",
    "tls": "1s",
    "http": "1s"
  },
  "dependencies": [
    {
      "name": "api",
      "description": "Application readiness endpoint",
      "protocol": "https",
      "host": "api.example.com",
      "port": 443,
      "http": {
        "path": "/ready",
        "accepted_statuses": [200, 204]
      }
    }
  ]
}
```

Supported protocols are `tcp`, `tls`, `http`, and `https`.

For `http` and `https`, the operator must configure a path and one or more accepted HTTP statuses. HTTP settings are rejected for `tcp` and `tls` dependencies. Invalid ports, hosts, paths, duplicate names/statuses, unsupported versions/protocols, missing timeouts, excessive timeouts, unknown fields, trailing JSON values, and oversized catalogs fail startup.

## Running the MCP server

Using `just`:

```bash
just mcp-demo
```

Using Go directly with your own catalog:

```bash
go run ./cmd/agent-dependency-preflight -config path/to/catalog.json
```

The server uses stdio transport. A real MCP client should launch the command rather than treating stdout as a human-readable console.

## Prerequisites

### Required

- [Go](https://go.dev/) 1.25.0 or newer
- [Git](https://git-scm.com/)

Development and final Windows validation used Go 1.27.1 on Windows/amd64.

### Recommended development tooling

- [`just`](https://github.com/casey/just) 1.56.0 or newer

The project is currently developed against `just` 1.58.0.

### Optional demo tooling

- Node.js 22.19.0 or newer for the current MCP Inspector

Node.js is only needed for the optional Inspector workflow; the Go MCP server itself does not require Node.js, npm, or npx.

On Windows, the complete setup installs or updates Node.js LTS automatically:

```powershell
just setup-windows
```

To install only the Inspector prerequisite:

```powershell
just setup-inspector-windows
```

Manual Windows equivalent:

```powershell
winget install --id OpenJS.NodeJS.LTS --exact
```

After a first-time Node.js installation, open a new PowerShell session, then verify:

```powershell
node --version
npx --version
```

### Windows race-detector tooling

The application itself does not require cgo or a C compiler. Go's race detector does.

The recommended Windows setup is:

```powershell
just setup-windows
```

This performs the standard Go module setup, installs/verifies MSYS2 UCRT64 GCC at the standard `C:\msys64` location, and installs/verifies Node.js LTS for the optional MCP Inspector. The project does not permanently modify global `CGO_ENABLED`; the race recipe supplies cgo and the GCC path only for that command.

## Setup without `just`

Download and verify modules:

```bash
go mod download
go mod verify
```

Build, vet, and test:

```bash
go build ./...
go vet ./...
go test -count=1 ./...
```

Format and normalize module metadata when developing:

```bash
go fmt ./...
go mod tidy
```

### Manual Windows race-detector setup

Install MSYS2:

```powershell
winget install --id MSYS2.MSYS2 --exact
```

Update it and install UCRT64 GCC:

```powershell
& 'C:\msys64\usr\bin\bash.exe' -lc 'pacman --noconfirm -Syuu'
& 'C:\msys64\usr\bin\bash.exe' -lc 'pacman --noconfirm -Syuu'
& 'C:\msys64\usr\bin\bash.exe' -lc 'pacman --noconfirm --needed -S mingw-w64-ucrt-x86_64-gcc'
```

Verify the compiler and Windows race runtime:

```powershell
& 'C:\msys64\ucrt64\bin\gcc.exe' --version
& 'C:\msys64\ucrt64\bin\gcc.exe' --print-file-name libsynchronization.a
```

The second command must return a full path ending in `libsynchronization.a`, not only the filename.

Then run:

```powershell
$env:CGO_ENABLED = "1"
$env:CC = "C:\msys64\ucrt64\bin\gcc.exe"
go test -race -count=1 ./...
```

## Development and validation

List the available recipes:

```bash
just --list
```

Common commands:

```bash
just fmt
just build
just vet
just test
just check
just check-race
just verify-inspector
just demo-upstream
just inspector-demo
```

`just check` runs:

```text
go fmt ./...
go mod tidy
go mod verify
go build ./...
go vet ./...
go test -count=1 ./...
```

`just check-race` adds:

```text
go test -race -count=1 ./...
```

Tests are deterministic and use local/in-memory fixtures. They do not require public network services. MCP integration tests use the official SDK's in-memory client/server transports so they exercise tool discovery, input validation, tool-error behavior, and structured output through the MCP layer.

## Current limitations

This take-home deliberately keeps the MVP narrow. It does not currently provide:

- Streamable HTTP transport;
- authentication or tenant isolation;
- rate limiting, concurrency limits, or load shedding;
- destination IP-class restrictions or a broader SSRF/rebinding policy for untrusted catalog authors;
- dynamic configuration reload;
- HTTP body-content assertions;
- remediation actions;
- shell, SSH, subnet scanning, or arbitrary port scanning;
- production tracing/metrics or distributed deployment support.

The stdio MVP assumes a trusted local operator controls the process and catalog. Those omissions should be addressed before exposing this server as a shared or remotely reachable service.

## LLM assistance

LLM assistance was used for product exploration, MCP SDK/specification research, implementation review, and test-case generation. Product scope, security boundaries, implementation decisions, code review, and validation remained human-reviewed. See [`DECISIONS.md`](DECISIONS.md) for additional design rationale.

## License

MIT
