package index

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kiliankoe/opendata-dresden/internal/config"
	"github.com/kiliankoe/opendata-dresden/internal/portal"
	"github.com/kiliankoe/opendata-dresden/internal/portal/portaltest"
)

func TestMirror(t *testing.T) {
	fake := portaltest.New(t)
	client := portal.NewClient(&config.Config{PortalURL: fake.URL})
	ctx := context.Background()
	dir := t.TempDir()
	file := filepath.Join(dir, "geborene.csv")

	idx, changed, err := Build(ctx, client, &Index{})
	if err != nil {
		t.Fatal(err)
	}

	// Only the statistics fixture D2 has a table to mirror; the geodata
	// layer's CSV and the broken dataset's are left to the portal
	written, err := Mirror(ctx, client, idx, changed, dir)
	if err != nil || written != 1 {
		t.Fatalf("first run: err=%v written=%d", err, written)
	}
	if got, err := os.ReadFile(file); err != nil || string(got) != portaltest.StatsCSV {
		t.Fatalf("mirrored table: err=%v content=%q", err, got)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("got %d files, want only the statistics table", len(entries))
	}

	// Nothing changed, so nothing is downloaded again
	requests := fake.CSVRequests
	if written, err = Mirror(ctx, client, idx, nil, dir); err != nil || written != 0 || fake.CSVRequests != requests {
		t.Errorf("unchanged: err=%v written=%d requests=%d", err, written, fake.CSVRequests)
	}

	// A file an earlier run failed to write comes back without a changed date
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if written, err = Mirror(ctx, client, idx, nil, dir); err != nil || written != 1 {
		t.Errorf("missing file: err=%v written=%d", err, written)
	}

	// Datasets the portal stops listing take their file with them
	stray := filepath.Join(dir, "verschwunden.csv")
	if err := os.WriteFile(stray, []byte("alt"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Mirror(ctx, client, idx, nil, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stray); !os.IsNotExist(err) {
		t.Errorf("stray file survived: %v", err)
	}
}
