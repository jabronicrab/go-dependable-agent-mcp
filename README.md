# Agent Dependency Preflight

Agent Dependency Preflight is a read-only MCP server for checking the readiness of operator-approved service dependencies without giving an AI agent unrestricted shell or network access.

The operator owns the dependency catalog. MCP clients will select only a logical dependency name; they will not be allowed to provide arbitrary hostnames, IP addresses, ports, URL schemes, HTTP paths, headers, credentials, request bodies, or redirect destinations.

> **Status:** implementation in progress. The current milestone establishes the Go module and developer tooling. The dependency catalog, readiness checker, and MCP server will be added in subsequent milestones.

## Prerequisites

- Go 1.25.0 or newer. Development is currently using Go 1.27.1 on Windows/amd64.
- Optional: `just` 1.56.0 or newer for project command shortcuts. The project was designed against `just` 1.58.0.

On Windows, `just` can be installed with Windows Package Manager:

```powershell
winget install --id Casey.Just --exact
just --version
```

On Mac, `just` can be installed with homebred:

```bash
brew install just
```

## Setup
