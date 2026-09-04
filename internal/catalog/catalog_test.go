package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLoadValidCatalog(t *testing.T) {
	catalog, err := Load(strings.NewReader(validCatalogJSON()))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	wantTimeouts := Timeouts{
		Total: 5 * time.Second,
		DNS:   time.Second,
		TCP:   time.Second,
		TLS:   2 * time.Second,
		HTTP:  2 * time.Second,
	}

	if got := catalog.Timeouts(); got != wantTimeouts {
		t.Fatalf(
			"Timeouts() = %#v, want %#v",
			got,
			wantTimeouts,
		)
	}

	wantSummaries := []Summary{
		{
			Name:        "api",
			Description: "Application API",
		},
		{
			Name:        "database",
			Description: "Primary database",
		},
	}

	if got := catalog.List(); !reflect.DeepEqual(got, wantSummaries) {
		t.Fatalf(
			"List() = %#v, want %#v",
			got,
			wantSummaries,
		)
	}

	api, ok := catalog.Lookup("api")
	if !ok {
		t.Fatal("Lookup(api) did not find configured dependency")
	}

	if api.Protocol != ProtocolHTTPS ||
		api.Host != "api.example.com" ||
		api.Port != 443 {
		t.Fatalf("Lookup(api) = %#v", api)
	}

	if api.HTTP == nil {
		t.Fatal("Lookup(api).HTTP = nil")
	}

	if got, want := api.HTTP.AcceptedStatuses, []int{200, 204}; !reflect.DeepEqual(got, want) {
		t.Fatalf(
			"accepted statuses = %#v, want %#v",
			got,
			want,
		)
	}

	if _, ok := catalog.Lookup("missing"); ok {
		t.Fatal("Lookup(missing) unexpectedly succeeded")
	}
}

func TestCatalogReturnsDefensiveCopies(t *testing.T) {
	catalog, err := Load(strings.NewReader(validCatalogJSON()))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	summaries := catalog.List()
	summaries[0].Description = "mutated"

	if got := catalog.List()[0].Description; got == "mutated" {
		t.Fatal("List() exposed mutable catalog state")
	}

	dependency, ok := catalog.Lookup("api")
	if !ok || dependency.HTTP == nil {
		t.Fatal("Lookup(api) failed")
	}

	dependency.HTTP.Path = "/mutated"
	dependency.HTTP.AcceptedStatuses[0] = 599

	fresh, ok := catalog.Lookup("api")
	if !ok || fresh.HTTP == nil {
		t.Fatal("second Lookup(api) failed")
	}

	if fresh.HTTP.Path == "/mutated" ||
		fresh.HTTP.AcceptedStatuses[0] == 599 {
		t.Fatal("Lookup() exposed mutable catalog state")
	}
}

func TestLoadRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{
			name: "unsupported version",
			mutate: func(config map[string]any) {
				config["version"] = 2
			},
		},
		{
			name: "missing dependencies",
			mutate: func(config map[string]any) {
				config["dependencies"] = []any{}
			},
		},
		{
			name: "duplicate dependency name",
			mutate: func(config map[string]any) {
				dependencies := config["dependencies"].([]any)

				duplicate := maps.Clone(
					dependencies[0].(map[string]any),
				)

				dependencies[1] = duplicate
			},
		},
		{
			name: "invalid dependency name",
			mutate: func(config map[string]any) {
				firstDependency(config)["name"] = "API"
			},
		},
		{
			name: "blank description",
			mutate: func(config map[string]any) {
				firstDependency(config)["description"] = " "
			},
		},
		{
			name: "host contains URL",
			mutate: func(config map[string]any) {
				firstDependency(config)["host"] =
					"https://api.example.com"
			},
		},
		{
			name: "host contains port",
			mutate: func(config map[string]any) {
				firstDependency(config)["host"] =
					"api.example.com:443"
			},
		},
		{
			name: "invalid port",
			mutate: func(config map[string]any) {
				firstDependency(config)["port"] = 70000
			},
		},
		{
			name: "unsupported protocol",
			mutate: func(config map[string]any) {
				firstDependency(config)["protocol"] = "udp"
			},
		},
		{
			name: "HTTP configuration missing",
			mutate: func(config map[string]any) {
				delete(apiDependency(config), "http")
			},
		},
		{
			name: "HTTP configuration forbidden for TCP",
			mutate: func(config map[string]any) {
				apiDependency(config)["protocol"] = "tcp"
			},
		},
		{
			name: "HTTP path has query",
			mutate: func(config map[string]any) {
				firstHTTP(config)["path"] =
					"/ready?verbose=true"
			},
		},
		{
			name: "HTTP statuses empty",
			mutate: func(config map[string]any) {
				firstHTTP(config)["accepted_statuses"] =
					[]any{}
			},
		},
		{
			name: "HTTP status duplicated",
			mutate: func(config map[string]any) {
				firstHTTP(config)["accepted_statuses"] =
					[]any{200, 200}
			},
		},
		{
			name: "HTTP status invalid",
			mutate: func(config map[string]any) {
				firstHTTP(config)["accepted_statuses"] =
					[]any{700}
			},
		},
		{
			name: "stage timeout exceeds total",
			mutate: func(config map[string]any) {
				config["timeouts"].(map[string]any)["dns"] =
					"6s"
			},
		},
		{
			name: "total timeout exceeds maximum",
			mutate: func(config map[string]any) {
				config["timeouts"].(map[string]any)["total"] =
					"31s"
			},
		},
		{
			name: "stage timeout exceeds maximum",
			mutate: func(config map[string]any) {
				timeouts := config["timeouts"].(map[string]any)
				timeouts["total"] = "20s"
				timeouts["tls"] = "11s"
			},
		},
		{
			name: "timeout missing",
			mutate: func(config map[string]any) {
				delete(
					config["timeouts"].(map[string]any),
					"dns",
				)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := validConfigMap(t)
			test.mutate(config)

			data, err := json.Marshal(config)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}

			_, err = Load(strings.NewReader(string(data)))
			if err == nil {
				t.Fatal("Load() unexpectedly succeeded")
			}

			if !errors.Is(err, ErrInvalidConfiguration) {
				t.Fatalf(
					"Load() error = %v, want ErrInvalidConfiguration",
					err,
				)
			}
		})
	}
}

func TestLoadRejectsUnknownJSONField(t *testing.T) {
	input := strings.Replace(
		validCatalogJSON(),
		`"version": 1,`,
		`"version": 1, "unexpected": true,`,
		1,
	)

	_, err := Load(strings.NewReader(input))
	if err == nil || !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf(
			"Load() error = %v, want ErrInvalidConfiguration",
			err,
		)
	}
}

func TestLoadRejectsTrailingJSON(t *testing.T) {
	_, err := Load(
		strings.NewReader(
			validCatalogJSON() + ` {"extra":true}`,
		),
	)

	if err == nil || !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf(
			"Load() error = %v, want ErrInvalidConfiguration",
			err,
		)
	}
}

func TestLoadRejectsOversizedConfiguration(t *testing.T) {
	_, err := Load(
		strings.NewReader(
			strings.Repeat(" ", maxConfigBytes+1),
		),
	)

	if err == nil || !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf(
			"Load() error = %v, want ErrInvalidConfiguration",
			err,
		)
	}
}

func TestLoadRejectsTooManyDependencies(t *testing.T) {
	config := validConfigMap(t)
	dependencies := make([]any, maxDependencies+1)
	base := firstDependency(config)

	for i := range dependencies {
		dependency := maps.Clone(base)
		dependency["name"] = fmt.Sprintf("dep%03d", i)
		dependencies[i] = dependency
	}

	config["dependencies"] = dependencies

	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	_, err = Load(strings.NewReader(string(data)))
	if err == nil || !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf(
			"Load() error = %v, want ErrInvalidConfiguration",
			err,
		)
	}
}

func TestLoadAcceptsIPv6Address(t *testing.T) {
	config := validConfigMap(t)
	dependency := firstDependency(config)
	dependency["host"] = "::1"

	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	if _, err := Load(strings.NewReader(string(data))); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}

func validCatalogJSON() string {
	return `{
  "version": 1,
  "timeouts": {
    "total": "5s",
    "dns": "1s",
    "tcp": "1s",
    "tls": "2s",
    "http": "2s"
  },
  "dependencies": [
    {
      "name": "database",
      "description": "Primary database",
      "protocol": "tcp",
      "host": "db.example.com",
      "port": 5432
    },
    {
      "name": "api",
      "description": "Application API",
      "protocol": "https",
      "host": "api.example.com",
      "port": 443,
      "http": {
        "path": "/ready",
        "accepted_statuses": [204, 200]
      }
    }
  ]
}`
}

func validConfigMap(t *testing.T) map[string]any {
	t.Helper()

	var config map[string]any

	if err := json.Unmarshal(
		[]byte(validCatalogJSON()),
		&config,
	); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	return config
}

func firstDependency(
	config map[string]any,
) map[string]any {
	return config["dependencies"].([]any)[0].(map[string]any)
}

func apiDependency(
	config map[string]any,
) map[string]any {
	return config["dependencies"].([]any)[1].(map[string]any)
}

func firstHTTP(
	config map[string]any,
) map[string]any {
	return apiDependency(config)["http"].(map[string]any)
}
