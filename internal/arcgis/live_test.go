//go:build integration

package arcgis

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kiliankoe/opendata-dresden/internal/config"
	"github.com/kiliankoe/opendata-dresden/internal/portal"
)

// This test hits the real organization and checks the assumptions the client
// makes about it. Run with: go test -tags integration ./...
func TestLiveListAndFetch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client := NewClient(config.LoadConfig())

	datasets, err := client.ListDatasets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var stops *portal.Dataset
	for i := range datasets {
		if datasets[i].Title == "Haltestellen_p" {
			stops = &datasets[i]
		}
	}
	if stops == nil {
		t.Fatalf("reference layer missing from %d datasets", len(datasets))
	}
	if stops.Portal != Portal || stops.Updated == "" || len(stops.Resources) != 2 {
		t.Errorf("unexpected dataset %+v", stops)
	}

	data, err := client.FetchResource(ctx, stops, "GEOJSON", portal.FetchOptions{Limit: 3, BBox: "13.72,51.04,13.76,51.07"})
	if err != nil {
		t.Fatal(err)
	}
	var collection struct {
		Features []struct {
			Geometry struct {
				Coordinates []float64 `json:"coordinates"`
			} `json:"geometry"`
		} `json:"features"`
	}
	if err := json.Unmarshal(data, &collection); err != nil || len(collection.Features) != 3 {
		t.Fatalf("got %d features (err %v)", len(collection.Features), err)
	}
	// The service answers in WGS84 regardless of its own reference system
	if lon := collection.Features[0].Geometry.Coordinates[0]; lon < 13.72 || lon > 13.76 {
		t.Errorf("feature outside bbox: %v", collection.Features[0].Geometry.Coordinates)
	}
}
