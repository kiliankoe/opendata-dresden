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
	client := portal.NewClient(&config.Config{PortalURL: fake.URL})
	ctx := context.Background()

	idx, changed, err := Build(ctx, client, &Index{})
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Datasets) != 3 || len(changed) != 3 || fake.InfoRequests != 1 {
		t.Fatalf("got %d datasets, %d changed, %d info requests", len(idx.Datasets), len(changed), fake.InfoRequests)
	}
	if got := idx.Datasets[0]; got.ID != "D1" || got.Description == "" || got.Origin == "" {
		t.Errorf("first dataset = %+v, want described D1", got)
	}

	// Unchanged datasets keep their details without another page fetch;
	// only the geodata fixture D1 has a page to fetch at all
	again, changed, err := Build(ctx, client, idx)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 || fake.InfoRequests != 1 || again.Datasets[0].Description != idx.Datasets[0].Description {
		t.Errorf("rebuild: %d changed, %d info requests, description %q", len(changed), fake.InfoRequests, again.Datasets[0].Description)
	}

	// A changed update date invalidates the details
	idx.Datasets[0].Updated = "01.01.2000"
	if _, changed, err = Build(ctx, client, idx); err != nil || len(changed) != 1 || changed[0] != "D1" || fake.InfoRequests != 2 {
		t.Errorf("changed: err=%v changed=%v, %d info requests", err, changed, fake.InfoRequests)
	}

	// Mirror only reads the tables of republished datasets, so the date it
	// recorded has to survive every rebuild, including those
	idx.Datasets[0].Changed = "05.03.2026"
	idx.Datasets[0].Updated = "02.01.2000"
	if again, _, err = Build(ctx, client, idx); err != nil || again.Datasets[0].Changed != "05.03.2026" {
		t.Errorf("rebuild dropped the change date: err=%v dataset=%+v", err, again.Datasets[0])
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
