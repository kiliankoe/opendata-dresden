package archive

import (
	"fmt"
	"testing"
	"time"

	"github.com/kiliankoe/opendata-dresden/internal/portal"
)

// requested reports whether path was requested since the last call
func (s *stubServer) requested(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.ifNoneMatch[path]
	delete(s.ifNoneMatch, path)
	return ok
}

// Time series layers keep their own history, so the archive keeps one
// snapshot of them per UTC year and carries it forward in between
func TestRunSeriesLayersYearly(t *testing.T) {
	s := newStubServer(t)
	a, b, c, items := s.URL+"/a", s.URL+"/b", s.URL+"/c", s.URL+"/items"
	idx := layerIndex(a, b, c, items)
	all := idx.Datasets
	dir := t.TempDir()
	cet := time.FixedZone("CET", 3600)

	type check struct {
		path  string
		fetch bool
	}
	step := func(name string, stamp time.Time, checks ...check) []Entry {
		t.Helper()
		at(t, stamp)
		entries := readManifest(t, run(t, idx, dir).Manifest)
		for _, ch := range checks {
			if got := s.requested(ch.path); got != ch.fetch {
				t.Errorf("%s: %s requested %v, want %v", name, ch.path, got, ch.fetch)
			}
		}
		// The ordinary layer is fetched and stored as rows every run
		if e := entry(t, entries, items); e.Fetched != stamp.UTC().Format(time.RFC3339) || e.Rows == "" {
			t.Errorf("%s: ordinary layer entry %+v", name, e)
		}
		return entries
	}
	raw := func(name string, e Entry, body string, stamp time.Time) {
		t.Helper()
		if e.Error != "" || e.Status != 200 || e.SHA256 != sum([]byte(body)) || e.Size != int64(len(body)) ||
			e.Rows != "" || e.Delta != "" || e.DeltaFrom != "" || e.Fetched != stamp.UTC().Format(time.RFC3339) {
			t.Errorf("%s: entry %+v, want %q downloaded and stored raw", name, e, body)
			return
		}
		if got := object(t, dir, e.SHA256); string(got) != body {
			t.Errorf("%s: stored %q, want the bytes as served %q", name, got, body)
		}
	}
	serve := func(version int) {
		for _, path := range []string{"/a", "/b", "/c", "/items"} {
			s.serveTable(path, fmt.Sprintf("id\n%d\n1\n", version))
		}
	}

	// Before /a is a time series it is archived as rows, like any layer
	serve(1)
	idx.Datasets = []portal.Dataset{all[0], all[3]}
	first := time.Date(2026, 12, 30, 2, 0, 0, 0, time.UTC)
	entries := step("legacy", first, check{"/a", true})
	legacy := entry(t, entries, a)
	if legacy.Rows == "" || legacy.Delta == "" {
		t.Fatalf("legacy entry %+v, want it stored as rows", legacy)
	}

	// Already the new year in local time, still the old one in UTC. The
	// previous entry of /a is carried without its delta; /b has none to carry.
	serve(2)
	all[0].LayerID, all[1].LayerID, all[2].LayerID = "L1625", "L1780", "L1781"
	idx.Datasets = []portal.Dataset{all[0], all[1], all[3]}
	second := time.Date(2027, 1, 1, 0, 30, 0, 0, cet)
	entries = step("same year", second, check{"/a", false}, check{"/b", true})
	want := legacy
	want.Delta, want.DeltaFrom = "", ""
	if e := entry(t, entries, a); e != want {
		t.Errorf("same year: carried entry\n%+v\nwant\n%+v", e, want)
	}
	raw("same year /b", entry(t, entries, b), "id\n2\n1\n", second)

	// The new year's first run downloads every series again
	serve(3)
	s.set("/b", stubFile{body: []byte("Fehler"), status: 500, contentType: "text/csv"})
	idx.Datasets = all
	third := time.Date(2027, 1, 1, 1, 30, 0, 0, time.UTC)
	entries = step("new year", third, check{"/a", true}, check{"/b", true}, check{"/c", true})
	newYear := entry(t, entries, a)
	raw("new year /a", newYear, "id\n3\n1\n", third)
	raw("new year /c", entry(t, entries, c), "id\n3\n1\n", third)
	if e := entry(t, entries, b); e.Error == "" {
		t.Errorf("new year: failed entry %+v, want an error", e)
	}

	// A failed series is retried; the others are carried, also a second time
	serve(4)
	fourth := time.Date(2027, 1, 2, 1, 30, 0, 0, time.UTC)
	entries = step("retry", fourth, check{"/a", false}, check{"/b", true}, check{"/c", false})
	raw("retry /b", entry(t, entries, b), "id\n4\n1\n", fourth)
	if e := entry(t, entries, a); e != newYear {
		t.Errorf("retry: carried entry\n%+v\nwant\n%+v", e, newYear)
	}

	serve(5)
	entries = step("carried again", time.Date(2027, 1, 3, 1, 30, 0, 0, time.UTC),
		check{"/a", false}, check{"/b", false}, check{"/c", false})
	if e := entry(t, entries, a); e != newYear {
		t.Errorf("carried again: entry\n%+v\nwant\n%+v", e, newYear)
	}
}
