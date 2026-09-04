# Agent Dependency Preflight

Agent Dependency Preflight is a read-only MCP server for checking the readiness of operator-approved service dependencies without giving an AI agent unrestricted shell or network access.

The operator owns the dependency catalog. MCP clients identify dependencies by logical name rather than supplying arbitrary network destinations such as hostnames, IP addresses, ports, URL schemes, HTTP paths, headers, credentials, request bodies, or redirect destinations.

> **Status:** implementation in progress. The dependency catalog and readiness result contracts are implemented. The network readiness checker and MCP server will be added in subsequent milestones.

## Quick start

The recommended development workflow uses [`just`](https://github.com/casey/just) as a thin wrapper around the project's Go tooling.

### Windows

Install `just`:

```powershell
winget install --id Casey.Just --exact
```

Verify it:

```powershell
just --version
```

Then, from the repository root:

```powershell
just setup-windows
just check-race
```

`just setup-windows` performs the normal Go project setup and installs/verifies the Windows toolchain required by Go's race detector.

`just check-race` runs the normal validation suite followed by the race-enabled test suite.

### macOS

Install `just`:

```bash
brew install just
```

Then:

```bash
just setup
just check
```

### Other platforms

If `just` is already installed:

```bash
just setup
just check
```

Run:

```bash
just --list
```

to see all available project commands.

`just` is optional. Every project operation can also be run directly with standard Go tooling.

## Prerequisites

### Required

* [Go](https://go.dev/) 1.25.0 or newer
* [Git](https://git-scm.com/)

Development is currently using Go 1.27.1 on Windows/amd64.

### Recommended development tooling

* [`just`](https://github.com/casey/just) 1.56.0 or newer

The project is currently developed against `just` 1.58.0.

`just` provides convenient names for the project's standard setup, formatting, build, test, static-analysis, and validation commands. It does not contain application logic.

### Windows race-detector tooling

The application itself does not require cgo or a C compiler.

Go's race detector does require cgo and, on Windows, a compatible C compiler. The Windows development workflow uses MSYS2 UCRT64 GCC for this purpose.

The recommended setup:

```powershell
just setup-windows
```

installs and verifies this toolchain automatically.

## Setup

### Clone the repository

```bash
git clone https://github.com/jabronicrab/go-dependable-agent-mcp.git
cd go-dependable-agent-mcp
```

### Standard setup with `just`

On any supported platform:

```bash
just setup
```

This runs:

```text
go mod download
go mod verify
```

Run the standard validation suite with:

```bash
just check
```

### Complete Windows setup with race detection

On Windows:

```powershell
just setup-windows
```

The command performs the standard Go setup and then asks before installing or updating the Windows race-detector toolchain.

It:

1. verifies that Windows Package Manager (`winget`) is available;
2. installs MSYS2 if necessary;
3. updates the MSYS2 installation;
4. installs the MSYS2 UCRT64 GCC compiler;
5. verifies the compiler;
6. verifies that the compiler provides the runtime required by Go's Windows race detector.

The setup expects the standard MSYS2 installation location:

```text
C:\msys64
```

The project does not permanently add GCC to the Windows `PATH`.

When race-enabled tests are run through `just`, the `justfile` supplies the compiler explicitly and enables cgo only for that recipe.

After setup:

```powershell
just test-race
```

or run the complete validation suite:

```powershell
just check-race
```

## Manual setup without `just`

`just` is only a convenience wrapper. The project can be built and validated entirely with standard Go tooling.

Verify Go:

```bash
go version
```

Download dependencies:

```bash
go mod download
```

Verify downloaded modules:

```bash
go mod verify
```

Format the Go packages:

```bash
go fmt ./...
```

Normalize `go.mod` and `go.sum`:

```bash
go mod tidy
```

Build all packages:

```bash
go build ./...
```

Run Go's static analysis:

```bash
go vet ./...
```

Run the test suite without cached test results:

```bash
go test -count=1 ./...
```

## Manual Windows race-detector setup

The steps below are the manual equivalent of:

```powershell
just setup-windows
```

Install MSYS2:

```powershell
winget install --id MSYS2.MSYS2 --exact
```

Update MSYS2:

```powershell
& 'C:\msys64\usr\bin\bash.exe' -lc 'pacman --noconfirm -Syuu'
& 'C:\msys64\usr\bin\bash.exe' -lc 'pacman --noconfirm -Syuu'
```

Install the UCRT64 GCC compiler:

```powershell
& 'C:\msys64\usr\bin\bash.exe' -lc 'pacman --noconfirm --needed -S mingw-w64-ucrt-x86_64-gcc'
```

Verify GCC:

```powershell
& 'C:\msys64\ucrt64\bin\gcc.exe' --version
```

Verify that the runtime required by the Go race detector is available:

```powershell
& 'C:\msys64\ucrt64\bin\gcc.exe' --print-file-name libsynchronization.a
```

The second command should return a full path ending in:

```text
libsynchronization.a
```

Returning only the filename indicates that the compiler does not satisfy the Go race detector's Windows requirements.

Run the race-enabled tests:

```powershell
$env:CGO_ENABLED = "1"
$env:CC = "C:\msys64\ucrt64\bin\gcc.exe"

go test -race -count=1 ./...
```

These environment variables apply only to the current PowerShell session.

## Development

The `justfile` provides shortcuts for the common development and validation commands.

List available recipes:

```bash
just --list
```

### Format

```bash
just fmt
```

Equivalent command:

```bash
go fmt ./...
```

### Build

```bash
just build
```

Equivalent command:

```bash
go build ./...
```

### Test

```bash
just test
```

Equivalent command:

```bash
go test -count=1 ./...
```

### Static analysis

```bash
just vet
```

Equivalent command:

```bash
go vet ./...
```

### Standard validation

```bash
just check
```

This runs:

```text
go fmt ./...
go mod tidy
go mod verify
go build ./...
go vet ./...
go test -count=1 ./...
```

`go fmt` and `go mod tidy` may modify files. Review any resulting working-tree changes before committing them.

### Race-enabled validation

```bash
just check-race
```

This runs the standard validation suite followed by:

```text
go test -race -count=1 ./...
```

On Windows, the race recipe enables cgo and points Go at the MSYS2 UCRT64 GCC compiler without permanently changing the machine's Go environment or `PATH`.

## Planned MCP surface

The MVP will expose two read-only MCP tools:

* `list_dependencies` returns approved logical dependency names and descriptions.
* `check_dependency` checks one approved dependency through its configured DNS, TCP, optional TLS, and optional HTTP stages.

Unknown logical dependency names fail closed and are never interpreted as hostnames or URLs.

## License

MIT
