# Dependable Agent MCP

[![Validate](https://github.com/jabronicrab/go-dependable-agent-mcp/actions/workflows/validate.yml/badge.svg)](https://github.com/jabronicrab/go-dependable-agent-mcp/actions/workflows/validate.yml)

Dependable Agent MCP is a read-only MCP server for checking the readiness of operator-approved service dependencies without giving an AI agent unrestricted shell or network access.

The operator owns the dependency catalog. MCP clients select dependencies by logical name rather than supplying arbitrary hostnames, IP addresses, ports, URL schemes, HTTP paths, headers, credentials, request bodies, or redirect destinations.

The MVP uses the official MCP Go SDK `v1.7.0` over stdio transport.

## Quick start

The project keeps the common developer workflow intentionally small:

```text
just setup    # prepare dependencies and local tooling
just check    # run the complete local validation suite
just run      # run the MCP server
just inspect  # open MCP Inspector against the server
```

Run `just --list` for the granular commands behind those four entry points.

### Windows

Install Go 1.26.0 or newer for the full development workflow and `just`, then run the project setup. The server module itself remains compatible with Go 1.25.0 or newer.

```powershell
winget install --id Casey.Just --exact
just setup
```

`just setup` downloads and verifies Go modules, prepares the pinned analysis tools used by `just check`, installs/verifies the MSYS2 UCRT64 GCC toolchain required by Go's Windows race detector, and installs/verifies Node.js LTS plus `npx` for MCP Inspector.

The Windows race setup asks for confirmation before installing or updating MSYS2. If Node.js is installed for the first time, open a new PowerShell session before running `just inspect` so the parent shell picks up the updated `PATH`.

Then validate the repository:

```powershell
just check
```

Run the server with the checked-in demo catalog:

```powershell
just run
```

Or launch the reference MCP Inspector against the same server configuration:

```powershell
just inspect
```

### macOS

On macOS, Homebrew provides the small bootstrap surface. Install Go and `just`, then let the project setup prepare the remaining tooling:

```bash
brew install go just
just setup
just check
```

`just setup` downloads and verifies the Go modules and pinned analysis tools, then installs or updates Homebrew Node.js when needed for MCP Inspector. The macOS Go race detector does **not** require a separate C compiler setup; `just check` still runs `go test -race -count=1 ./...` normally.

Run the server or Inspector with the same top-level commands:

```bash
just run
just inspect
```

### Other Unix-like systems

Install Go 1.26.0 or newer, `just`, Node.js 22.19.0 or newer, and the platform C toolchain required by Go's race detector. `just setup` verifies those prerequisites; it does not choose a system package manager outside macOS.

## Two-minute local demo

The demo uses only loopback networking and does not depend on a public service.

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

### 2. Open MCP Inspector

In terminal 2:

```bash
just inspect
```

The Inspector launches the MCP server over stdio with `examples/demo/catalog.json`.

From the Inspector:

1. call `list_dependencies`;
2. call `check_dependency` with `{"name":"demo_ready"}`;
3. call `check_dependency` with `{"name":"demo_unhealthy"}`;
4. call `check_dependency` with `{"name":"demo_closed"}`;
5. call `check_dependency` with `{"name":"not_configured"}`.

The three configured checks are successful MCP tool executions even when a dependency is `not_ready`. The unknown logical name is a tool error because the request itself is outside the operator-approved catalog.

## Core commands

### `just setup`

Prepares the repository and local reviewer tooling.

On Windows it:

- downloads and verifies project Go modules;
- downloads/verifies the pinned Staticcheck and govulncheck tool versions;
- installs or verifies MSYS2 UCRT64 GCC for `go test -race`;
- installs or verifies Node.js LTS and `npx` for MCP Inspector.

On macOS it downloads/verifies Go tooling, installs or updates Homebrew Node.js when needed, verifies `npx`, and confirms that no separate C compiler setup is required for the Darwin race detector.

On other Unix-like systems it downloads/verifies Go tooling and verifies the Node.js/npx and C-toolchain prerequisites used by Inspector and race testing.

### `just check`

Runs the complete local validation suite:

```text
go fmt ./...
go mod tidy
go mod verify
go build ./...
go vet ./...
go test -count=1 ./...
go test -race -count=1 ./...
go test -count=10 ./...
staticcheck ./...
govulncheck ./...
```

The analysis tools are invoked through pinned `go run ...@version` commands, so they do not need global installs.

`just check` intentionally does **not** inspect Git working-tree state. It answers whether the checked-out code formats, builds, analyzes, tests, race-tests, and passes the vulnerability scan. Patch/working-tree review remains a separate Git concern.

`go fmt` and `go mod tidy` can modify files when normalization is needed; review those changes before committing.

### `just run`

Runs the stdio MCP server with the demo catalog by default:

```bash
just run
```

Supply another catalog when needed:

```bash
just run path/to/catalog.json
```

Equivalent Go command:

```bash
go run ./cmd/agent-dependency-preflight -config path/to/catalog.json
```

The process reserves stdout for MCP protocol traffic and writes diagnostics only to stderr.

### `just inspect`

Launches the reference MCP Inspector against the server and demo catalog:

```bash
just inspect
```

Supply another catalog when needed:

```bash
just inspect path/to/catalog.json
```

Node.js is required only for the Inspector workflow, not for the Go MCP server itself.

## Granular commands

The top-level commands are intended for normal development. Individual recipes remain available for debugging or CI parity:

```text
just setup-go
just setup-tools
just setup-race-windows
just setup-inspector-windows
just setup-race-unix
just setup-inspector-unix
just verify-inspector
just fmt
just tidy
just verify
just build
just vet
just test
just test-repeat
just test-race
just staticcheck
just vuln
just demo-upstream
```

Run `just --list` for the authoritative list.

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

Returns approved logical dependency names and descriptions. It deliberately does not expose configured network destinations.

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

An operational failure also returns a normal tool result, but with `status: "not_ready"`, the failed stage, stage-by-stage evidence, and a safe failure category. Later stages are marked `not_attempted` with an explicit reason when an earlier stage fails.

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

## Prerequisites

### Required

- [Go](https://go.dev/) 1.25.0 or newer to build and run the module
- Go 1.26.0 or newer for the full recommended `just setup` / `just check` development workflow, because the pinned Staticcheck release requires Go 1.26
- [Git](https://git-scm.com/)
- [`just`](https://github.com/casey/just) 1.56.0 or newer for the recommended workflow

Development and final Windows validation used Go 1.27.1 on Windows/amd64 and `just` 1.58.0.

### Inspector

The pinned MCP Inspector 2.5.0 requires Node.js 22.19.0 or newer. Node.js is only needed for `just inspect`; the Go MCP server itself does not require Node.js, npm, or npx.

On Windows, `just setup` installs or updates Node.js LTS automatically. On macOS, `just setup` installs or updates the Homebrew `node` package when needed.

To work on only the Windows prerequisite:

```powershell
just setup-inspector-windows
```

Manual Windows equivalent:

```powershell
winget install --id OpenJS.NodeJS.LTS --exact
```

After a first-time Windows Node.js installation, open a new PowerShell session before invoking `just inspect`.

On macOS, the equivalent granular setup command is:

```bash
just setup-inspector-unix
```

### Race detector

On macOS, Go's Darwin race detector runs directly with:

```bash
go test -race -count=1 ./...
```

No separate C compiler installation or cgo override is required for the macOS race test.

On Windows, the application itself does not require cgo or a C compiler, but Go's Windows race detector does. `just setup` installs/verifies MSYS2 UCRT64 GCC at the standard `C:\msys64` location. The project does not permanently modify global `CGO_ENABLED`; the Windows `test-race` recipe supplies cgo and the compiler path only for that command.

To prepare only the Windows race toolchain:

```powershell
just setup-race-windows
```

On other Unix-like systems, `just setup-race-unix` verifies the C compiler used by the race test.

## Setup without `just`

The task runner contains no application logic. The server can be built and tested directly with standard Go tooling:

```bash
go mod download
go mod verify
go fmt ./...
go mod tidy
go build ./...
go vet ./...
go test -count=1 ./...
```

Run the race detector directly on macOS:

```bash
go test -race -count=1 ./...
```

On non-Darwin platforms where the race detector requires cgo, ensure the platform C compiler is installed first. The `just` recipes handle the Windows compiler configuration and verify the Unix prerequisite.

The additional analysis commands used by `just check` are:

```bash
go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...
```

## Testing

Tests are deterministic and use local/in-memory fixtures. They do not require public network services.

MCP integration tests use the official SDK's in-memory client/server transports so they exercise tool discovery, strict input validation, tool-error behavior, portable output schemas, and structured output through the MCP layer.

The checked-in demo catalog is also loaded through the real catalog validator in tests so the reviewer fixture cannot silently drift into an invalid configuration.

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
