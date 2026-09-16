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

func TestBuild(t *testing.T) {
	fake := portaltest.New(t)
	cfg := &config.Config{PortalURL: fake.URL, ArcGISURL: fake.URL}
	ctx := context.Background()

	idx, refreshed, err := Build(ctx, cfg, &Index{})
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Datasets) != 4 || refreshed != 4 || fake.InfoRequests != 1 {
		t.Fatalf("got %d datasets, %d refreshed, %d info requests", len(idx.Datasets), refreshed, fake.InfoRequests)
	}
	if got := idx.Datasets[0]; got.ID != "D1" || got.Description == "" || got.Origin == "" {
		t.Errorf("first dataset = %+v, want described D1", got)
	}
	if got := idx.Datasets[3]; got.ID != "a1b2-0" || got.Portal != "ArcGIS Online" {
		t.Errorf("last dataset = %+v, want the ArcGIS layer", got)
	}

	// Unchanged datasets keep their details without another page fetch;
	// only the geodata fixture D1 has a page to fetch at all
	again, refreshed, err := Build(ctx, cfg, idx)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed != 0 || fake.InfoRequests != 1 || again.Datasets[0].Description != idx.Datasets[0].Description {
		t.Errorf("rebuild: %d refreshed, %d info requests, description %q", refreshed, fake.InfoRequests, again.Datasets[0].Description)
	}

	// A changed update date invalidates the details
	idx.Datasets[0].Updated = "01.01.2000"
	if _, refreshed, err = Build(ctx, cfg, idx); err != nil || refreshed != 1 || fake.InfoRequests != 2 {
		t.Errorf("changed: err=%v %d refreshed, %d info requests", err, refreshed, fake.InfoRequests)
	}
}

func TestReadWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.json")

	idx, err := Read(path)
	if err != nil || len(idx.Datasets) != 0 {
		t.Fatalf("missing file: err=%v datasets=%d", err, len(idx.Datasets))
	}

	idx.Datasets = []portal.Dataset{{ID: "B", Title: "b"}, {ID: "A", Title: "a"}}
	if err := idx.Write(path); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data[:16]) != "{\n  \"datasets\": " {
		t.Errorf("unexpected file start %q", data[:16])
	}

	idx, err = Read(path)
	if err != nil || len(idx.Datasets) != 2 || idx.Datasets[0].ID != "A" {
		t.Errorf("roundtrip: err=%v datasets=%+v, want sorted by ID", err, idx.Datasets)
	}
}
