package archive

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kiliankoe/opendata-dresden/internal/index"
	"github.com/kiliankoe/opendata-dresden/internal/portal"
	"github.com/klauspost/compress/zstd"
)

// The ref helpers compute a table's canonical form and deltas independently
// of the implementation, as the oracle of the tests below.

// refRecords splits content at newlines outside double quotes. An escaped
// quote "" toggles twice and so leaves the state as it was.
func refRecords(content []byte) [][]byte {
	var records [][]byte
	quoted, start := false, 0
	for i, c := range content {
		switch {
		case c == '"':
			quoted = !quoted
		case c == '\n' && !quoted:
			records = append(records, content[start:i])
			start = i + 1
		}
	}
	if start < len(content) {
		records = append(records, content[start:])
	}
	return records
}

func refCanonical(content []byte) []byte {
	records := refRecords(content)
	rest := slices.Clone(records[1:])
	slices.SortFunc(rest, bytes.Compare)
	var b bytes.Buffer
	for _, r := range append([][]byte{records[0]}, rest...) {
		b.Write(r)
		b.WriteByte('\n')
	}
	return b.Bytes()
}

// refDelta is the delta from the canonical table base to next; a nil base
// gives the full version
func refDelta(base, next []byte) []byte {
	count := map[string]int{}
	if base != nil {
		for _, r := range refRecords(base)[1:] {
			count[string(r)]--
		}
	}
	records := refRecords(next)
	for _, r := range records[1:] {
		count[string(r)]++
	}
	var b bytes.Buffer
	b.Write(records[0])
	b.WriteByte('\n')
	for _, r := range slices.Sorted(maps.Keys(count)) {
		for n := count[r]; n < 0; n++ {
			b.WriteString("-" + r + "\n")
		}
		for n := count[r]; n > 0; n-- {
			b.WriteString("+" + r + "\n")
		}
	}
	return b.Bytes()
}

// The oracle itself must read the spec right, or every test below is moot
func TestReferenceCanonicalForm(t *testing.T) {
	for content, want := range map[string]string{
		"h\nb\na\n":                 "h\na\nb\n",
		"h\nb\na":                   "h\na\nb\n",
		"h\r\nb\r\na\r\n":           "h\r\na\r\nb\r\n",
		"h\n\"x\ny\";1\na\n":        "h\n\"x\ny\";1\na\n",
		"h\n\"q\"\"\n\";2\n\"q\"\n": "h\n\"q\"\n\"q\"\"\n\";2\n",
		"h\nb\nb\na\n":              "h\na\nb\nb\n",
		"h\n1;a\t\n1;a\n":           "h\n1;a\n1;a\t\n",
	} {
		if got := string(refCanonical([]byte(content))); got != want {
			t.Errorf("canonical of %q is %q, want %q", content, got, want)
		}
	}
	base := []byte("h\na\nb\nb\nc\n")
	next := []byte("h2\nb\nc\nd\n")
	if got, want := string(refDelta(base, next)), "h2\n-a\n-b\n+d\n"; got != want {
		t.Errorf("delta %q, want %q", got, want)
	}
	if got, want := string(refDelta(nil, next)), "h2\n+b\n+c\n+d\n"; got != want {
		t.Errorf("full delta %q, want %q", got, want)
	}
}

// stubFile is a response of stubServer; unlike fakeServer it sets a Content-Type
type stubFile struct {
	body        []byte
	status      int // 200 when zero
	contentType string
	etag        string
}

type stubServer struct {
	*httptest.Server
	mu          sync.Mutex
	files       map[string]stubFile
	ifNoneMatch map[string]string // the last If-None-Match per path
}

func newStubServer(t *testing.T) *stubServer {
	t.Helper()
	s := &stubServer{files: map[string]stubFile{}, ifNoneMatch: map[string]string{}}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		f, ok := s.files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		s.ifNoneMatch[r.URL.Path] = r.Header.Get("If-None-Match")
		w.Header().Set("Content-Type", f.contentType)
		if f.etag != "" {
			w.Header().Set("ETag", f.etag)
			if r.Header.Get("If-None-Match") == f.etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
		if f.status != 0 {
			w.WriteHeader(f.status)
		}
		_, _ = w.Write(f.body)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *stubServer) set(path string, f stubFile) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[path] = f
}

// serveTable serves a layer's CSV the way the portal does
func (s *stubServer) serveTable(path, body string) {
	s.set(path, stubFile{body: []byte(body), contentType: "text/csv; charset=utf-8"})
}

func layerIndex(urls ...string) *index.Index {
	idx := &index.Index{}
	for i, url := range urls {
		idx.Datasets = append(idx.Datasets, portal.Dataset{ID: fmt.Sprint("D", i), LayerID: fmt.Sprint("L", i),
			Resources: []portal.Resource{{Format: "CSV", URL: url}}})
	}
	return idx
}

func hasObject(dir, sha string) bool {
	_, err := os.Stat(filepath.Join(dir, "objects", sha[:2], sha[2:]+".zst"))
	return err == nil
}

func tableBytes(t *testing.T, dir, rows string) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := Table(dir, rows, &b); err != nil {
		t.Fatalf("Table(%s): %v", rows, err)
	}
	return b.Bytes()
}

// completedEntries are the entries of all completed manifests in run order
func completedEntries(t *testing.T, dir string) []Entry {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "runs", "*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var entries []Entry
	for _, path := range paths {
		entries = append(entries, readManifest(t, path)...)
	}
	return entries
}

// checkTables reconstructs every table version the completed manifests name
func checkTables(t *testing.T, dir string, known map[string][]byte) {
	t.Helper()
	checked := map[string]bool{}
	for _, e := range completedEntries(t, dir) {
		if e.Rows == "" || checked[e.Rows] {
			continue
		}
		checked[e.Rows] = true
		want, ok := known[e.Rows]
		if !ok {
			t.Fatalf("entry %+v names a table that was never served", e)
		}
		if got := tableBytes(t, dir, e.Rows); !bytes.Equal(got, want) {
			t.Fatalf("Table(%s) =\n%q\nwant\n%q", e.Rows, got, want)
		}
	}
}

// checkDelta asserts that e stores next as a delta against base, a full
// version when base is nil
func checkDelta(t *testing.T, dir string, e Entry, base, next []byte) {
	t.Helper()
	wantFrom := ""
	if base != nil {
		wantFrom = sum(base)
	}
	if e.Rows != sum(next) || e.DeltaFrom != wantFrom || e.Delta == "" {
		t.Fatalf("entry %+v, want rows %s as a delta from %q", e, sum(next), wantFrom)
	}
	got := object(t, dir, e.Delta)
	if sum(got) != e.Delta {
		t.Errorf("delta object %s holds content hashing to %s", e.Delta, sum(got))
	}
	if want := refDelta(base, next); !bytes.Equal(got, want) {
		t.Errorf("delta\n%q\nwant\n%q", got, want)
	}
}

func checkNoDelta(t *testing.T, e Entry, rows string) {
	t.Helper()
	if e.Rows != rows || e.Delta != "" || e.DeltaFrom != "" {
		t.Errorf("entry %+v, want rows %s without a delta", e, rows)
	}
}

func TestRunLayerTableExample(t *testing.T) {
	s := newStubServer(t)
	url := s.URL + "/items"
	idx := &index.Index{Datasets: []portal.Dataset{{ID: "D1", LayerID: "L1",
		Resources: []portal.Resource{{Format: "csv", URL: url + "?format=csv"}}}}}
	url += "?format=csv"
	dir := t.TempDir()

	nights := []struct{ body, canonical, delta string }{
		// A quoted field holds a newline and an escaped quote
		{"id;name\n2;b\n1;\"x\ny\"\n4;\"q\"\"r\"\n",
			"id;name\n1;\"x\ny\"\n2;b\n4;\"q\"\"r\"\n",
			"id;name\n+1;\"x\ny\"\n+2;b\n+4;\"q\"\"r\"\n"},
		// One record left, one was duplicated, one joined; no final newline
		{"id;name\n2;b\n3;c\n4;\"q\"\"r\"\n2;b",
			"id;name\n2;b\n2;b\n3;c\n4;\"q\"\"r\"\n",
			"id;name\n-1;\"x\ny\"\n+2;b\n+3;c\n"},
		// The same records in another order
		{"id;name\n4;\"q\"\"r\"\n3;c\n2;b\n2;b\n", "id;name\n2;b\n2;b\n3;c\n4;\"q\"\"r\"\n", ""},
		// The same bytes again
		{"id;name\n4;\"q\"\"r\"\n3;c\n2;b\n2;b\n", "id;name\n2;b\n2;b\n3;c\n4;\"q\"\"r\"\n", ""},
		// One of two copies left
		{"id;name\n3;c\n2;b\n4;\"q\"\"r\"\n", "id;name\n2;b\n3;c\n4;\"q\"\"r\"\n", "id;name\n-2;b\n"},
	}
	var previous string
	for i, night := range nights {
		s.serveTable("/items", night.body)
		before := objects(t, dir)
		at(t, day.Add(time.Duration(i)*24*time.Hour))
		summary := run(t, idx, dir)
		e := entry(t, readManifest(t, summary.Manifest), url)

		if e.Status != 200 || e.SHA256 != sum([]byte(night.body)) || e.Size != int64(len(night.body)) || e.Error != "" {
			t.Errorf("night %d: entry %+v, want the served bytes described", i, e)
		}
		if hasObject(dir, e.SHA256) {
			t.Errorf("night %d: the served bytes were stored", i)
		}
		if e.Rows != sum([]byte(night.canonical)) {
			t.Errorf("night %d: rows %s, want the hash of %q", i, e.Rows, night.canonical)
		}
		switch {
		case night.delta == "":
			checkNoDelta(t, e, previous)
			if summary.NewObjects != 0 || len(objects(t, dir)) != len(before) {
				t.Errorf("night %d: an unchanged table stored %d objects", i, summary.NewObjects)
			}
		case previous == "":
			if e.DeltaFrom != "" {
				t.Errorf("night %d: first version %+v is not full", i, e)
			}
		default:
			if e.DeltaFrom != previous {
				t.Errorf("night %d: delta from %s, want %s", i, e.DeltaFrom, previous)
			}
		}
		if night.delta != "" {
			if got := object(t, dir, e.Delta); string(got) != night.delta {
				t.Errorf("night %d: delta %q, want %q", i, got, night.delta)
			}
		}
		if got := tableBytes(t, dir, e.Rows); string(got) != night.canonical {
			t.Errorf("night %d: Table %q, want %q", i, got, night.canonical)
		}
		previous = e.Rows
	}
	// Every version stays reconstructible, not only the latest
	if got := tableBytes(t, dir, sum([]byte(nights[0].canonical))); string(got) != nights[0].canonical {
		t.Errorf("first version reconstructed as %q", got)
	}
	if err := Table(dir, sum([]byte("id;name\n")), &bytes.Buffer{}); err == nil {
		t.Error("Table of a version no manifest names succeeded")
	}
}

func TestRunStoresOtherCSVsRaw(t *testing.T) {
	s := newStubServer(t)
	cases := []struct {
		name        string
		layer       string
		format      string
		contentType string
		body        string
	}{
		// Statistics keep their row order, which carries meaning
		{"statistics", "", "CSV", "text/csv", "Jahr;Wert\n2021;2\n2020;1\n"},
		{"other content type", "L1", "CSV", "text/plain; charset=utf-8", "id;n\n2;b\n1;a\n"},
		{"unterminated quote", "L2", "CSV", "text/csv", "id;n\n2;\"open\n1;a\n"},
		{"not a CSV resource", "L3", "JSON", "text/csv", "id;n\n2;c\n1;a\n"},
	}
	idx := &index.Index{}
	for i, c := range cases {
		path := fmt.Sprint("/r", i)
		s.set(path, stubFile{body: []byte(c.body), contentType: c.contentType})
		idx.Datasets = append(idx.Datasets, portal.Dataset{ID: fmt.Sprint("D", i), LayerID: c.layer,
			Resources: []portal.Resource{{Format: c.format, URL: s.URL + path}}})
	}
	dir := t.TempDir()
	at(t, day)
	entries := readManifest(t, run(t, idx, dir).Manifest)
	for i, c := range cases {
		e := entry(t, entries, fmt.Sprint(s.URL, "/r", i))
		if e.Rows != "" || e.Delta != "" || e.DeltaFrom != "" || e.SHA256 != sum([]byte(c.body)) {
			t.Errorf("%s: entry %+v, want it archived raw", c.name, e)
			continue
		}
		if got := object(t, dir, e.SHA256); string(got) != c.body {
			t.Errorf("%s: stored %q, want the bytes as served", c.name, got)
		}
	}
}

func TestRunLayerTableNotModified(t *testing.T) {
	s := newStubServer(t)
	idx := layerIndex(s.URL + "/items")
	dir := t.TempDir()
	v1 := "id\n2\n1\n"
	s.set("/items", stubFile{body: []byte(v1), contentType: "text/csv", etag: `"v1"`})
	at(t, day)
	first := entry(t, readManifest(t, run(t, idx, dir).Manifest), s.URL+"/items")
	checkDelta(t, dir, first, nil, refCanonical([]byte(v1)))

	// A 304 still vouches for the table, though its bytes are not stored raw
	at(t, day.Add(24*time.Hour))
	second := entry(t, readManifest(t, run(t, idx, dir).Manifest), s.URL+"/items")
	if s.ifNoneMatch["/items"] != `"v1"` || second.Status != 304 {
		t.Errorf("request with If-None-Match %q gave %+v, want a 304", s.ifNoneMatch["/items"], second)
	}
	checkNoDelta(t, second, first.Rows)

	v2 := "id\n3\n1\n2\n"
	s.set("/items", stubFile{body: []byte(v2), contentType: "text/csv", etag: `"v2"`})
	at(t, day.Add(48*time.Hour))
	third := entry(t, readManifest(t, run(t, idx, dir).Manifest), s.URL+"/items")
	checkDelta(t, dir, third, refCanonical([]byte(v1)), refCanonical([]byte(v2)))
}

// Entries archived before tables were stored as rows have no Rows to build on
func TestRunLayerTableAfterLegacyEntries(t *testing.T) {
	s := newStubServer(t)
	s.serveTable("/same", "id\nb\na\n")
	s.serveTable("/moved", "id\nd\nc\n")
	idx := layerIndex(s.URL+"/same", s.URL+"/moved")
	dir := t.TempDir()

	// Without a layer the CSVs are archived raw, as every CSV was before
	for i := range idx.Datasets {
		idx.Datasets[i].LayerID = ""
	}
	at(t, day)
	for _, e := range readManifest(t, run(t, idx, dir).Manifest) {
		if e.Rows != "" {
			t.Fatalf("entry %+v of a non-layer CSV has rows", e)
		}
	}

	idx = layerIndex(s.URL+"/same", s.URL+"/moved")
	s.serveTable("/moved", "id\nc\nd\n")
	at(t, day.Add(24*time.Hour))
	entries := readManifest(t, run(t, idx, dir).Manifest)
	checkDelta(t, dir, entry(t, entries, s.URL+"/same"), nil, []byte("id\na\nb\n"))
	checkDelta(t, dir, entry(t, entries, s.URL+"/moved"), nil, []byte("id\nc\nd\n"))

	// A delta needs a table larger than itself
	s.serveTable("/moved", "id\nc\nd\ne\nf\ng\n")
	at(t, day.Add(48*time.Hour))
	e := entry(t, readManifest(t, run(t, idx, dir).Manifest), s.URL+"/moved")
	checkDelta(t, dir, e, []byte("id\nc\nd\n"), []byte("id\nc\nd\ne\nf\ng\n"))
}

func TestRunLayerTableYearBoundary(t *testing.T) {
	s := newStubServer(t)
	idx := layerIndex(s.URL + "/items")
	dir := t.TempDir()
	canonical := []byte("id\n1\n2\n3\n")
	cet := time.FixedZone("CET", 3600)
	orders := []string{"id\n1\n2\n3\n", "id\n3\n2\n1\n", "id\n2\n1\n3\n", "id\n3\n1\n2\n"}

	steps := []struct {
		stamp time.Time
		full  bool
	}{
		{time.Date(2026, 12, 31, 22, 0, 0, 0, time.UTC), true},
		// Already the new year in local time, still the old one in UTC
		{time.Date(2027, 1, 1, 0, 30, 0, 0, cet), false},
		// The new year's first run stores the unchanged table in full
		{time.Date(2027, 1, 1, 1, 30, 0, 0, time.UTC), true},
		{time.Date(2027, 1, 2, 1, 30, 0, 0, time.UTC), false},
	}
	for i, step := range steps {
		s.serveTable("/items", orders[i])
		at(t, step.stamp)
		e := entry(t, readManifest(t, run(t, idx, dir).Manifest), s.URL+"/items")
		if step.full {
			checkDelta(t, dir, e, nil, canonical)
		} else {
			checkNoDelta(t, e, sum(canonical))
		}
	}

	// Later changes build on the new year's full version
	s.serveTable("/items", "id\n3\n1\n2\n4\n")
	at(t, time.Date(2027, 1, 3, 1, 30, 0, 0, time.UTC))
	e := entry(t, readManifest(t, run(t, idx, dir).Manifest), s.URL+"/items")
	checkDelta(t, dir, e, canonical, []byte("id\n1\n2\n3\n4\n"))
}

func TestRunLayerTableWithoutState(t *testing.T) {
	s := newStubServer(t)
	idx := layerIndex(s.URL + "/items")
	dir := t.TempDir()
	s.serveTable("/items", "id\n1\n2\n")
	at(t, day)
	run(t, idx, dir)

	if err := os.RemoveAll(filepath.Join(dir, "state")); err != nil {
		t.Fatal(err)
	}
	s.serveTable("/items", "id\n2\n1\n3\n")
	at(t, day.Add(24*time.Hour))
	e := entry(t, readManifest(t, run(t, idx, dir).Manifest), s.URL+"/items")
	checkDelta(t, dir, e, nil, []byte("id\n1\n2\n3\n"))
	checkTables(t, dir, map[string][]byte{
		sum([]byte("id\n1\n2\n")): []byte("id\n1\n2\n"), e.Rows: []byte("id\n1\n2\n3\n")})
}

// A table larger than chunkSize is sorted in spilled chunks, and a nightly
// append costs about as much as the appended records
func TestRunLayerTableSmallAppend(t *testing.T) {
	defer func(original int) { chunkSize = original }(chunkSize)
	chunkSize = 1 << 14
	r := rand.New(rand.NewSource(7))
	records := []string{}
	for i := range 20000 {
		records = append(records, fmt.Sprintf("%d;Straße %d;\"Ort\n%d\"", i, r.Intn(1e6), r.Intn(1e6)))
	}
	body := func() string {
		r.Shuffle(len(records), func(i, j int) { records[i], records[j] = records[j], records[i] })
		return "id;strasse;ort\n" + strings.Join(records, "\n") + "\n"
	}
	s := newStubServer(t)
	idx := layerIndex(s.URL + "/items")
	dir := t.TempDir()

	first := body()
	s.serveTable("/items", first)
	at(t, day)
	run(t, idx, dir)

	for i := range 5 {
		records = append(records, fmt.Sprintf("%d;Neu %d;\"Ort\n0\"", 20000+i, i))
	}
	second := body()
	s.serveTable("/items", second)
	at(t, day.Add(24*time.Hour))
	summary := run(t, idx, dir)
	e := entry(t, readManifest(t, summary.Manifest), s.URL+"/items")
	checkDelta(t, dir, e, refCanonical([]byte(first)), refCanonical([]byte(second)))
	if delta := object(t, dir, e.Delta); len(delta) > 200 || summary.NewBytes > 1024 {
		t.Errorf("appending 5 records to %d bytes took a %d byte delta, %d bytes on disk", len(second), len(delta), summary.NewBytes)
	}
	if got := tableBytes(t, dir, e.Rows); !bytes.Equal(got, refCanonical([]byte(second))) {
		t.Error("the appended table is not reconstructed")
	}
}

// Whatever an interrupted run stored must not become the base of a delta,
// since its manifest never completes and Table cannot follow it
func TestRunLayerTableAfterInterruptedRun(t *testing.T) {
	var mu sync.Mutex
	body, blocking := "id\n1\n2\n", false
	served, blocked := make(chan struct{}, 8), make(chan struct{}, 8)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		b, block := body, blocking
		mu.Unlock()
		switch r.URL.Path {
		case "/items":
			w.Header().Set("Content-Type", "text/csv")
			_, _ = w.Write([]byte(b))
			served <- struct{}{}
		case "/other":
			if block {
				blocked <- struct{}{}
				select {
				case <-release:
				case <-r.Context().Done():
				}
				return
			}
			_, _ = w.Write([]byte("other"))
		}
	}))
	t.Cleanup(func() { close(release); server.Close() })
	url := server.URL + "/items"
	idx := layerIndex(url)
	idx.Datasets = append(idx.Datasets, portal.Dataset{ID: "D9",
		Resources: []portal.Resource{{Format: "CSV", URL: server.URL + "/other"}}})
	dir := t.TempDir()
	known := map[string][]byte{}
	serve := func(content string) {
		mu.Lock()
		defer mu.Unlock()
		body = content
		known[sum(refCanonical([]byte(content)))] = refCanonical([]byte(content))
	}

	serve("id\n1\n2\n")
	at(t, day)
	run(t, idx, dir)
	<-served

	serve("id\n2\n3\n1\n")
	mu.Lock()
	blocking = true
	mu.Unlock()
	at(t, day.Add(24*time.Hour))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() {
		_, err := Run(ctx, http.DefaultClient, idx, dir)
		done <- err
	}()
	for _, signal := range []chan struct{}{served, blocked} {
		select {
		case <-signal:
		case <-time.After(5 * time.Second):
			t.Fatal("the run did not request both resources")
		}
	}
	// Give the run time to store the table before it is cut off
	time.Sleep(300 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if paths, _ := filepath.Glob(filepath.Join(dir, "runs", "*.jsonl")); err != nil && len(paths) != 1 {
			t.Errorf("the failed run left a completed manifest: %v", paths)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the cancelled run did not return")
	}

	// The base of the next delta is the latest table a completed run names
	var base string
	for _, e := range completedEntries(t, dir) {
		if e.URL == url && e.Rows != "" {
			base = e.Rows
		}
	}
	interrupted := sum(refCanonical([]byte("id\n2\n3\n1\n")))
	if base != interrupted {
		if err := Table(dir, interrupted, &bytes.Buffer{}); err == nil {
			t.Error("Table reconstructed a version only an interrupted run names")
		}
	}

	serve("id\n2\n3\n1\n4\n")
	mu.Lock()
	blocking = false
	mu.Unlock()
	at(t, day.Add(48*time.Hour))
	e := entry(t, readManifest(t, run(t, idx, dir).Manifest), url)
	checkDelta(t, dir, e, known[base], refCanonical([]byte("id\n2\n3\n1\n4\n")))
	checkTables(t, dir, known)
}

// tableHistory evolves one layer table night by night
type tableHistory struct {
	r       *rand.Rand
	header  string
	records []string
	next    int
	eol     string
}

func (h *tableHistory) word() string {
	const letters = "abcxyzäß019 "
	runes := []rune(letters)
	var b strings.Builder
	for range h.r.Intn(6) {
		b.WriteRune(runes[h.r.Intn(len(runes))])
	}
	return b.String()
}

func (h *tableHistory) record() string {
	fields := []string{strconv.Itoa(h.next)}
	h.next++
	for range 1 + h.r.Intn(3) {
		switch h.r.Intn(8) {
		case 0:
			fields = append(fields, `"`+h.word()+"\n"+h.word()+`"`)
		case 1:
			fields = append(fields, `"`+h.word()+`""`+h.word()+`"`)
		case 2:
			fields = append(fields, `"`+h.word()+";"+h.word()+`"`)
		case 3:
			// A tab sorts before the newline that ends a record
			fields = append(fields, h.word()+"\t")
		default:
			fields = append(fields, h.word())
		}
	}
	return strings.Join(fields, ";")
}

func (h *tableHistory) mutate() {
	r := h.r
	if r.Intn(3) == 0 || len(h.records) == 0 {
		for range 1 + r.Intn(5) {
			h.records = append(h.records, h.record())
		}
	}
	if r.Intn(5) == 0 {
		for range 1 + r.Intn(3) {
			if len(h.records) > 1 {
				i := r.Intn(len(h.records))
				h.records = slices.Delete(h.records, i, i+1)
			}
		}
	}
	if r.Intn(5) == 0 {
		h.records[r.Intn(len(h.records))] = h.record()
	}
	if r.Intn(6) == 0 {
		h.records = append(h.records, h.records[r.Intn(len(h.records))])
	}
	if r.Intn(15) == 0 {
		h.header = fmt.Sprintf("id;name;feld%d", r.Intn(100))
	}
	if r.Intn(15) == 0 {
		h.eol = map[string]string{"\n": "\r\n", "\r\n": "\n"}[h.eol]
	}
}

// serialize writes the records in a new order, as the portal does every night
func (h *tableHistory) serialize() []byte {
	r := h.r
	if r.Intn(5) != 0 {
		r.Shuffle(len(h.records), func(i, j int) { h.records[i], h.records[j] = h.records[j], h.records[i] })
	}
	var b strings.Builder
	for _, line := range append([]string{h.header}, h.records...) {
		b.WriteString(line + h.eol)
	}
	content := b.String()
	switch r.Intn(10) {
	case 0:
		content = strings.TrimSuffix(content, "\n")
	case 1:
		content = strings.TrimSuffix(content, h.eol)
	}
	return []byte(content)
}

// toggleFinalNewline changes the bytes but not the canonical table
func toggleFinalNewline(content []byte) []byte {
	if trimmed, ok := bytes.CutSuffix(content, []byte("\n")); ok {
		return trimmed
	}
	return append(slices.Clone(content), '\n')
}

func TestRunLayerTableHistory(t *testing.T) {
	for seed := int64(1); seed <= 2; seed++ {
		for _, chunk := range []int{0, 64} {
			t.Run(fmt.Sprintf("seed%d/chunk%d", seed, chunk), func(t *testing.T) {
				if chunk != 0 {
					defer func(original int) { chunkSize = original }(chunkSize)
					chunkSize = chunk
				}
				tableHistoryRun(t, seed)
			})
		}
	}
}

func tableHistoryRun(t *testing.T, seed int64) {
	r := rand.New(rand.NewSource(seed))
	h := &tableHistory{r: r, header: "id;name;feld", eol: "\n"}
	s := newStubServer(t)
	url := s.URL + "/items"
	idx := layerIndex(url)
	dir := t.TempDir()
	cet := time.FixedZone("CET", 3600)
	stamp := time.Date(2026, 12, 10, 2, 0, 0, 0, time.UTC)

	// The model of what the archive stored last
	known := map[string][]byte{}
	var lastBody, base []byte
	var baseYear int
	stateless := false

	for night := range 45 {
		if r.Intn(4) == 0 {
			stamp = stamp.Add(5 * time.Hour)
		} else {
			stamp = stamp.Add(24 * time.Hour)
		}
		if r.Intn(10) == 0 {
			if err := os.RemoveAll(filepath.Join(dir, "state")); err != nil {
				t.Fatal(err)
			}
			stateless = true
		}
		newChain := base == nil || stateless || stamp.UTC().Year() != baseYear

		failed := lastBody != nil && r.Intn(10) == 0
		var body []byte
		switch {
		case failed:
			s.set("/items", stubFile{body: []byte("Fehler"), status: 500, contentType: "text/csv"})
		case lastBody != nil && r.Intn(6) == 0:
			body = lastBody
			// Identical bytes carry the previous version over, which would
			// leave a version that must be full unwritten
			if newChain {
				body = toggleFinalNewline(lastBody)
			}
		default:
			h.mutate()
			body = h.serialize()
		}
		if !failed {
			contentType := []string{"text/csv", "text/csv; charset=utf-8", "text/csv;charset=UTF-8"}[r.Intn(3)]
			s.set("/items", stubFile{body: body, contentType: contentType})
		}

		at(t, stamp.In(cet))
		e := entry(t, readManifest(t, run(t, idx, dir).Manifest), url)
		if failed {
			if e.Error == "" || e.Rows != "" || e.Delta != "" {
				t.Fatalf("night %d: failed download recorded as %+v", night, e)
			}
			continue
		}
		next := refCanonical(body)
		known[sum(next)] = next
		full := newChain || !bytes.Equal(next, base) && len(refDelta(base, next)) > len(next)
		if e.Error != "" || e.SHA256 != sum(body) || e.Size != int64(len(body)) || hasObject(dir, e.SHA256) {
			t.Fatalf("night %d: entry %+v for %d bytes, want them described but not stored", night, e, len(body))
		}
		switch {
		case full:
			checkDelta(t, dir, e, nil, next)
		case bytes.Equal(next, base):
			checkNoDelta(t, e, sum(base))
		default:
			checkDelta(t, dir, e, base, next)
		}
		if t.Failed() {
			t.Fatalf("night %d at %s, served %q", night, stamp.UTC(), body)
		}
		if full || !bytes.Equal(next, base) {
			base, baseYear = next, stamp.UTC().Year()
		}
		lastBody, stateless = body, false
		// Checking every version every night makes the test quadratic
		if got := tableBytes(t, dir, e.Rows); !bytes.Equal(got, next) {
			t.Fatalf("night %d: Table %q, want %q", night, got, next)
		}
		if night%10 == 0 {
			checkTables(t, dir, known)
		}
	}
	checkTables(t, dir, known)
	if baseYear != 2027 {
		t.Fatalf("the history ended in %d without crossing a year", baseYear)
	}
}

// A table that returns to an earlier version names that version's rows again,
// and every version stays reconstructible
func TestRunLayerTableRevisitsVersion(t *testing.T) {
	s := newStubServer(t)
	idx := layerIndex(s.URL + "/items")
	dir := t.TempDir()
	stable := "id\n" + strings.Repeat("0;ein langer Datensatz\n", 20)
	known := map[string][]byte{}
	for i, body := range []string{stable, stable + "1\n", stable + "1\n2\n", stable + "1\n"} {
		s.serveTable("/items", body)
		at(t, day.Add(time.Duration(i)*24*time.Hour))
		run(t, idx, dir)
		known[sum(refCanonical([]byte(body)))] = refCanonical([]byte(body))
	}
	checkTables(t, dir, known)
}

// The state is only the base of the next delta, so a damaged one costs a full
// version and not the run
func TestRunLayerTableDamagedState(t *testing.T) {
	stable := "id\n" + strings.Repeat("0;ein langer Datensatz\n", 20)
	for name, damage := range map[string]func(path string) error{
		"unreadable": func(path string) error { return os.WriteFile(path, []byte("kein zstd"), 0o644) },
		// A readable state of another table under the same name
		"wrong content": func(path string) error {
			encoder, err := zstd.NewWriter(nil)
			if err != nil {
				return err
			}
			return os.WriteFile(path, encoder.EncodeAll([]byte("id\n9\n"), nil), 0o644)
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := newStubServer(t)
			idx := layerIndex(s.URL + "/items")
			dir := t.TempDir()
			s.serveTable("/items", stable)
			at(t, day)
			run(t, idx, dir)

			states, err := filepath.Glob(filepath.Join(dir, "state", "*", "*"))
			if err != nil || len(states) != 1 {
				t.Fatalf("states %v, %v", states, err)
			}
			if err := damage(states[0]); err != nil {
				t.Fatal(err)
			}
			s.serveTable("/items", stable+"1\n")
			at(t, day.Add(24*time.Hour))
			e := entry(t, readManifest(t, run(t, idx, dir).Manifest), s.URL+"/items")
			checkDelta(t, dir, e, nil, refCanonical([]byte(stable+"1\n")))
		})
	}
}

// A table whose records all change is stored in full rather than as a delta
// twice its size
func TestRunLayerTableRewritten(t *testing.T) {
	s := newStubServer(t)
	idx := layerIndex(s.URL + "/items")
	dir := t.TempDir()
	s.serveTable("/items", "id;stand\n1;Montag\n2;Montag\n")
	at(t, day)
	run(t, idx, dir)
	s.serveTable("/items", "id;stand\n1;Dienstag\n2;Dienstag\n")
	at(t, day.Add(24*time.Hour))
	e := entry(t, readManifest(t, run(t, idx, dir).Manifest), s.URL+"/items")
	checkDelta(t, dir, e, nil, []byte("id;stand\n1;Dienstag\n2;Dienstag\n"))
}
