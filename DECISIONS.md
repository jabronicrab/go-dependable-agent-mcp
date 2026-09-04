# Development approach

This project was developed with LLM assistance for product exploration,
MCP SDK research, implementation review, and test-case generation.

I retained responsibility for the product scope, security boundary,
implementation decisions, code review, and validation of the documented
commands.

## Why the official MCP Go SDK

The server uses the official `github.com/modelcontextprotocol/go-sdk` module
rather than a third-party MCP implementation. The SDK boundary is isolated in
`internal/mcpserver` so protocol churn does not leak into catalog or readiness
logic.

## Why justfile

The `justfile` is a thin, optional wrapper around standard Go commands. It keeps
setup, formatting, validation, race testing, demo, and Inspector commands easy
to discover without moving application logic into task-runner recipes.

## Why stdio

The take-home uses stdio because it minimizes deployment and authentication
surface while remaining a standard MCP transport. Stdout is reserved for MCP
protocol traffic; human diagnostics go to stderr.

## Why destinations are configured

Tool callers select logical dependency names rather than arbitrary hosts or
URLs. This keeps network authority under operator control and prevents the MCP
caller from turning the tool into a generic network probe.

## Why catalog lookup precedes network access

`preflight.Service` resolves the caller-supplied logical name before invoking
the checker. Unknown names therefore fail closed and can be tested to prove that
no network operation occurs.

## Why DNS is explicit before TCP

For hostnames, the checker records DNS as its own stage and then dials the
returned IP addresses directly. This keeps DNS evidence distinct from TCP
evidence and avoids an implicit second hostname lookup during the TCP stage.
TLS and HTTP still use the configured hostname for certificate verification and
HTTP host semantics.

## Why redirects are disabled

Following redirects could cause a request initially directed at an approved
service to reach an unapproved destination.

## Why response bodies are omitted

The tool is intended to report readiness evidence rather than retrieve
arbitrary remote content. HTTP readiness therefore uses status and connection
evidence without reading the response body.

## Why public errors are sanitized

Raw resolver, socket, TLS, and HTTP errors may expose unnecessary internal
network details and are not stable API contracts. Callers receive a bounded set
of safe categories and messages, while the original cause remains available
internally for stderr diagnostics.

## Why dependency failures are not MCP tool errors

A configured dependency being unavailable is an expected observation, so the
MCP call succeeds with `status: "not_ready"`. Invalid or unauthorized tool input,
such as an unknown logical dependency name, is a tool error because the request
itself cannot be executed within the configured authority boundary.
