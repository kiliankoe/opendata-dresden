package index

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kiliankoe/opendata-dresden/internal/config"
	"github.com/kiliankoe/opendata-dresden/internal/portal/portaltest"
)

// fakeRepo serves the two GitHub endpoints History uses and records the paths
// it was asked for
func fakeRepo(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var asked []string
	mux := http.NewServeMux()
	mux.HandleFunc("/commits", func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.String())
		_ = json.NewEncoder(w).Encode([]any{
			map[string]any{"sha": "bbb", "commit": map[string]any{
				"message": "update data", "committer": map[string]any{"date": "2026-09-18T03:21:07Z"}}},
			map[string]any{"sha": "aaa", "commit": map[string]any{
				"message": "add statistics mirror", "committer": map[string]any{"date": "2026-09-17T09:00:00Z"}}},
		})
	})
	mux.HandleFunc("/commits/", func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.String())
		sha := strings.TrimPrefix(r.URL.Path, "/commits/")
		files := []any{
			map[string]any{"filename": "data/statistics/other.csv", "patch": "wrong file"},
			map[string]any{"filename": "data/statistics/geborene.csv", "patch": "@@ -2 +2 @@\n-2020;Altstadt;123\n+2020;Altstadt;124"},
		}
		// GitHub leaves the patch out of changes it considers too large
		if sha == "aaa" {
			files = []any{map[string]any{"filename": "data/statistics/geborene.csv"}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"files": files, "sha": sha})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, &asked
}

func TestHistory(t *testing.T) {
	fake := portaltest.New(t)
	index, _ := indexServer(t, fake)
	repo, asked := fakeRepo(t)
	catalog := NewCatalog(&config.Config{PortalURL: fake.URL, IndexURL: index.URL + "/index.json", RepoAPI: repo.URL})
	ctx := context.Background()

	history, err := catalog.History(ctx, "D2", 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if history.Title != "Geborene nach Geschlecht 2020" || len(history.Changes) != 2 {
		t.Fatalf("got %+v, want two changes of D2", history)
	}
	if got := history.Changes[0]; got.Date != "2026-09-18" || got.Commit != "bbb" || got.Message != "update data" || got.Diff != "" {
		t.Errorf("newest change = %+v", got)
	}
	// One request, asking only for this dataset's table
	if len(*asked) != 1 || !strings.Contains((*asked)[0], "path=data%2Fstatistics%2Fgeborene.csv") {
		t.Errorf("requests = %v", *asked)
	}

	// --diff adds the patch of this file only, one request per change
	history, err = catalog.History(ctx, "D2", 10, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := history.Changes[0].Diff; !strings.Contains(got, "+2020;Altstadt;124") || strings.Contains(got, "wrong file") {
		t.Errorf("diff = %q", got)
	}
	if got := history.Changes[1].Diff; !strings.Contains(got, "too large") {
		t.Errorf("omitted patch = %q, want it to say so", got)
	}
	if len(*asked) != 4 {
		t.Errorf("got %d requests, want 1 listing plus 2 commits after the first call", len(*asked))
	}

	// Geodata layers are not mirrored, so there is nothing to report
	if _, err := catalog.History(ctx, "D1", 10, false); err == nil || !strings.Contains(err.Error(), "not mirrored") {
		t.Errorf("geodata: err=%v, want it to say the dataset is not mirrored", err)
	}
}
