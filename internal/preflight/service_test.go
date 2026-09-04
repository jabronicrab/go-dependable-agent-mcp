package preflight

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jabronicrab/go-dependable-agent-mcp/internal/catalog"
)

type recordingChecker struct {
	calls      int
	dependency catalog.Dependency
	timeouts   catalog.Timeouts
	result     Result
}

func (c *recordingChecker) Check(
	_ context.Context,
	dependency catalog.Dependency,
	timeouts catalog.Timeouts,
) Result {
	c.calls++
	c.dependency = dependency
	c.timeouts = timeouts

	return c.result
}

func TestServiceRejectsUnknownDependencyBeforeNetworkCheck(
	t *testing.T,
) {
	catalogValue := mustLoadServiceCatalog(t)

	checker := &recordingChecker{}

	service := NewService(
		catalogValue,
		checker,
	)

	_, err := service.CheckDependency(
		context.Background(),
		"not-approved",
	)

	if !errors.Is(err, ErrUnknownDependency) {
		t.Fatalf(
			"CheckDependency() error = %v, want ErrUnknownDependency",
			err,
		)
	}

	if checker.calls != 0 {
		t.Fatalf(
			"checker calls = %d, want 0",
			checker.calls,
		)
	}
}

func TestServicePassesApprovedDependencyAndTimeoutsToChecker(
	t *testing.T,
) {
	catalogValue := mustLoadServiceCatalog(t)

	wantDependency, ok :=
		catalogValue.Lookup("api")

	if !ok {
		t.Fatal("test catalog is missing api")
	}

	wantResult := Result{
		Dependency: "api",
		Status:     StatusReady,
	}

	checker := &recordingChecker{
		result: wantResult,
	}

	service := NewService(
		catalogValue,
		checker,
	)

	got, err := service.CheckDependency(
		context.Background(),
		"api",
	)

	if err != nil {
		t.Fatalf(
			"CheckDependency() error = %v",
			err,
		)
	}

	if checker.calls != 1 {
		t.Fatalf(
			"checker calls = %d, want 1",
			checker.calls,
		)
	}

	if !reflect.DeepEqual(
		checker.dependency,
		wantDependency,
	) {
		t.Fatalf(
			"checker dependency = %#v, want %#v",
			checker.dependency,
			wantDependency,
		)
	}

	if checker.timeouts !=
		catalogValue.Timeouts() {
		t.Fatalf(
			"checker timeouts = %#v, want %#v",
			checker.timeouts,
			catalogValue.Timeouts(),
		)
	}

	if !reflect.DeepEqual(got, wantResult) {
		t.Fatalf(
			"result = %#v, want %#v",
			got,
			wantResult,
		)
	}
}

func mustLoadServiceCatalog(
	t *testing.T,
) *catalog.Catalog {
	t.Helper()

	value, err := catalog.Load(
		strings.NewReader(`{
			"version": 1,
			"timeouts": {
				"total": "5s",
				"dns": "1s",
				"tcp": "1s",
				"tls": "2s",
				"http": "2s"
			},
			"dependencies": [{
				"name": "api",
				"description": "Application API",
				"protocol": "tcp",
				"host": "127.0.0.1",
				"port": 443
			}]
		}`),
	)

	if err != nil {
		t.Fatalf(
			"catalog.Load() error = %v",
			err,
		)
	}

	return value
}
