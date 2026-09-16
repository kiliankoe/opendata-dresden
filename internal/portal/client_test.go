package portal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/kiliankoe/opendatadresdenmcp/internal/config"
)

// fixtures replicate the portal's search results for three datasets: a geodata
// layer, a statistics table and one whose download is broken.
const fixtures = `[
 {"bezeichnung":"Stadtteil-Wanderwege","letzteAenderung":"13.03.2024",
  "themen":["Umwelt und Klima","Umwelt und Klima"],"raeume":["Dresden"],"zeiten":["2024","2024"],
  "dataSource":{"name":"Umweltamt"},"presentationLicense":{"name":"dl-de/by-2-0"},
  "presentations":[
   {"ergebnisId":"D1","darstellungsArtBezeichnung":"CSV","schnittstelle":"{{base}}/ogcapi/collections/L1527/items?format=csv/ewkt&delimiter=semicolon","aufrufUrl":null},
   {"ergebnisId":"D1","darstellungsArtBezeichnung":"GEOJSON","schnittstelle":"{{base}}/ogcapi/collections/L1527","aufrufUrl":null},
   {"ergebnisId":"D1","darstellungsArtBezeichnung":"WFS","schnittstelle":"{{base}}/ogcsl.ashx?nodeid=1959&service=wfs","aufrufUrl":null}]},
 {"bezeichnung":"Geborene nach Geschlecht 2020","letzteAenderung":"15.07.2025",
  "themen":["Bevölkerung","Bevölkerung"],"raeume":["Stadtbezirk"],"zeiten":["2020","2020"],
  "dataSource":{"name":"Einwohnermelderegister"},"presentationLicense":{"name":"dl-de/by-2-0"},
  "presentations":[
   {"ergebnisId":"D2","darstellungsArtBezeichnung":"JSON","schnittstelle":"{{base}}/dcat-ap/dataset/geborene/content.json","aufrufUrl":null},
   {"ergebnisId":"D2","darstellungsArtBezeichnung":"Tabelle","schnittstelle":"Bevölkerung/Geborene nach Geschlecht_2020_TAB","aufrufUrl":"{{base}}/aswdb/asw.dll/?aw="}]},
 {"bezeichnung":"Kaputter Datensatz","letzteAenderung":"01.01.2020","themen":[],"raeume":[],"zeiten":[],
  "dataSource":{"name":"Test"},"presentationLicense":{"name":"none"},
  "presentations":[
   {"ergebnisId":"D3","darstellungsArtBezeichnung":"CSV","schnittstelle":"{{base}}/broken","aufrufUrl":null}]}
]`

type fakePortal struct {
	*httptest.Server
	client     *Client
	lastSearch searchRequest // most recent search request body
	lastQuery  string        // raw query string of the most recent OGC items request
}

func newFakePortal(t *testing.T) *fakePortal {
	t.Helper()
	fake := &fakePortal{}
	mux := http.NewServeMux()
	fake.Server = httptest.NewServer(mux)
	t.Cleanup(fake.Close)
	fake.client = NewClient(&config.Config{PortalURL: fake.URL})

	var datasets []searchResult
	if err := json.Unmarshal([]byte(strings.ReplaceAll(fixtures, "{{base}}", fake.URL)), &datasets); err != nil {
		t.Fatal(err)
	}

	mux.HandleFunc("POST /service/app/search/all", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&fake.lastSearch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		req := fake.lastSearch
		byID := len(req.TextSearchConfig.Attribute) == 1 && req.TextSearchConfig.Attribute[0].Name == datasetIDField
		var matches []searchResult
		for _, ds := range datasets {
			if byID && ds.Presentations[0].DatasetID == req.TextSearch ||
				!byID && strings.Contains(strings.ToLower(ds.Title), strings.ToLower(req.TextSearch)) {
				matches = append(matches, ds)
			}
		}
		total := len(matches)
		matches = matches[min(req.PagingStart, total):]
		matches = matches[:min(req.NumOfResults, len(matches))]
		_ = json.NewEncoder(w).Encode(map[string]any{"numOfResults": total, "ipResults": matches})
	})
	mux.HandleFunc("/ogcapi/collections/L1527/items", func(w http.ResponseWriter, r *http.Request) {
		fake.lastQuery = r.URL.RawQuery
		if strings.HasPrefix(r.URL.Query().Get("format"), "csv") {
			_, _ = w.Write([]byte("\"id\";\"geom\"\n1;\"SRID=4326;POINT(13.7 51.0)\"\n"))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "FeatureCollection", "features": []any{}})
	})
	mux.HandleFunc("/dcat-ap/dataset/geborene/content.json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"Jahr": "2020"}}})
	})
	mux.HandleFunc("/broken", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	return fake
}

func TestSearchDatasets(t *testing.T) {
	fake := newFakePortal(t)
	result, err := fake.client.SearchDatasets(context.Background(), "wanderwege", 10, 0)
	if err != nil {
		t.Fatal(err)
	}

	req := fake.lastSearch
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
		},
	})
	if string(got) != string(want) {
		t.Errorf("dataset\n got %s\nwant %s", got, want)
	}
}

func TestSearchDatasetsPaging(t *testing.T) {
	fake := newFakePortal(t)
	result, err := fake.client.SearchDatasets(context.Background(), "", 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 3 || len(result.Datasets) != 2 || result.Datasets[0].ID != "D2" || result.Datasets[1].ID != "D3" {
		t.Errorf("unexpected page %+v", result)
	}
}

func TestGetDataset(t *testing.T) {
	fake := newFakePortal(t)
	ds, err := fake.client.GetDataset(context.Background(), "D2")
	if err != nil {
		t.Fatal(err)
	}
	if attrs := fake.lastSearch.TextSearchConfig.Attribute; len(attrs) != 1 || attrs[0].Name != datasetIDField {
		t.Errorf("lookup searched %v, want only %s", attrs, datasetIDField)
	}
	if ds.Title != "Geborene nach Geschlecht 2020" || ds.LayerID != "" {
		t.Errorf("unexpected dataset %+v", ds)
	}
	wantTable := fake.URL + "/aswdb/asw.dll/?aw=Bev%C3%B6lkerung/Geborene%20nach%20Geschlecht_2020_TAB"
	if len(ds.Resources) != 2 || ds.Resources[1].URL != wantTable {
		t.Errorf("table resource = %+v, want URL %s", ds.Resources, wantTable)
	}

	if _, err := fake.client.GetDataset(context.Background(), "nope"); err == nil {
		t.Error("expected error for unknown dataset, got nil")
	}
}

func TestFetchResource(t *testing.T) {
	fake := newFakePortal(t)
	ctx := context.Background()
	geo, _ := fake.client.GetDataset(ctx, "D1")
	stats, _ := fake.client.GetDataset(ctx, "D2")
	broken, _ := fake.client.GetDataset(ctx, "D3")
	limit := url.Values{"limit": {"5"}}

	data, err := fake.client.FetchResource(ctx, geo, "csv", nil)
	if err != nil || !strings.HasPrefix(string(data), "\"id\";") {
		t.Errorf("CSV fetch: err=%v data=%q", err, data)
	}
	if fake.lastQuery != "format=csv/ewkt&delimiter=semicolon" {
		t.Errorf("CSV query = %q", fake.lastQuery)
	}

	if _, err := fake.client.FetchResource(ctx, geo, "GeoJSON", limit); err != nil {
		t.Fatal(err)
	}
	if fake.lastQuery != "limit=5" {
		t.Errorf("GeoJSON query = %q, want limit=5", fake.lastQuery)
	}
	if _, err := fake.client.FetchResource(ctx, geo, "CSV", limit); err != nil {
		t.Fatal(err)
	}
	if fake.lastQuery != "format=csv/ewkt&delimiter=semicolon&limit=5" {
		t.Errorf("CSV query with limit = %q", fake.lastQuery)
	}
	if _, err := fake.client.FetchResource(ctx, geo, "GeoJSON", url.Values{"bbox": {"13.7,51.0,13.8,51.1"}}); err != nil {
		t.Fatal(err)
	}
	if got := fake.lastQuery; got != "bbox=13.7,51.0,13.8,51.1" && got != "bbox=13.7%2C51.0%2C13.8%2C51.1" {
		t.Errorf("GeoJSON query with bbox = %q", got)
	}

	data, err = fake.client.FetchResource(ctx, stats, "JSON", limit)
	if err != nil || !strings.Contains(string(data), `"Jahr"`) {
		t.Errorf("JSON fetch: err=%v data=%q", err, data)
	}

	if _, err := fake.client.FetchResource(ctx, stats, "CSV", nil); err == nil {
		t.Error("expected error for unavailable format, got nil")
	}
	if _, err := fake.client.FetchResource(ctx, broken, "CSV", nil); err == nil {
		t.Error("expected error for HTTP 500, got nil")
	}
}
