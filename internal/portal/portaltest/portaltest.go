// Package portaltest serves a fake OpenData portal and a fake ArcGIS Online
// organization for tests. The portal answers search requests from three
// fixture datasets and serves their resources; the organization has one
// feature service with a single layer.
package portaltest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// datasetIDField is the search index field the client uses for ID lookups
const datasetIDField = "ergebnis_id"

// Request is the search request body the fake records
type Request struct {
	TextSearch       string   `json:"textSearch"`
	NumOfResults     int      `json:"numOfResults"`
	PagingStart      int      `json:"pagingStart"`
	UserGroupIDs     []string `json:"userGroupIds"`
	TextSearchConfig struct {
		Attribute []struct {
			Name string `json:"name"`
		} `json:"attribute"`
	} `json:"textSearchConfig"`
}

type Server struct {
	*httptest.Server
	LastSearch     Request  // body of the most recent search request
	SearchRequests int      // number of search requests served
	LastQuery      string   // raw query string of the most recent OGC items request
	InfoRequests   int      // number of information page requests served
	LayerQueries   []string // raw query strings of the feature layer queries served
}

// arcgisItems is the ArcGIS Online search result for the organization's
// feature services
const arcgisItems = `{"total":1,"start":1,"num":100,"nextStart":-1,"results":[
 {"id":"a1b2","title":"Haltestellen","type":"Feature Service","owner":"afguk_e1",
  "modified":1757980800000,"snippet":"Haltestellen des ÖPNV",
  "description":"<p>Alle <b>Haltestellen</b> in Dresden.</p>","licenseInfo":"<span>Beachten Sie die Nutzungsbedingungen.</span>",
  "accessInformation":"Landeshauptstadt Dresden","tags":["Dresden"],
  "url":"{{base}}/rest/services/Haltestellen/FeatureServer"}]}`

// infoPage mimics the metadata page of a geodata layer on kommisdd.dresden.de
const infoPage = `<html><body><table>
<tr class="mainsection"><td class="mainsection" colspan="2">Beschreibung </td></tr>
<tr class="data0"><td class="caption0">Name </td><td class="value0">Stadtteil-Wanderwege
        </td></tr>
<tr class="data0"><td class="caption0">Beschreibung</td><td class="value0">
  <p class="text-absatz">Wanderwege durch die Dresdner Stadtteile.<br>Stand 2024.</p>
</td></tr>
<tr class="data0"><td class="caption0">Herkunft</td><td class="value0">
  <p class="text-absatz">Erhoben durch das Umweltamt. </p>
</td></tr>
</table></body></html>`

// fixtures replicate the portal's search results for a geodata layer (D1), a
// statistics table (D2) and a dataset whose download is broken (D3).
const fixtures = `[
 {"bezeichnung":"Stadtteil-Wanderwege","letzteAenderung":"13.03.2024",
  "themen":["Umwelt und Klima","Umwelt und Klima"],"raeume":["Dresden"],"zeiten":["2024","2024"],
  "dataSource":{"name":"Umweltamt"},"presentationLicense":{"name":"dl-de/by-2-0"},
  "presentations":[
   {"ergebnisId":"D1","darstellungsArtBezeichnung":"CSV","schnittstelle":"{{base}}/ogcapi/collections/L1527/items?format=csv/ewkt&delimiter=semicolon","aufrufUrl":null},
   {"ergebnisId":"D1","darstellungsArtBezeichnung":"GEOJSON","schnittstelle":"{{base}}/ogcapi/collections/L1527","aufrufUrl":null},
   {"ergebnisId":"D1","darstellungsArtBezeichnung":"WFS","schnittstelle":"{{base}}/ogcsl.ashx?nodeid=1959&service=wfs","aufrufUrl":null},
   {"ergebnisId":"D1","darstellungsArtBezeichnung":"Information","schnittstelle":"{{base}}/ogc.ashx?Service=Ikx&RenderHint=TargetHtml&NODEID=1959&","aufrufUrl":null}]},
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

type dataset struct {
	raw           json.RawMessage
	title         string
	presentations []struct {
		DatasetID string `json:"ergebnisId"`
	}
}

func New(t *testing.T) *Server {
	t.Helper()
	fake := &Server{}
	mux := http.NewServeMux()
	fake.Server = httptest.NewServer(mux)
	t.Cleanup(fake.Close)

	var raw []json.RawMessage
	if err := json.Unmarshal([]byte(strings.ReplaceAll(fixtures, "{{base}}", fake.URL)), &raw); err != nil {
		t.Fatal(err)
	}
	datasets := make([]dataset, len(raw))
	for i, r := range raw {
		var meta struct {
			Title         string `json:"bezeichnung"`
			Presentations []struct {
				DatasetID string `json:"ergebnisId"`
			} `json:"presentations"`
		}
		if err := json.Unmarshal(r, &meta); err != nil {
			t.Fatal(err)
		}
		datasets[i] = dataset{raw: r, title: meta.Title, presentations: meta.Presentations}
	}

	mux.HandleFunc("POST /service/app/search/all", func(w http.ResponseWriter, r *http.Request) {
		fake.SearchRequests++
		if err := json.NewDecoder(r.Body).Decode(&fake.LastSearch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		req := fake.LastSearch
		byID := len(req.TextSearchConfig.Attribute) == 1 && req.TextSearchConfig.Attribute[0].Name == datasetIDField
		var matches []json.RawMessage
		for _, ds := range datasets {
			if byID && ds.presentations[0].DatasetID == req.TextSearch ||
				!byID && strings.Contains(strings.ToLower(ds.title), strings.ToLower(req.TextSearch)) {
				matches = append(matches, ds.raw)
			}
		}
		total := len(matches)
		matches = matches[min(req.PagingStart, total):]
		matches = matches[:min(req.NumOfResults, len(matches))]
		_ = json.NewEncoder(w).Encode(map[string]any{"numOfResults": total, "ipResults": matches})
	})
	mux.HandleFunc("/ogcapi/collections/L1527/items", func(w http.ResponseWriter, r *http.Request) {
		fake.LastQuery = r.URL.RawQuery
		if strings.HasPrefix(r.URL.Query().Get("format"), "csv") {
			_, _ = w.Write([]byte("\"id\";\"geom\"\n1;\"SRID=4326;POINT(13.7 51.0)\"\n"))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "FeatureCollection", "features": []any{}})
	})
	mux.HandleFunc("/dcat-ap/dataset/geborene/content.json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"Jahr": "2020"}}})
	})
	mux.HandleFunc("/ogc.ashx", func(w http.ResponseWriter, r *http.Request) {
		fake.InfoRequests++
		_, _ = w.Write([]byte(infoPage))
	})
	mux.HandleFunc("/broken", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})

	mux.HandleFunc("/sharing/rest/search", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.ReplaceAll(arcgisItems, "{{base}}", fake.URL)))
	})
	mux.HandleFunc("/rest/services/Haltestellen/FeatureServer", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"layers": []any{map[string]any{"id": 0, "name": "Haltestellen"}}})
	})
	mux.HandleFunc("/rest/services/Haltestellen/FeatureServer/0", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"editingInfo": map[string]any{"lastEditDate": 1758067200000}})
	})
	// The layer holds two features but hands out one per page, like the
	// live service does with its transfer limit
	mux.HandleFunc("/rest/services/Haltestellen/FeatureServer/0/query", func(w http.ResponseWriter, r *http.Request) {
		fake.LayerQueries = append(fake.LayerQueries, r.URL.RawQuery)
		feature := func(id int) map[string]any {
			return map[string]any{"type": "Feature", "id": id, "geometry": map[string]any{"type": "Point", "coordinates": []float64{13.7, 51.0}}, "properties": map[string]any{"Name": "Stop"}}
		}
		page := map[string]any{"type": "FeatureCollection", "features": []any{feature(1)}, "properties": map[string]any{"exceededTransferLimit": true}}
		if r.URL.Query().Get("resultOffset") == "1" {
			page = map[string]any{"type": "FeatureCollection", "features": []any{feature(2)}}
		}
		_ = json.NewEncoder(w).Encode(page)
	})
	return fake
}
