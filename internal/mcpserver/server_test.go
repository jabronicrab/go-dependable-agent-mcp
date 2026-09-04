package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jabronicrab/go-dependable-agent-mcp/internal/catalog"
	"github.com/jabronicrab/go-dependable-agent-mcp/internal/preflight"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeChecker struct {
	mu    sync.Mutex
	calls []string
}

func (c *fakeChecker) Check(
	_ context.Context,
	dependency catalog.Dependency,
	_ catalog.Timeouts,
) preflight.Result {
	c.mu.Lock()
	c.calls = append(c.calls, dependency.Name)
	c.mu.Unlock()

	checkedAt := time.Date(2026, time.September, 4, 0, 0, 0, 0, time.UTC)
	if dependency.Name == "down" {
		return preflight.Result{
			Dependency:  dependency.Name,
			Status:      preflight.StatusNotReady,
			CheckedAt:   checkedAt,
			DurationMS:  1,
			FailedStage: preflight.StageTCP,
			Error: &preflight.Failure{
				Category: preflight.ErrorConnectionRefused,
				Message:  "TCP connection was refused",
			},
			Stages: []preflight.StageResult{
				{
					Name:   preflight.StageDNS,
					Status: preflight.StageNotApplicable,
					Reason: preflight.StageReasonLiteralIP,
				},
				{
					Name:       preflight.StageTCP,
					Status:     preflight.StageFailed,
					DurationMS: 1,
				},
				{
					Name:   preflight.StageTLS,
					Status: preflight.StageNotApplicable,
					Reason: preflight.StageReasonProtocolNotApplicable,
				},
				{
					Name:   preflight.StageHTTP,
					Status: preflight.StageNotApplicable,
					Reason: preflight.StageReasonProtocolNotApplicable,
				},
			},
		}
	}

	return preflight.Result{
		Dependency: dependency.Name,
		Status:     preflight.StatusReady,
		CheckedAt:  checkedAt,
		DurationMS: 1,
		Stages: []preflight.StageResult{
			{
				Name:   preflight.StageDNS,
				Status: preflight.StageNotApplicable,
				Reason: preflight.StageReasonLiteralIP,
			},
			{
				Name:       preflight.StageTCP,
				Status:     preflight.StagePassed,
				DurationMS: 1,
			},
			{
				Name:   preflight.StageTLS,
				Status: preflight.StageNotApplicable,
				Reason: preflight.StageReasonProtocolNotApplicable,
			},
			{
				Name:   preflight.StageHTTP,
				Status: preflight.StageNotApplicable,
				Reason: preflight.StageReasonProtocolNotApplicable,
			},
		},
	}
}

func (c *fakeChecker) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.calls)
}

func TestServerExposesExactlyTwoToolsWithSecurityAnnotations(t *testing.T) {
	ctx, session, _ := newTestSession(t)

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(result.Tools) != 2 {
		t.Fatalf("tool count = %d, want 2", len(result.Tools))
	}

	tools := make(map[string]*mcp.Tool, len(result.Tools))
	for _, tool := range result.Tools {
		tools[tool.Name] = tool
	}

	assertToolAnnotations(t, tools["list_dependencies"], false)
	assertToolAnnotations(t, tools["check_dependency"], true)
}

func TestToolOutputSchemasUsePortableSingleValueTypes(t *testing.T) {
	ctx, session, _ := newTestSession(t)

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}

	for _, tool := range result.Tools {
		if tool.OutputSchema == nil {
			t.Fatalf("tool %q has no output schema", tool.Name)
		}

		data, err := json.Marshal(tool.OutputSchema)
		if err != nil {
			t.Fatalf("json.Marshal(%s output schema) error = %v", tool.Name, err)
		}

		assertPortableSchemaTypes(t, "outputSchema", data)
	}
}

func TestListDependenciesReturnsOnlySafeSummaries(t *testing.T) {
	ctx, session, _ := newTestSession(t)

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "list_dependencies",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool(list_dependencies) error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool(list_dependencies) IsError = true; content = %q", firstText(t, result))
	}

	output := decodeStructured[listDependenciesOutput](t, result.StructuredContent)
	want := []catalog.Summary{
		{Name: "down", Description: "Unhealthy test dependency"},
		{Name: "ready", Description: "Healthy test dependency"},
	}
	if !reflect.DeepEqual(output.Dependencies, want) {
		t.Fatalf("dependencies = %#v, want %#v", output.Dependencies, want)
	}

	text := firstText(t, result)
	if strings.Contains(text, "127.0.0.1") || strings.Contains(text, "18080") {
		t.Fatalf("list_dependencies leaked network destination: %s", text)
	}
}

func TestCheckDependencyDistinguishesReadyAndNotReady(t *testing.T) {
	ctx, session, checker := newTestSession(t)

	readyResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "check_dependency",
		Arguments: map[string]any{
			"name": "ready",
		},
	})
	if err != nil {
		t.Fatalf("CallTool(check_dependency ready) error = %v", err)
	}
	if readyResult.IsError {
		t.Fatalf("ready check IsError = true; content = %q", firstText(t, readyResult))
	}

	ready := decodeStructured[checkDependencyOutput](t, readyResult.StructuredContent)
	if ready.Result == nil || ready.Result.Status != preflight.StatusReady || ready.Error != nil {
		t.Fatalf("ready output = %#v", ready)
	}

	downResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "check_dependency",
		Arguments: map[string]any{
			"name": "down",
		},
	})
	if err != nil {
		t.Fatalf("CallTool(check_dependency down) error = %v", err)
	}
	if downResult.IsError {
		t.Fatalf("operational not_ready result unexpectedly has IsError = true; content = %q", firstText(t, downResult))
	}

	down := decodeStructured[checkDependencyOutput](t, downResult.StructuredContent)
	if down.Result == nil || down.Result.Status != preflight.StatusNotReady {
		t.Fatalf("down output = %#v", down)
	}
	if down.Result.Error == nil || down.Result.Error.Category != preflight.ErrorConnectionRefused {
		t.Fatalf("down failure = %#v", down.Result.Error)
	}
	if checker.callCount() != 2 {
		t.Fatalf("checker calls = %d, want 2", checker.callCount())
	}
}

func TestCheckDependencyUnknownNameIsToolErrorWithoutNetworkCheck(t *testing.T) {
	ctx, session, checker := newTestSession(t)

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "check_dependency",
		Arguments: map[string]any{
			"name": "missing",
		},
	})
	if err != nil {
		t.Fatalf("CallTool(check_dependency missing) error = %v", err)
	}
	if !result.IsError {
		t.Fatal("unknown dependency IsError = false, want true")
	}

	output := decodeStructured[checkDependencyOutput](t, result.StructuredContent)
	if output.Result != nil || output.Error == nil {
		t.Fatalf("unknown dependency output = %#v", output)
	}
	if output.Error.Category != preflight.ErrorUnknownDependency {
		t.Fatalf("error category = %q, want %q", output.Error.Category, preflight.ErrorUnknownDependency)
	}
	if checker.callCount() != 0 {
		t.Fatalf("checker calls = %d, want 0", checker.callCount())
	}
}

func TestCheckDependencyRejectsMalformedNameBeforeNetworkCheck(t *testing.T) {
	ctx, session, checker := newTestSession(t)

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "check_dependency",
		Arguments: map[string]any{
			"name": "https://not-allowed.example",
		},
	})
	if err != nil {
		t.Fatalf("CallTool(check_dependency malformed) error = %v", err)
	}
	if !result.IsError {
		t.Fatal("malformed dependency name IsError = false, want true")
	}
	if checker.callCount() != 0 {
		t.Fatalf("checker calls = %d, want 0", checker.callCount())
	}
}

func newTestSession(t *testing.T) (context.Context, *mcp.ClientSession, *fakeChecker) {
	t.Helper()

	dependencyCatalog, err := catalog.Load(strings.NewReader(`{
		"version": 1,
		"timeouts": {
			"total": "2s",
			"dns": "500ms",
			"tcp": "500ms",
			"tls": "500ms",
			"http": "500ms"
		},
		"dependencies": [
			{
				"name": "ready",
				"description": "Healthy test dependency",
				"protocol": "tcp",
				"host": "127.0.0.1",
				"port": 18080
			},
			{
				"name": "down",
				"description": "Unhealthy test dependency",
				"protocol": "tcp",
				"host": "127.0.0.1",
				"port": 18081
			}
		]
	}`))
	if err != nil {
		t.Fatalf("catalog.Load() error = %v", err)
	}

	checker := &fakeChecker{}
	service := preflight.NewService(dependencyCatalog, checker)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := New(service, logger)

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}
	t.Cleanup(func() {
		_ = serverSession.Close()
	})

	client := mcp.NewClient(
		&mcp.Implementation{Name: "mcpserver-test-client", Version: "0.1.0"},
		&mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}},
	)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	t.Cleanup(func() {
		_ = clientSession.Close()
	})

	return ctx, clientSession, checker
}

func decodeStructured[T any](t *testing.T, value any) T {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal(structured content) error = %v", err)
	}

	var output T
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatalf("json.Unmarshal(structured content) error = %v; JSON = %s", err, data)
	}
	return output
}

func firstText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()

	if result == nil || len(result.Content) == 0 {
		t.Fatal("tool result has no content")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("first tool content is %T, want *mcp.TextContent", result.Content[0])
	}
	return text.Text
}

func assertPortableSchemaTypes(t *testing.T, path string, data []byte) {
	t.Helper()

	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", path, err)
	}

	walkPortableSchemaTypes(t, path, value)
}

func walkPortableSchemaTypes(t *testing.T, path string, value any) {
	t.Helper()

	switch node := value.(type) {
	case map[string]any:
		if typeValue, ok := node["type"]; ok {
			if _, isArray := typeValue.([]any); isArray {
				t.Fatalf("%s.type uses an array; use a single type or composition for MCP client portability", path)
			}
		}

		for key, child := range node {
			walkPortableSchemaTypes(t, path+"."+key, child)
		}

	case []any:
		for _, child := range node {
			walkPortableSchemaTypes(t, path, child)
		}
	}
}

func assertToolAnnotations(t *testing.T, tool *mcp.Tool, wantOpenWorld bool) {
	t.Helper()

	if tool == nil {
		t.Fatal("tool is missing")
	}
	annotations := tool.Annotations
	if annotations == nil {
		t.Fatalf("tool %q has no annotations", tool.Name)
	}
	if !annotations.ReadOnlyHint {
		t.Fatalf("tool %q ReadOnlyHint = false", tool.Name)
	}
	if !annotations.IdempotentHint {
		t.Fatalf("tool %q IdempotentHint = false", tool.Name)
	}
	if annotations.DestructiveHint == nil || *annotations.DestructiveHint {
		t.Fatalf("tool %q DestructiveHint = %v, want false", tool.Name, annotations.DestructiveHint)
	}
	if annotations.OpenWorldHint == nil || *annotations.OpenWorldHint != wantOpenWorld {
		t.Fatalf("tool %q OpenWorldHint = %v, want %t", tool.Name, annotations.OpenWorldHint, wantOpenWorld)
	}
}
