package arcgis

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kiliankoe/opendata-dresden/internal/config"
	"github.com/kiliankoe/opendata-dresden/internal/portal"
	"github.com/kiliankoe/opendata-dresden/internal/portal/portaltest"
)

func newFake(t *testing.T) (*portaltest.Server, *Client) {
	t.Helper()
	fake := portaltest.New(t)
	return fake, NewClient(&config.Config{ArcGISURL: fake.URL})
}

func TestListDatasets(t *testing.T) {
	fake, client := newFake(t)
	datasets, err := client.ListDatasets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(datasets) != 1 {
		t.Fatalf("got %d datasets, want 1", len(datasets))
	}
	got, _ := json.Marshal(datasets[0])
	layer := fake.URL + "/rest/services/Haltestellen/FeatureServer/0"
	want, _ := json.Marshal(portal.Dataset{
		ID: "a1b2-0", Title: "Haltestellen", Updated: "17.09.2025", Source: "Landeshauptstadt Dresden",
		License: "Beachten Sie die Nutzungsbedingungen.", Portal: Portal,
		Resources: []portal.Resource{
			{Format: "GEOJSON", URL: layer + "/query?where=1%3D1&outFields=*&f=geojson"},
			{Format: FeatureServer, URL: layer},
		},
		Description: "Alle Haltestellen in Dresden.",
	})
	if string(got) != string(want) {
		t.Errorf("dataset\n got %s\nwant %s", got, want)
	}
}

func TestConvertMultiLayerService(t *testing.T) {
	it := item{ID: "x", Title: "INSEK", Modified: 1757980800000}
	layers := []layer{{ID: 2, Name: "Projekte"}, {ID: 3, Name: "Räume"}}
	datasets := convert(it, layers)
	if len(datasets) != 2 || datasets[0].ID != "x-2" || datasets[0].Title != "INSEK - Projekte" || datasets[1].Title != "INSEK - Räume" {
		t.Errorf("got %+v", datasets)
	}
	// Without a description the item's snippet and finally the org name serve
	if datasets[0].Source != orgName || datasets[0].Updated != "16.09.2025" {
		t.Errorf("got source %q updated %q", datasets[0].Source, datasets[0].Updated)
	}
	it.Snippet = "Kurz"
	if ds := convert(it, layers[:1]); ds[0].Description != "Kurz" || ds[0].Title != "INSEK" {
		t.Errorf("single layer: %+v", ds[0])
	}
}

func TestFetchResource(t *testing.T) {
	fake, client := newFake(t)
	ctx := context.Background()
	datasets, _ := client.ListDatasets(ctx)
	ds := &datasets[0]

	// GeoJSON is paged through until the service stops announcing more
	data, err := client.FetchResource(ctx, ds, "geojson", portal.FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var collection struct {
		Features []json.RawMessage `json:"features"`
	}
	if err := json.Unmarshal(data, &collection); err != nil || len(collection.Features) != 2 {
		t.Errorf("got %d features (err %v): %s", len(collection.Features), err, data)
	}
	if len(fake.LayerQueries) != 2 || !strings.Contains(fake.LayerQueries[1], "resultOffset=1") {
		t.Errorf("queries = %q", fake.LayerQueries)
	}

	// A limit stops the paging, a bbox becomes an envelope filter
	fake.LayerQueries = nil
	if _, err := client.FetchResource(ctx, ds, "GEOJSON", portal.FetchOptions{Limit: 1, BBox: "13.7,51.0,13.8,51.1"}); err != nil {
		t.Fatal(err)
	}
	if q := strings.Join(fake.LayerQueries, " "); len(fake.LayerQueries) != 1 ||
		!strings.Contains(q, "resultRecordCount=1") || !strings.Contains(q, "geometry=13.7%2C51.0%2C13.8%2C51.1") || !strings.Contains(q, "inSR=4326") {
		t.Errorf("queries = %q", fake.LayerQueries)
	}

	// The layer's own endpoint is returned as is
	data, err = client.FetchResource(ctx, ds, FeatureServer, portal.FetchOptions{})
	if err != nil || !strings.Contains(string(data), "lastEditDate") {
		t.Errorf("layer fetch: err=%v data=%s", err, data)
	}
	if _, err := client.FetchResource(ctx, ds, "CSV", portal.FetchOptions{}); err == nil {
		t.Error("expected error for unavailable format, got nil")
	}
}
