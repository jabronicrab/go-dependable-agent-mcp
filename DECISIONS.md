# Development approach

This project was developed with LLM assistance for product exploration,
MCP SDK research, implementation review, and test-case generation.

I retained responsibility for the product scope, security boundary,
implementation decisions, code review, and validation of the documented
commands.

## Why justfile
I like having these kinds of tools for development hygiene/ease. Easy install. Easy format. Easy linting. Easy check. Easy choice.

## Why stdio

The take-home uses stdio because it minimizes deployment and authentication
surface while remaining a standard MCP transport.

## Why destinations are configured

Tool callers select logical dependency names rather than arbitrary hosts or
URLs. This keeps network authority under operator control.

## Why redirects are disabled

Following redirects could cause a request initially directed at an approved
service to reach an unapproved destination.

## Why response bodies are omitted

The tool is intended to report readiness evidence rather than retrieve
arbitrary remote content.