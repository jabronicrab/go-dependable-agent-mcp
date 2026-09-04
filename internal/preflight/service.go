package preflight

import (
	"context"
	"errors"

	"github.com/jabronicrab/go-dependable-agent-mcp/internal/catalog"
)

// ErrUnknownDependency identifies a logical dependency name that is not present
// in the operator-approved catalog.
var ErrUnknownDependency = errors.New("unknown dependency")

// DependencyChecker performs a readiness check for one already-approved dependency.
type DependencyChecker interface {
	Check(context.Context, catalog.Dependency, catalog.Timeouts) Result
}

// Service enforces catalog lookup before any dependency reaches the network checker.
type Service struct {
	catalog *catalog.Catalog
	checker DependencyChecker
}

// NewService constructs the application service used by MCP tool handlers.
func NewService(catalog *catalog.Catalog, checker DependencyChecker) *Service {
	return &Service{
		catalog: catalog,
		checker: checker,
	}
}

// ListDependencies returns the safe public summaries from the operator catalog.
func (s *Service) ListDependencies() []catalog.Summary {
	return s.catalog.List()
}

// CheckDependency resolves a logical name through the catalog before invoking
// the network checker. Unknown names fail closed and never reach the checker.
func (s *Service) CheckDependency(
	ctx context.Context,
	name string,
) (Result, error) {
	dependency, ok := s.catalog.Lookup(name)
	if !ok {
		return Result{}, ErrUnknownDependency
	}

	return s.checker.Check(
		ctx,
		dependency,
		s.catalog.Timeouts(),
	), nil
}
