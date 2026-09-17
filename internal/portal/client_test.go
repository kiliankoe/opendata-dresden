package portal

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kiliankoe/opendata-dresden/internal/config"
	"github.com/kiliankoe/opendata-dresden/internal/portal/portaltest"
)

func newFakePortal(t *testing.T) (*portaltest.Server, *Client) {
	t.Helper()
	fake := portaltest.New(t)
	return fake, NewClient(&config.Config{PortalURL: fake.URL})
}

func TestSearchDatasets(t *testing.T) {
	fake, client := newFakePortal(t)
	result, err := client.SearchDatasets(context.Background(), "wanderwege", 10, 0)
	if err != nil {
		t.Fatal(err)
	}

	req := fake.LastSearch
	var fields []string
	for _, a := range req.TextSearchConfig.Attribute {
		fields = append(fields, a.Name)
	}
	if req.TextSearch != "wanderwege" || req.NumOfResults != 10 || req.PagingStart != 0 ||
		strings.Join(req.UserGroupIDs, ",") != guestGroupID || strings.Join(fields, ",") != strings.Join(searchFields, ",") {
		t.Errorf("unexpected search request %+v", req)
	}

	if result.Total != 1 || len(result.Datasets) != 1 {
		t.Fatalf("got total=%d datasets=%d, want 1 and 1", result.Total, len(result.Datasets))
	}
	got, _ := json.Marshal(result.Datasets[0])
	want, _ := json.Marshal(Dataset{
		ID: "D1", Title: "Stadtteil-Wanderwege", Updated: "13.03.2024", Source: "Umweltamt", License: "dl-de/by-2-0",
		Topics: []string{"Umwelt und Klima"}, Regions: []string{"Dresden"}, Years: []string{"2024"}, LayerID: "L1527",
		Resources: []Resource{
			{Format: "CSV", URL: fake.URL + "/ogcapi/collections/L1527/items?format=csv/ewkt&delimiter=semicolon"},
			{Format: "GEOJSON", URL: fake.URL + "/ogcapi/collections/L1527/items"},
			{Format: "WFS", URL: fake.URL + "/ogcsl.ashx?nodeid=1959&service=wfs"},
			{Format: "Information", URL: fake.URL + "/ogc.ashx?Service=Ikx&RenderHint=TargetHtml&NODEID=1959&"},
		},
	})
	if string(got) != string(want) {
		t.Errorf("dataset\n got %s\nwant %s", got, want)
	}
}

func TestSearchDatasetsPaging(t *testing.T) {
	_, client := newFakePortal(t)
	result, err := client.SearchDatasets(context.Background(), "", 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 3 || len(result.Datasets) != 2 || result.Datasets[0].ID != "D2" || result.Datasets[1].ID != "D3" {
		t.Errorf("unexpected page %+v", result)
	}
}

func TestGetDataset(t *testing.T) {
	fake, client := newFakePortal(t)
	ds, err := client.GetDataset(context.Background(), "D2")
	if err != nil {
		t.Fatal(err)
	}
	if attrs := fake.LastSearch.TextSearchConfig.Attribute; len(attrs) != 1 || attrs[0].Name != datasetIDField {
		t.Errorf("lookup searched %v, want only %s", attrs, datasetIDField)
	}
	if ds.Title != "Geborene nach Geschlecht 2020" || ds.LayerID != "" {
		t.Errorf("unexpected dataset %+v", ds)
	}
	wantTable := fake.URL + "/aswdb/asw.dll/?aw=Bev%C3%B6lkerung/Geborene%20nach%20Geschlecht_2020_TAB"
	if len(ds.Resources) != 3 || ds.Resources[2].URL != wantTable {
		t.Errorf("table resource = %+v, want URL %s", ds.Resources, wantTable)
	}

	if _, err := client.GetDataset(context.Background(), "nope"); err == nil {
		t.Error("expected error for unknown dataset, got nil")
	}
}

func TestFetchResource(t *testing.T) {
	fake, client := newFakePortal(t)
	ctx := context.Background()
	geo, _ := client.GetDataset(ctx, "D1")
	stats, _ := client.GetDataset(ctx, "D2")
	broken, _ := client.GetDataset(ctx, "D3")
	limit := FetchOptions{Limit: 5}

	data, err := client.FetchResource(ctx, geo, "csv", FetchOptions{})
	if err != nil || !strings.HasPrefix(string(data), "\"id\";") {
		t.Errorf("CSV fetch: err=%v data=%q", err, data)
	}
	if fake.LastQuery != "format=csv/ewkt&delimiter=semicolon" {
		t.Errorf("CSV query = %q", fake.LastQuery)
	}

	if _, err := client.FetchResource(ctx, geo, "GeoJSON", limit); err != nil {
		t.Fatal(err)
	}
	if fake.LastQuery != "limit=5" {
		t.Errorf("GeoJSON query = %q, want limit=5", fake.LastQuery)
	}
	if _, err := client.FetchResource(ctx, geo, "CSV", limit); err != nil {
		t.Fatal(err)
	}
	if fake.LastQuery != "format=csv/ewkt&delimiter=semicolon&limit=5" {
		t.Errorf("CSV query with limit = %q", fake.LastQuery)
	}
	if _, err := client.FetchResource(ctx, geo, "GeoJSON", FetchOptions{BBox: "13.7,51.0,13.8,51.1"}); err != nil {
		t.Fatal(err)
	}
	if got := fake.LastQuery; got != "bbox=13.7,51.0,13.8,51.1" && got != "bbox=13.7%2C51.0%2C13.8%2C51.1" {
		t.Errorf("GeoJSON query with bbox = %q", got)
	}

	data, err = client.FetchResource(ctx, stats, "JSON", limit)
	if err != nil || !strings.Contains(string(data), `"Jahr"`) {
		t.Errorf("JSON fetch: err=%v data=%q", err, data)
	}

	if _, err := client.FetchResource(ctx, stats, "WFS", FetchOptions{}); err == nil {
		t.Error("expected error for unavailable format, got nil")
	}
	if _, err := client.FetchResource(ctx, broken, "CSV", FetchOptions{}); err == nil {
		t.Error("expected error for HTTP 500, got nil")
	}
}

func TestDescribe(t *testing.T) {
	_, client := newFakePortal(t)
	ctx := context.Background()
	geo, _ := client.GetDataset(ctx, "D1")
	if err := client.Describe(ctx, geo); err != nil {
		t.Fatal(err)
	}
	if geo.Description != "Wanderwege durch die Dresdner Stadtteile.\nStand 2024." || geo.Origin != "Erhoben durch das Umweltamt." {
		t.Errorf("got description %q origin %q", geo.Description, geo.Origin)
	}

	// Statistics datasets have no page worth reading and stay untouched
	stats, _ := client.GetDataset(ctx, "D2")
	if err := client.Describe(ctx, stats); err != nil || stats.Description != "" {
		t.Errorf("stats: err=%v description=%q", err, stats.Description)
	}
}
