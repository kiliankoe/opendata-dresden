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
	"github.com/kiliankoe/opendata-dresden/internal/portal"
	"github.com/kiliankoe/opendata-dresden/internal/portal/portaltest"
)

func run(t *testing.T, fake *portaltest.Server, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := Run(context.Background(), portal.NewClient(&config.Config{PortalURL: fake.URL}), args, &out)
	return out.String(), err
}

func TestSearch(t *testing.T) {
	fake := portaltest.New(t)
	// Query words are joined so no quoting is needed on the shell
	out, err := run(t, fake, "search", "--output", "json", "--limit", "5", "geborene", "nach")
	if err != nil {
		t.Fatal(err)
	}
	if fake.LastSearch.TextSearch != "geborene nach" || fake.LastSearch.NumOfResults != 5 {
		t.Errorf("unexpected search request %+v", fake.LastSearch)
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
	if result.Total != 3 || len(result.Datasets) != 2 || result.Datasets[0].ID != "D2" {
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
	want := `Geborene nach Geschlecht 2020
  D2  updated 15.07.2025  JSON, Tabelle
Kaputter Datensatz
  D3  updated 01.01.2020  CSV

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
	out, err := run(t, fake, "info", "D2")
	if err != nil {
		t.Fatal(err)
	}
	want := `Geborene nach Geschlecht 2020
ID       D2
Updated  15.07.2025
Source   Einwohnermelderegister
License  dl-de/by-2-0
Topics   Bevölkerung
Regions  Stadtbezirk
Years    2020

Resources
  JSON     ` + fake.URL + `/dcat-ap/dataset/geborene/content.json
  Tabelle  ` + fake.URL + `/aswdb/asw.dll/?aw=Bev%C3%B6lkerung/Geborene%20nach%20Geschlecht_2020_TAB
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
