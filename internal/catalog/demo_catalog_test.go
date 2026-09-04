package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDemoCatalogLoads(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "demo", "catalog.json")
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("os.Open(%q) error = %v", path, err)
	}
	defer file.Close()

	value, err := Load(file)
	if err != nil {
		t.Fatalf("Load(%q) error = %v", path, err)
	}

	summaries := value.List()
	if len(summaries) != 3 {
		t.Fatalf("demo dependency count = %d, want 3", len(summaries))
	}

	wantNames := []string{"demo_closed", "demo_ready", "demo_unhealthy"}
	for i, want := range wantNames {
		if summaries[i].Name != want {
			t.Fatalf("demo dependency %d name = %q, want %q", i, summaries[i].Name, want)
		}
	}
}
