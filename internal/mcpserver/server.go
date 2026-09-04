package mcpserver

import (
	"context"
	"errors"
	"io"
	"log/slog"

	"github.com/jabronicrab/go-dependable-agent-mcp/internal/catalog"
	"github.com/jabronicrab/go-dependable-agent-mcp/internal/preflight"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	serverName    = "agent-dependency-preflight"
	serverVersion = "0.1.0"
)

type adapter struct {
	service *preflight.Service
	logger  *slog.Logger
}

type listDependenciesInput struct{}

type listDependenciesOutput struct {
	Dependencies []catalog.Summary `json:"dependencies" jsonschema:"operator-approved logical dependencies"`
}

type checkDependencyInput struct {
	Name string `json:"name" jsonschema:"logical dependency name returned by list_dependencies"`
}

type toolFailure struct {
	Category preflight.ErrorCategory `json:"category" jsonschema:"stable failure category"`
	Message  string                  `json:"message" jsonschema:"safe caller-facing failure message"`
}

type checkDependencyOutput struct {
	Result *preflight.Result `json:"result,omitempty" jsonschema:"readiness observation for the configured dependency"`
	Error  *toolFailure      `json:"error,omitempty" jsonschema:"tool-level failure when the request cannot be executed"`
}

// New constructs the MCP adapter around the package-owned preflight service.
func New(service *preflight.Service, logger *slog.Logger) *mcp.Server {
	if service == nil {
		panic("mcpserver.New: nil preflight service")
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	a := &adapter{
		service: service,
		logger:  logger,
	}

	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    serverName,
			Version: serverVersion,
		},
		&mcp.ServerOptions{
			Instructions: "Use list_dependencies to discover operator-approved logical names, then use check_dependency with one of those names. Callers cannot supply network destinations.",
			Logger:       logger,
			Capabilities: &mcp.ServerCapabilities{},
		},
	)

	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        "list_dependencies",
			Description: "List operator-approved logical dependency names and descriptions without exposing their network destinations.",
			Annotations: readOnlyAnnotations(false),
		},
		a.listDependencies,
	)

	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        "check_dependency",
			Description: "Check readiness of one operator-approved dependency selected only by logical name.",
			InputSchema: checkDependencyInputSchema(),
			Annotations: readOnlyAnnotations(true),
		},
		a.checkDependency,
	)

	return server
}

func (a *adapter) listDependencies(
	_ context.Context,
	_ *mcp.CallToolRequest,
	_ listDependenciesInput,
) (*mcp.CallToolResult, listDependenciesOutput, error) {
	return nil, listDependenciesOutput{
		Dependencies: a.service.ListDependencies(),
	}, nil
}

func (a *adapter) checkDependency(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input checkDependencyInput,
) (*mcp.CallToolResult, checkDependencyOutput, error) {
	result, err := a.service.CheckDependency(ctx, input.Name)
	if err != nil {
		if errors.Is(err, preflight.ErrUnknownDependency) {
			return &mcp.CallToolResult{IsError: true}, checkDependencyOutput{
				Error: &toolFailure{
					Category: preflight.ErrorUnknownDependency,
					Message:  "dependency is not configured; call list_dependencies to see approved names",
				},
			}, nil
		}

		a.logger.Error("dependency tool failed", "error", err)
		return &mcp.CallToolResult{IsError: true}, checkDependencyOutput{
			Error: &toolFailure{
				Category: preflight.ErrorInternal,
				Message:  "dependency check could not be executed",
			},
		}, nil
	}

	if cause := result.Cause(); cause != nil {
		category := preflight.ErrorInternal
		if result.Error != nil {
			category = result.Error.Category
		}
		a.logger.Debug(
			"dependency not ready",
			"dependency", result.Dependency,
			"category", category,
			"cause", cause,
		)
	}

	return nil, checkDependencyOutput{
		Result: &result,
	}, nil
}

func readOnlyAnnotations(openWorld bool) *mcp.ToolAnnotations {
	destructive := false
	return &mcp.ToolAnnotations{
		DestructiveHint: &destructive,
		IdempotentHint:  true,
		OpenWorldHint:   &openWorld,
		ReadOnlyHint:    true,
	}
}

func checkDependencyInputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Operator-approved logical dependency name returned by list_dependencies.",
				"pattern":     `^[a-z][a-z0-9_-]{0,63}$`,
			},
		},
		"required": []string{"name"},
	}
}
