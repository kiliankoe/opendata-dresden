package index

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kiliankoe/opendata-dresden/internal/config"
	"github.com/kiliankoe/opendata-dresden/internal/portal"
	"github.com/kiliankoe/opendata-dresden/internal/portal/portaltest"
)

// indexServer builds an index from the fake portal and serves it over HTTP,
// counting the downloads
func indexServer(t *testing.T, fake *portaltest.Server) (*httptest.Server, *int) {
	t.Helper()
	idx, _, err := Build(context.Background(), &config.Config{PortalURL: fake.URL, ArcGISURL: fake.URL}, &Index{})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := idx.Write(filepath.Join(dir, "index.json")); err != nil {
		t.Fatal(err)
	}
	downloads := 0
	files := http.FileServer(http.Dir(dir))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads++
		files.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	return server, &downloads
}

func ids(result *portal.SearchResult) []string {
	var ids []string
	for _, ds := range result.Datasets {
		ids = append(ids, ds.ID)
	}
	return ids
}

func TestSearch(t *testing.T) {
	fake := portaltest.New(t)
	server, _ := indexServer(t, fake)
	catalog := NewCatalog(&config.Config{PortalURL: fake.URL, ArcGISURL: fake.URL, IndexURL: server.URL + "/index.json"})
	ctx := context.Background()

	tests := []struct {
		query         string
		limit, offset int
		want          []string
		total         int
	}{
		{"", 30, 0, []string{"D2", "a1b2-0", "D3", "D1"}, 4}, // everything, by title
		{"", 1, 1, []string{"a1b2-0"}, 4},
		{"wanderwege", 30, 0, []string{"D1"}, 1},
		{"geborene geschlecht", 30, 0, []string{"D2"}, 1},
		{"geborene wanderwege", 30, 0, nil, 0},     // all words must match
		{"Bevoelkerung", 30, 0, []string{"D2"}, 1}, // umlauts fold both ways
		{"umweltamt", 30, 0, []string{"D1"}, 1},    // source and description count too
		{"D3", 30, 0, []string{"D3"}, 1},
		{"arcgis", 30, 0, []string{"a1b2-0"}, 1}, // the portal counts like a topic
	}
	for _, tt := range tests {
		result, err := catalog.Search(ctx, tt.query, tt.limit, tt.offset)
		if err != nil {
			t.Fatal(err)
		}
		if got := ids(result); result.Total != tt.total || len(got) != len(tt.want) || (len(got) > 0 && got[0] != tt.want[0]) {
			t.Errorf("Search(%q, %d, %d) = %v (total %d), want %v (total %d)", tt.query, tt.limit, tt.offset, got, result.Total, tt.want, tt.total)
		}
	}
}

func TestSearchRanksTitleFirst(t *testing.T) {
	idx := &Index{Datasets: []portal.Dataset{
		{ID: "A", Title: "Bäume", Description: "Straßenbäume im Stadtgebiet"},
		{ID: "B", Title: "Straßenbäume", Description: "Bäume an Straßen"},
	}}
	if got := ids(idx.Search("strassenbaeume", 10, 0)); len(got) != 2 || got[0] != "B" {
		t.Errorf("got %v, want B first", got)
	}
}

func TestCatalogCache(t *testing.T) {
	fake := portaltest.New(t)
	server, downloads := indexServer(t, fake)
	cfg := &config.Config{PortalURL: fake.URL, ArcGISURL: fake.URL, IndexURL: server.URL + "/index.json", CacheDir: t.TempDir()}
	ctx := context.Background()

	if _, err := NewCatalog(cfg).Search(ctx, "", 1, 0); err != nil || *downloads != 1 {
		t.Fatalf("first load: err=%v downloads=%d", err, *downloads)
	}
	// A fresh cache serves the next process without a download
	if _, err := NewCatalog(cfg).Search(ctx, "", 1, 0); err != nil || *downloads != 1 {
		t.Errorf("cached load: err=%v downloads=%d", err, *downloads)
	}
	// A stale cache is refreshed
	cache := filepath.Join(cfg.CacheDir, "index.json")
	old := time.Now().Add(-2 * maxAge)
	if err := os.Chtimes(cache, old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := NewCatalog(cfg).Search(ctx, "", 1, 0); err != nil || *downloads != 2 {
		t.Errorf("stale load: err=%v downloads=%d", err, *downloads)
	}
	// Without network the stale cache is better than nothing
	if err := os.Chtimes(cache, old, old); err != nil {
		t.Fatal(err)
	}
	server.Close()
	if result, err := NewCatalog(cfg).Search(ctx, "", 1, 0); err != nil || result.Total != 4 {
		t.Errorf("offline load: err=%v result=%+v", err, result)
	}

	if _, err := NewCatalog(&config.Config{IndexURL: server.URL + "/index.json"}).Search(ctx, "", 1, 0); err == nil {
		t.Error("expected error without index and cache, got nil")
	}
}

func TestCatalogGetAndFetch(t *testing.T) {
	fake := portaltest.New(t)
	server, _ := indexServer(t, fake)
	catalog := NewCatalog(&config.Config{PortalURL: fake.URL, ArcGISURL: fake.URL, IndexURL: server.URL + "/index.json"})
	ctx := context.Background()

	ds, err := catalog.Get(ctx, "D1")
	if err != nil || ds.Description == "" {
		t.Errorf("Get(D1): err=%v dataset=%+v, want the described index entry", err, ds)
	}
	searches := fake.SearchRequests

	// Datasets the nightly index does not know yet are looked up in the portal
	catalog.index.Datasets = catalog.index.Datasets[:2]
	delete(catalog.byID, "D3")
	if ds, err := catalog.Get(ctx, "D3"); err != nil || ds.Title != "Kaputter Datensatz" || fake.SearchRequests != searches+1 {
		t.Errorf("Get(D3): err=%v dataset=%+v searches=%d", err, ds, fake.SearchRequests-searches)
	}
	if _, err := catalog.Get(ctx, "nope"); err == nil {
		t.Error("expected error for unknown dataset, got nil")
	}

	data, err := catalog.Fetch(ctx, "D1", "geojson", portal.FetchOptions{Limit: 2})
	if err != nil || fake.LastQuery != "limit=2" {
		t.Errorf("Fetch: err=%v data=%q query=%q", err, data, fake.LastQuery)
	}
	// ArcGIS layers are fetched from their own service
	if _, err := catalog.Fetch(ctx, "a1b2-0", "geojson", portal.FetchOptions{Limit: 2}); err != nil || len(fake.LayerQueries) != 1 {
		t.Errorf("Fetch ArcGIS: err=%v queries=%q", err, fake.LayerQueries)
	}
}
