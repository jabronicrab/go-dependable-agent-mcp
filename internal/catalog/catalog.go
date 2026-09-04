package catalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"
)

const (
	// CurrentVersion is the only catalog schema version understood by this build.
	CurrentVersion = 1
	maxConfigBytes = 1 << 20
)

// ErrInvalidConfiguration identifies configuration content that is unsafe or unsupported.
var ErrInvalidConfiguration = errors.New("invalid configuration")

// Protocol determines which readiness stages apply to a dependency.
type Protocol string

const (
	ProtocolTCP   Protocol = "tcp"
	ProtocolTLS   Protocol = "tls"
	ProtocolHTTP  Protocol = "http"
	ProtocolHTTPS Protocol = "https"
)

// Timeouts contains the total request deadline and per-stage deadline ceilings.
type Timeouts struct {
	Total time.Duration
	DNS   time.Duration
	TCP   time.Duration
	TLS   time.Duration
	HTTP  time.Duration
}

// HTTPCheck contains the fixed HTTP request policy owned by the operator.
type HTTPCheck struct {
	Path             string
	AcceptedStatuses []int
}

// Dependency is a validated operator-approved network destination.
type Dependency struct {
	Name        string
	Description string
	Protocol    Protocol
	Host        string
	Port        uint16
	HTTP        *HTTPCheck
}

// Summary is the safe subset of dependency metadata exposed by list_dependencies.
type Summary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Catalog is an immutable, validated snapshot of dependency configuration.
type Catalog struct {
	timeouts     Timeouts
	dependencies map[string]Dependency
	summaries    []Summary
}

type rawConfig struct {
	Version      int             `json:"version"`
	Timeouts     rawTimeouts     `json:"timeouts"`
	Dependencies []rawDependency `json:"dependencies"`
}

type rawTimeouts struct {
	Total string `json:"total"`
	DNS   string `json:"dns"`
	TCP   string `json:"tcp"`
	TLS   string `json:"tls"`
	HTTP  string `json:"http"`
}

type rawDependency struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Protocol    Protocol `json:"protocol"`
	Host        string   `json:"host"`
	Port        int      `json:"port"`
	HTTP        *rawHTTP `json:"http,omitempty"`
}

type rawHTTP struct {
	Path             string `json:"path"`
	AcceptedStatuses []int  `json:"accepted_statuses"`
}

// Load parses and validates one complete catalog JSON document.
func Load(r io.Reader) (*Catalog, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxConfigBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read configuration: %w", err)
	}
	if len(data) > maxConfigBytes {
		return nil, invalidf("configuration exceeds %d bytes", maxConfigBytes)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var raw rawConfig
	if err := decoder.Decode(&raw); err != nil {
		return nil, invalidf("decode JSON: %v", err)
	}

	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, invalidf("configuration contains more than one JSON value")
		}
		return nil, invalidf("decode trailing data: %v", err)
	}

	return buildCatalog(raw)
}

// Timeouts returns the validated timeout policy.
func (c *Catalog) Timeouts() Timeouts {
	return c.timeouts
}

// Lookup returns a defensive copy of the dependency with the exact logical name.
func (c *Catalog) Lookup(name string) (Dependency, bool) {
	dependency, ok := c.dependencies[name]
	if !ok {
		return Dependency{}, false
	}
	return cloneDependency(dependency), true
}

// List returns safe dependency summaries sorted by logical name.
func (c *Catalog) List() []Summary {
	return append([]Summary(nil), c.summaries...)
}

func cloneDependency(dependency Dependency) Dependency {
	if dependency.HTTP == nil {
		return dependency
	}

	httpCheck := *dependency.HTTP
	httpCheck.AcceptedStatuses = append([]int(nil), dependency.HTTP.AcceptedStatuses...)
	dependency.HTTP = &httpCheck
	return dependency
}

func newCatalog(timeouts Timeouts, dependencies []Dependency) *Catalog {
	byName := make(map[string]Dependency, len(dependencies))
	summaries := make([]Summary, 0, len(dependencies))

	for _, dependency := range dependencies {
		byName[dependency.Name] = cloneDependency(dependency)
		summaries = append(summaries, Summary{
			Name:        dependency.Name,
			Description: dependency.Description,
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Name < summaries[j].Name
	})

	return &Catalog{
		timeouts:     timeouts,
		dependencies: byName,
		summaries:    summaries,
	}
}
