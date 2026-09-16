package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kiliankoe/opendata-dresden/internal/config"
	"github.com/kiliankoe/opendata-dresden/internal/index"
	"github.com/kiliankoe/opendata-dresden/internal/portal"
	"github.com/kiliankoe/opendata-dresden/internal/portal/portaltest"
)

// run executes a command against the fake portal and an index built from it
func run(t *testing.T, fake *portaltest.Server, args ...string) (string, error) {
	t.Helper()
	cfg := &config.Config{PortalURL: fake.URL, IndexURL: filepath.Join(t.TempDir(), "index.json")}
	idx, _, err := index.Build(context.Background(), portal.NewClient(cfg), &index.Index{})
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.Write(cfg.IndexURL); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err = Run(context.Background(), cfg, args, &out)
	return out.String(), err
}

func TestSearch(t *testing.T) {
	fake := portaltest.New(t)
	// Query words are joined so no quoting is needed on the shell
	out, err := run(t, fake, "search", "--output", "json", "--limit", "5", "geborene", "nach")
	if err != nil {
		t.Fatal(err)
	}
	var result portal.SearchResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || len(result.Datasets) != 1 || result.Datasets[0].ID != "D2" {
		t.Errorf("got %+v, want only D2", result)
	}
}

func TestSearchPaging(t *testing.T) {
	fake := portaltest.New(t)
	out, err := run(t, fake, "search", "--output", "json", "--limit", "2", "--offset", "1")
	if err != nil {
		t.Fatal(err)
	}
	var result portal.SearchResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if result.Total != 3 || len(result.Datasets) != 2 || result.Datasets[0].ID != "D3" {
		t.Errorf("unexpected page %+v", result)
	}
}

func TestInfo(t *testing.T) {
	fake := portaltest.New(t)
	out, err := run(t, fake, "info", "--output", "json", "D2")
	if err != nil {
		t.Fatal(err)
	}
	var ds portal.Dataset
	if err := json.Unmarshal([]byte(out), &ds); err != nil {
		t.Fatal(err)
	}
	if ds.Title != "Geborene nach Geschlecht 2020" || len(ds.Resources) != 2 {
		t.Errorf("unexpected dataset %+v", ds)
	}

	if _, err := run(t, fake, "info", "nope"); err == nil {
		t.Error("expected error for unknown dataset, got nil")
	}
	if _, err := run(t, fake, "info"); err == nil {
		t.Error("expected error for missing id, got nil")
	}
}

func TestSearchText(t *testing.T) {
	fake := portaltest.New(t)
	out, err := run(t, fake, "search", "--limit", "2", "--offset", "1")
	if err != nil {
		t.Fatal(err)
	}
	want := `Kaputter Datensatz
  D3  updated 01.01.2020  CSV
Stadtteil-Wanderwege
  D1  updated 13.03.2024  CSV, GEOJSON, WFS, Information

2 of 3 datasets
`
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}

	if _, err := run(t, fake, "search", "--output", "yaml"); err == nil {
		t.Error("expected error for unknown output format, got nil")
	}
}

func TestInfoText(t *testing.T) {
	fake := portaltest.New(t)
	out, err := run(t, fake, "info", "D1")
	if err != nil {
		t.Fatal(err)
	}
	want := `Stadtteil-Wanderwege
Wanderwege durch die Dresdner Stadtteile.
Stand 2024.
ID       D1
Updated  13.03.2024
Source   Umweltamt
License  dl-de/by-2-0
Topics   Umwelt und Klima
Regions  Dresden
Years    2024
Origin   Erhoben durch das Umweltamt.

Resources
  CSV          ` + fake.URL + `/ogcapi/collections/L1527/items?format=csv/ewkt&delimiter=semicolon
  GEOJSON      ` + fake.URL + `/ogcapi/collections/L1527/items
  WFS          ` + fake.URL + `/ogcsl.ashx?nodeid=1959&service=wfs
  Information  ` + fake.URL + `/ogc.ashx?Service=Ikx&RenderHint=TargetHtml&NODEID=1959&
`
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestFetch(t *testing.T) {
	fake := portaltest.New(t)
	out, err := run(t, fake, "fetch", "--limit", "5", "--bbox", "13.7,51.0,13.8,51.1", "D1", "csv")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "\"id\";") {
		t.Errorf("unexpected CSV output %q", out)
	}
	if q := fake.LastQuery; !strings.Contains(q, "limit=5") || !strings.Contains(q, "bbox=") {
		t.Errorf("query = %q, want limit and bbox", q)
	}

	if _, err := run(t, fake, "fetch", "D1", "pdf"); err == nil {
		t.Error("expected error for unavailable format, got nil")
	}
	if _, err := run(t, fake, "fetch", "D1"); err == nil {
		t.Error("expected error for missing format, got nil")
	}
}

func TestHelpAndUnknownCommand(t *testing.T) {
	fake := portaltest.New(t)
	for _, args := range [][]string{{"help"}, {"search", "-h"}, {}} {
		out, err := run(t, fake, args...)
		if err != nil || !strings.Contains(out, "Usage:") {
			t.Errorf("%v: err=%v out=%q", args, err, out)
		}
	}
	if _, err := run(t, fake, "bogus"); err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Errorf("expected error naming the unknown command, got %v", err)
	}
}

func TestIndex(t *testing.T) {
	fake := portaltest.New(t)
	file := filepath.Join(t.TempDir(), "index.json")
	out, err := run(t, fake, "index", "--file", file)
	if err != nil {
		t.Fatal(err)
	}
	if out != "3 datasets, 3 new or changed\n" {
		t.Errorf("unexpected output %q", out)
	}
	data, err := os.ReadFile(file)
	if err != nil || !strings.Contains(string(data), `"description": "Wanderwege`) {
		t.Errorf("index file: err=%v content=%s", err, data)
	}
}

func TestVersion(t *testing.T) {
	fake := portaltest.New(t)
	for _, args := range [][]string{{"version"}, {"--version"}} {
		out, err := run(t, fake, args...)
		if err != nil || out != "od3 "+config.Version+"\n" {
			t.Errorf("%v: err=%v out=%q", args, err, out)
		}
	}
}
