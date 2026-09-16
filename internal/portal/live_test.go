//go:build integration

package portal

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kiliankoe/opendatadresdenmcp/internal/config"
)

// These tests hit the real portal and check the assumptions the client makes
// about it. Run with: go test -tags integration ./...
const (
	geoDatasetID   = "0F6996E7-26AB-4585-81BD-1EDA4381B1BC" // Stadtteil-Wanderwege
	statsDatasetID = "1712B12B-18B5-4810-8B31-4B5B397F2C81" // Geborene nach Geschlecht 2020
)

func liveClient(t *testing.T) (context.Context, *Client) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	return ctx, NewClient(config.LoadConfig())
}

func TestLiveSearchDatasets(t *testing.T) {
	ctx, client := liveClient(t)
	result, err := client.SearchDatasets(ctx, "Wanderwege", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, ds := range result.Datasets {
		if ds.ID != geoDatasetID {
			continue
		}
		formats := make(map[string]bool)
		for _, r := range ds.Resources {
			formats[r.Format] = true
		}
		for _, want := range []string{"CSV", "GEOJSON", "WFS", "WMS"} {
			if !formats[want] {
				t.Errorf("missing %s resource in %+v", want, ds.Resources)
			}
		}
		if ds.LayerID != "L1527" || ds.License == "" || ds.Source == "" {
			t.Errorf("unexpected dataset %+v", ds)
		}
		return
	}
	t.Errorf("reference dataset missing from %d results (total %d)", len(result.Datasets), result.Total)
}

func TestLiveSearchDatasetsEmptyQueryListsAll(t *testing.T) {
	ctx, client := liveClient(t)
	result, err := client.SearchDatasets(ctx, "", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total < 1000 || len(result.Datasets) != 1 {
		t.Errorf("got total=%d datasets=%d, expected over a thousand and exactly one", result.Total, len(result.Datasets))
	}
}

func TestLiveGetDataset(t *testing.T) {
	ctx, client := liveClient(t)
	ds, err := client.GetDataset(ctx, geoDatasetID)
	if err != nil {
		t.Fatal(err)
	}
	if ds.ID != geoDatasetID || ds.Title != "Stadtteil-Wanderwege" {
		t.Errorf("unexpected dataset %+v", ds)
	}

	if _, err := client.GetDataset(ctx, "does-not-exist"); err == nil {
		t.Error("expected error for unknown dataset, got nil")
	}
}

func TestLiveFetchResource(t *testing.T) {
	ctx, client := liveClient(t)
	geo, err := client.GetDataset(ctx, geoDatasetID)
	if err != nil {
		t.Fatal(err)
	}

	csv, err := client.FetchResource(ctx, geo, "CSV", 2)
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Split(strings.TrimSpace(string(csv)), "\n"); len(lines) != 3 || !strings.Contains(lines[0], ";") {
		t.Errorf("expected a semicolon-separated header and 2 rows, got %q", csv)
	}

	geojson, err := client.FetchResource(ctx, geo, "GeoJSON", 2)
	if err != nil {
		t.Fatal(err)
	}
	var fc struct {
		Type     string `json:"type"`
		Features []any  `json:"features"`
	}
	if err := json.Unmarshal(geojson, &fc); err != nil || fc.Type != "FeatureCollection" || len(fc.Features) != 2 {
		t.Errorf("expected a FeatureCollection with 2 features, got type=%q features=%d err=%v", fc.Type, len(fc.Features), err)
	}

	stats, err := client.GetDataset(ctx, statsDatasetID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := client.FetchResource(ctx, stats, "JSON", 0)
	if err != nil {
		t.Fatal(err)
	}
	var table struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(data, &table); err != nil || len(table.Data) == 0 {
		t.Errorf("expected a non-empty data table, got %d rows err=%v", len(table.Data), err)
	}
}
