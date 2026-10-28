package archive

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/quick"
	"time"
	"unicode"

	"github.com/kiliankoe/opendata-dresden/internal/index"
	"github.com/kiliankoe/opendata-dresden/internal/portal"
	"github.com/klauspost/compress/zstd"
)

// viewers are the formats that hold no data of their own
var viewers = []string{
	"Themenstadtplan", "Tabelle",
	"Säulen-/Balkendiagramm", "Liniendiagramm", "Kreisdiagramm", "Pyramidendiagramm",
	"Bundestagswahlatlas", "Kommunalwahlatlas", "Landtagswahlatlas",
	"OParl-Schnittstelle", "3D-SceneLayer (I3S)",
}

// kept are the formats seen on the portal that are archived as they are
var kept = []string{
	"Information", "CSV", "WMS", "WFS", "JSON", ".xlsx", "Exceltabelle", "Link",
	"DXF (*.zip-Archiv)", "CityGML (*.zip-Archiv)", "Shape (*.zip-Archiv)", "GeoTIFF (*.zip-Archiv)",
	"HTML-Bericht", "Dashboard", "pdf-Datei", "Legende (PDF)",
}

func isViewer(format string) bool {
	return slices.ContainsFunc(viewers, func(v string) bool { return strings.EqualFold(v, format) })
}

func TestResourcesExample(t *testing.T) {
	ds := &portal.Dataset{ID: "D1", LayerID: "L334", Resources: []portal.Resource{
		{Format: "Information", URL: "https://host/ogc.ashx?NODEID=1"},
		{Format: "CSV", URL: "https://host/net4/public/ogcapi/collections/L334/items?format=csv"},
		{Format: "GEOJSON", URL: "https://host/net4/public/ogcapi/collections/L334/items"},
		{Format: "Themenstadtplan", URL: "https://host/themenstadtplan?layer=334"},
		{Format: "WFS", URL: "https://host/ogcsl.ashx?service=wfs"},
		{Format: "Tabelle", URL: "https://host/aswdb/asw.dll/?aw=x"},
		{Format: "Kreisdiagramm", URL: "https://host/aswdb/asw.dll/?aw=y"},
		{Format: "3D-SceneLayer (I3S)", URL: "https://host/i3s"},
		{Format: "pdf-Datei", URL: "https://host/a.pdf"},
	}}
	want := []portal.Resource{
		{Format: "Information", URL: "https://host/ogc.ashx?NODEID=1"},
		{Format: "CSV", URL: "https://host/net4/public/ogcapi/collections/L334/items?format=csv"},
		// The collection description replaces the features, in the GeoJSON's place
		{Format: FormatCollection, URL: "https://host/net4/public/ogcapi/collections/L334"},
		{Format: "WFS", URL: "https://host/ogcsl.ashx?service=wfs"},
		{Format: "pdf-Datei", URL: "https://host/a.pdf"},
	}
	if got := Resources(ds); !reflect.DeepEqual(got, want) {
		t.Errorf("got  %v\nwant %v", got, want)
	}
}

func TestResourcesCaseInsensitive(t *testing.T) {
	ds := &portal.Dataset{ID: "D1", LayerID: "L1", Resources: []portal.Resource{
		{Format: "geojson", URL: "https://host/ogcapi/collections/L1/items"},
		{Format: "THEMENSTADTPLAN", URL: "https://host/tsp"},
		{Format: "liniendiagramm", URL: "https://host/chart"},
		{Format: "oparl-schnittstelle", URL: "https://host/oparl"},
	}}
	want := []portal.Resource{{Format: FormatCollection, URL: "https://host/ogcapi/collections/L1"}}
	if got := Resources(ds); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// randomDataset draws resources from the formats seen on the portal, in
// random letter case, so that the properties cover every format
type randomDataset struct{ portal.Dataset }

func (randomDataset) Generate(r *rand.Rand, size int) reflect.Value {
	formats := slices.Concat(viewers, kept, []string{"GEOJSON"})
	ds := portal.Dataset{ID: "D", LayerID: "L7"}
	for i := range r.Intn(size + 1) {
		format := []rune(formats[r.Intn(len(formats))])
		for j := range format {
			if r.Intn(2) == 0 {
				format[j] = unicode.ToUpper(format[j])
			} else {
				format[j] = unicode.ToLower(format[j])
			}
		}
		url := "https://host/r/" + string(rune('a'+i%26)) + "?n=" + string(rune('0'+r.Intn(10)))
		if strings.EqualFold(string(format), "GEOJSON") {
			url = "https://host/ogcapi/collections/L7/items"
		}
		ds.Resources = append(ds.Resources, portal.Resource{Format: string(format), URL: url})
	}
	return reflect.ValueOf(randomDataset{ds})
}

func TestResourcesProperties(t *testing.T) {
	property := func(in randomDataset) bool {
		original := slices.Clone(in.Resources)
		got := Resources(&in.Dataset)

		// The dataset itself stays untouched; the index is written after the run
		if !reflect.DeepEqual(in.Resources, original) {
			t.Logf("input modified: %v", in.Resources)
			return false
		}

		// Every non-viewer survives in order, the GeoJSON as its collection
		var want []portal.Resource
		for _, r := range original {
			switch {
			case isViewer(r.Format):
			case strings.EqualFold(r.Format, "GEOJSON"):
				want = append(want, portal.Resource{Format: FormatCollection, URL: strings.TrimSuffix(r.URL, "/items")})
			default:
				want = append(want, r)
			}
		}
		if len(got) != len(want) || (len(want) > 0 && !reflect.DeepEqual(got, want)) {
			t.Logf("in   %v\ngot  %v\nwant %v", original, got, want)
			return false
		}

		// Applying it again changes nothing, so a collection is never mistaken
		// for a viewer or rewritten twice
		again := Resources(&portal.Dataset{ID: in.ID, LayerID: in.LayerID, Resources: got})
		if len(again) != len(got) || (len(got) > 0 && !reflect.DeepEqual(again, got)) {
			t.Logf("not idempotent: %v -> %v", got, again)
			return false
		}
		return true
	}
	if err := quick.Check(property, &quick.Config{MaxCount: 500}); err != nil {
		t.Error(err)
	}
}

// fakeResource is one download the fake server offers
type fakeResource struct {
	body         []byte
	status       int // 200 when zero
	etag         string
	lastModified string
}

// request records what the archive sent to the fake server
type request struct {
	path, ifNoneMatch, ifModifiedSince string
}

type fakeServer struct {
	*httptest.Server
	mu        sync.Mutex
	resources map[string]*fakeResource
	requests  []request
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	fake := &fakeServer{resources: map[string]*fakeResource{}}
	fake.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		req := request{r.URL.Path, r.Header.Get("If-None-Match"), r.Header.Get("If-Modified-Since")}
		fake.requests = append(fake.requests, req)
		res, ok := fake.resources[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if res.etag != "" {
			w.Header().Set("ETag", res.etag)
		}
		if res.lastModified != "" {
			w.Header().Set("Last-Modified", res.lastModified)
		}
		if res.etag != "" && req.ifNoneMatch == res.etag ||
			res.lastModified != "" && req.ifModifiedSince == res.lastModified {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		if res.status != 0 {
			w.WriteHeader(res.status)
		}
		_, _ = w.Write(res.body)
	}))
	t.Cleanup(fake.Close)
	return fake
}

func (f *fakeServer) set(path string, res *fakeResource) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resources[path] = res
}

// requestsTo returns the requests made for path since the last reset
func (f *fakeServer) requestsTo(path string) []request {
	f.mu.Lock()
	defer f.mu.Unlock()
	var matching []request
	for _, r := range f.requests {
		if r.path == path {
			matching = append(matching, r)
		}
	}
	return matching
}

func (f *fakeServer) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = nil
}

// at fixes the clock for the next run. Each run gets its own time, so that
// runs are told apart and ordered.
func at(t *testing.T, stamp time.Time) {
	t.Helper()
	original := now
	t.Cleanup(func() { now = original })
	now = func() time.Time { return stamp }
}

var day = time.Date(2026, 3, 5, 2, 0, 0, 0, time.UTC)

func run(t *testing.T, idx *index.Index, dir string) *Summary {
	t.Helper()
	summary, err := Run(context.Background(), http.DefaultClient, idx, dir)
	if err != nil {
		t.Fatal(err)
	}
	return summary
}

func readManifest(t *testing.T, path string) []Entry {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	var entries []Entry
	scanner := bufio.NewScanner(file)
	scanner.Buffer(nil, 1<<20)
	for scanner.Scan() {
		var e Entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatalf("manifest line %q: %v", scanner.Text(), err)
		}
		entries = append(entries, e)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return entries
}

// entry finds the manifest entry of a URL
func entry(t *testing.T, entries []Entry, url string) Entry {
	t.Helper()
	for _, e := range entries {
		if e.URL == url {
			return e
		}
	}
	t.Fatalf("no entry for %s in %+v", url, entries)
	return Entry{}
}

func catalogEntry(t *testing.T, entries []Entry) Entry {
	t.Helper()
	var found []Entry
	for _, e := range entries {
		if e.Format == FormatCatalog {
			found = append(found, e)
		}
	}
	if len(found) != 1 {
		t.Fatalf("got %d catalog entries, want 1: %+v", len(found), entries)
	}
	return found[0]
}

func sum(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// object reads back the stored content of a SHA-256
func object(t *testing.T, dir, sha string) []byte {
	t.Helper()
	compressed, err := os.ReadFile(filepath.Join(dir, "objects", sha[:2], sha[2:]+".zst"))
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.Close()
	data, err := decoder.DecodeAll(compressed, nil)
	if err != nil {
		t.Fatalf("object %s: %v", sha, err)
	}
	return data
}

// objects lists the stored object files with their sizes
func objects(t *testing.T, dir string) map[string]int64 {
	t.Helper()
	found := map[string]int64{}
	root := filepath.Join(dir, "objects")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return filepath.SkipDir
		}
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		found[path] = info.Size()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// catalogBytes is the index serialized the way index.Index.Write stores it
func catalogBytes(t *testing.T, idx *index.Index) []byte {
	t.Helper()
	copied := &index.Index{Datasets: slices.Clone(idx.Datasets)}
	path := filepath.Join(t.TempDir(), "index.json")
	if err := copied.Write(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestRunStoresResourcesAndCatalog(t *testing.T) {
	fake := newFakeServer(t)
	// Raw bytes as the portal sends them: a BOM, CRLF and Latin-1 must survive
	csv := []byte("\xef\xbb\xbfJahr;Stra\xdfe\r\n2020;1\r\n")
	collection := []byte(`{"id":"L1","properties":{}}`)
	fake.set("/stats.csv", &fakeResource{body: csv})
	fake.set("/ogcapi/collections/L1", &fakeResource{body: collection})
	idx := &index.Index{Datasets: []portal.Dataset{
		{ID: "D1", LayerID: "L1", Resources: []portal.Resource{
			{Format: "GEOJSON", URL: fake.URL + "/ogcapi/collections/L1/items"},
			{Format: "Themenstadtplan", URL: fake.URL + "/viewer"},
		}},
		{ID: "D2", Resources: []portal.Resource{
			{Format: "CSV", URL: fake.URL + "/stats.csv"},
			{Format: "Tabelle", URL: fake.URL + "/table"},
		}},
	}}
	dir := t.TempDir()
	at(t, time.Date(2026, 3, 5, 3, 4, 5, 0, time.FixedZone("CET", 3600)))

	summary := run(t, idx, dir)

	// Viewers and the GeoJSON features are never requested
	for _, path := range []string{"/viewer", "/table", "/ogcapi/collections/L1/items"} {
		if got := fake.requestsTo(path); len(got) != 0 {
			t.Errorf("%s was requested %d times", path, len(got))
		}
	}

	if filepath.Dir(summary.Manifest) != filepath.Join(dir, "runs") || !strings.HasSuffix(summary.Manifest, ".jsonl") {
		t.Errorf("manifest path %q", summary.Manifest)
	}
	if _, err := os.Stat(summary.Manifest + ".partial"); !os.IsNotExist(err) {
		t.Errorf("partial manifest left behind: %v", err)
	}
	entries := readManifest(t, summary.Manifest)
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want the catalog and two resources: %+v", len(entries), entries)
	}

	stats := entry(t, entries, fake.URL+"/stats.csv")
	want := Entry{Dataset: "D2", Format: "CSV", URL: fake.URL + "/stats.csv", Fetched: "2026-03-05T02:04:05Z",
		Status: 200, SHA256: sum(csv), Size: int64(len(csv))}
	if stats != want {
		t.Errorf("got  %+v\nwant %+v", stats, want)
	}
	if got := object(t, dir, sum(csv)); string(got) != string(csv) {
		t.Errorf("stored %q, want the bytes as served %q", got, csv)
	}

	layer := entry(t, entries, fake.URL+"/ogcapi/collections/L1")
	if layer.Dataset != "D1" || layer.Format != FormatCollection || layer.SHA256 != sum(collection) {
		t.Errorf("collection entry %+v", layer)
	}
	if got := object(t, dir, sum(collection)); string(got) != string(collection) {
		t.Errorf("stored collection %q", got)
	}

	catalog := catalogEntry(t, entries)
	data := catalogBytes(t, idx)
	if catalog.Dataset != "" || catalog.SHA256 != sum(data) || catalog.Size != int64(len(data)) || catalog.Error != "" {
		t.Errorf("catalog entry %+v, want sha %s of %d bytes", catalog, sum(data), len(data))
	}
	if got := object(t, dir, sum(data)); string(got) != string(data) {
		t.Errorf("stored catalog differs from index.Write's output:\n%s", got)
	}

	// NewBytes counts what landed on disk, compressed
	stored := objects(t, dir)
	var size int64
	for _, s := range stored {
		size += s
	}
	if summary.Resources != 3 || summary.NewObjects != 3 || len(stored) != 3 || summary.NewBytes != size || summary.Failed != 0 {
		t.Errorf("summary %+v, %d objects of %d bytes on disk", summary, len(stored), size)
	}
}

func TestRunDeduplicatesContent(t *testing.T) {
	fake := newFakeServer(t)
	body := []byte("a;b\n1;2\n")
	// No validators, so every run downloads again
	fake.set("/a.csv", &fakeResource{body: body})
	fake.set("/b.csv", &fakeResource{body: body})
	idx := &index.Index{Datasets: []portal.Dataset{
		{ID: "D1", Resources: []portal.Resource{{Format: "CSV", URL: fake.URL + "/a.csv"}}},
		{ID: "D2", Resources: []portal.Resource{{Format: "CSV", URL: fake.URL + "/b.csv"}}},
	}}
	dir := t.TempDir()

	// Two resources with the same bytes share one object
	at(t, day)
	first := run(t, idx, dir)
	if first.NewObjects != 2 || len(objects(t, dir)) != 2 {
		t.Errorf("first run: %+v, objects %v, want the catalog and one shared object", first, objects(t, dir))
	}

	// Downloading identical bytes again stores nothing new
	before := objects(t, dir)
	at(t, day.Add(24*time.Hour))
	second := run(t, idx, dir)
	if len(fake.requestsTo("/a.csv")) != 2 {
		t.Errorf("a resource without validators was requested %d times in two runs", len(fake.requestsTo("/a.csv")))
	}
	if second.NewObjects != 0 || second.NewBytes != 0 || !reflect.DeepEqual(objects(t, dir), before) {
		t.Errorf("second run: %+v, objects %v, want none new", second, objects(t, dir))
	}
	for _, e := range readManifest(t, second.Manifest) {
		if e.Format != FormatCatalog && (e.Status != 200 || e.SHA256 != sum(body)) {
			t.Errorf("entry %+v", e)
		}
	}

	// Changed content becomes a new object next to the old one
	fake.set("/a.csv", &fakeResource{body: []byte("a;b\n1;3\n")})
	at(t, day.Add(48*time.Hour))
	third := run(t, idx, dir)
	if third.NewObjects != 1 || len(objects(t, dir)) != 3 {
		t.Errorf("third run: %+v, objects %v", third, objects(t, dir))
	}
	if got := entry(t, readManifest(t, third.Manifest), fake.URL+"/a.csv").SHA256; got != sum([]byte("a;b\n1;3\n")) {
		t.Errorf("changed resource recorded as %s", got)
	}
}

// Content addressing holds for any bytes, whether or not they compress
func TestRunContentAddressing(t *testing.T) {
	fake := newFakeServer(t)
	dir := t.TempDir()
	start := day
	property := func(bodies [][]byte) bool {
		fake.reset()
		idx := &index.Index{}
		served := map[string][]byte{}
		for i, body := range bodies {
			path := "/r" + string(rune('a'+i%26)) + string(rune('a'+i/26%26))
			fake.set(path, &fakeResource{body: body})
			served[fake.URL+path] = body
			idx.Datasets = append(idx.Datasets, portal.Dataset{
				ID: "D" + path, Resources: []portal.Resource{{Format: "CSV", URL: fake.URL + path}}})
		}
		start = start.Add(time.Hour)
		at(t, start)
		summary := run(t, idx, dir)
		for i, e := range readManifest(t, summary.Manifest) {
			if e.Format == FormatCatalog {
				continue
			}
			body := served[e.URL]
			if e.Error != "" || e.SHA256 != sum(body) || e.Size != int64(len(body)) {
				t.Logf("entry %d: %+v for %d bytes", i, e, len(body))
				return false
			}
			if got := object(t, dir, e.SHA256); string(got) != string(body) {
				t.Logf("object %s holds %q, want %q", e.SHA256, got, body)
				return false
			}
		}
		return true
	}
	if err := quick.Check(property, &quick.Config{MaxCount: 30}); err != nil {
		t.Error(err)
	}
}

func TestRunConditionalRequests(t *testing.T) {
	fake := newFakeServer(t)
	tagged := []byte("tagged v1")
	dated := []byte("dated v1")
	fake.set("/tagged", &fakeResource{body: tagged, etag: `"v1"`})
	fake.set("/dated", &fakeResource{body: dated, lastModified: "Wed, 04 Mar 2026 10:00:00 GMT"})
	idx := &index.Index{Datasets: []portal.Dataset{{ID: "D1", Resources: []portal.Resource{
		{Format: "CSV", URL: fake.URL + "/tagged"},
		{Format: "JSON", URL: fake.URL + "/dated"},
	}}}}
	dir := t.TempDir()

	// The first run has nothing to compare against
	at(t, day)
	first := run(t, idx, dir)
	if r := fake.requestsTo("/tagged"); len(r) != 1 || r[0].ifNoneMatch != "" || r[0].ifModifiedSince != "" {
		t.Errorf("first request %+v, want an unconditional one", r)
	}
	entries := readManifest(t, first.Manifest)
	if e := entry(t, entries, fake.URL+"/tagged"); e.ETag != `"v1"` || e.Status != 200 {
		t.Errorf("tagged entry %+v, want the ETag recorded", e)
	}
	if e := entry(t, entries, fake.URL+"/dated"); e.LastModified != "Wed, 04 Mar 2026 10:00:00 GMT" || e.Status != 200 {
		t.Errorf("dated entry %+v, want Last-Modified recorded", e)
	}

	// Unchanged content answers 304 and is not downloaded or stored again
	fake.reset()
	before := objects(t, dir)
	at(t, day.Add(24*time.Hour))
	second := run(t, idx, dir)
	if r := fake.requestsTo("/tagged"); len(r) != 1 || r[0].ifNoneMatch != `"v1"` {
		t.Errorf("tagged request %+v, want If-None-Match from the previous run", r)
	}
	if r := fake.requestsTo("/dated"); len(r) != 1 || r[0].ifModifiedSince != "Wed, 04 Mar 2026 10:00:00 GMT" {
		t.Errorf("dated request %+v, want If-Modified-Since from the previous run", r)
	}
	entries = readManifest(t, second.Manifest)
	for url, body := range map[string][]byte{fake.URL + "/tagged": tagged, fake.URL + "/dated": dated} {
		e := entry(t, entries, url)
		if e.Status != 304 || e.SHA256 != sum(body) || e.Size != int64(len(body)) || e.Error != "" {
			t.Errorf("unchanged entry %+v, want 304 with the previous hash and size", e)
		}
		if e.Fetched != "2026-03-06T02:00:00Z" {
			t.Errorf("unchanged entry fetched %q, want this run's time", e.Fetched)
		}
	}
	if second.NewObjects != 0 || second.Failed != 0 || !reflect.DeepEqual(objects(t, dir), before) {
		t.Errorf("second run %+v stored new objects", second)
	}

	// A 304 entry keeps the validators, so the run after it is conditional too
	fake.reset()
	at(t, day.Add(48*time.Hour))
	third := run(t, idx, dir)
	if r := fake.requestsTo("/tagged"); len(r) != 1 || r[0].ifNoneMatch != `"v1"` {
		t.Errorf("request after a 304 run %+v, want it conditional still", r)
	}
	if e := entry(t, readManifest(t, third.Manifest), fake.URL+"/tagged"); e.Status != 304 || e.SHA256 != sum(tagged) || e.ETag != `"v1"` {
		t.Errorf("entry after two unchanged runs %+v", e)
	}

	// New content under a new ETag is downloaded and the new ETag recorded
	fake.set("/tagged", &fakeResource{body: []byte("tagged v2"), etag: `"v2"`})
	at(t, day.Add(72*time.Hour))
	fourth := run(t, idx, dir)
	e := entry(t, readManifest(t, fourth.Manifest), fake.URL+"/tagged")
	if e.Status != 200 || e.SHA256 != sum([]byte("tagged v2")) || e.ETag != `"v2"` || fourth.NewObjects != 1 {
		t.Errorf("changed entry %+v, summary %+v", e, fourth)
	}

	// The next run asks with the newest ETag
	fake.reset()
	at(t, day.Add(96*time.Hour))
	run(t, idx, dir)
	if r := fake.requestsTo("/tagged"); len(r) != 1 || r[0].ifNoneMatch != `"v2"` {
		t.Errorf("request %+v, want the latest run's ETag", r)
	}
}

// An interrupted run's manifest may be incomplete or point at objects that
// were never written, so it must not provide validators
func TestRunIgnoresPartialManifests(t *testing.T) {
	fake := newFakeServer(t)
	fake.set("/tagged", &fakeResource{body: []byte("v1"), etag: `"v1"`})
	idx := &index.Index{Datasets: []portal.Dataset{{ID: "D1", Resources: []portal.Resource{
		{Format: "CSV", URL: fake.URL + "/tagged"},
	}}}}
	dir := t.TempDir()

	at(t, day)
	run(t, idx, dir)

	// A later run died after recording v2, which it never finished storing
	fake.set("/tagged", &fakeResource{body: []byte("v2"), etag: `"v2"`})
	line, err := json.Marshal(Entry{Dataset: "D1", Format: "CSV", URL: fake.URL + "/tagged",
		Fetched: "2026-03-05T12:00:00Z", Status: 200, SHA256: sum([]byte("v2")), Size: 2, ETag: `"v2"`})
	if err != nil {
		t.Fatal(err)
	}
	// Named to sort after any stamp, so it would count as the latest run
	partial := filepath.Join(dir, "runs", "z.jsonl.partial")
	if err := os.WriteFile(partial, append(line, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	fake.reset()
	at(t, day.Add(24*time.Hour))
	second := run(t, idx, dir)
	if r := fake.requestsTo("/tagged"); len(r) != 1 || r[0].ifNoneMatch != `"v1"` {
		t.Errorf("request %+v, want the ETag of the completed run", r)
	}
	e := entry(t, readManifest(t, second.Manifest), fake.URL+"/tagged")
	if e.Status != 200 || e.SHA256 != sum([]byte("v2")) {
		t.Errorf("entry %+v, want a fresh download of v2", e)
	}
	if got := object(t, dir, sum([]byte("v2"))); string(got) != "v2" {
		t.Errorf("stored %q", got)
	}
}

func TestRunRecordsFailures(t *testing.T) {
	fake := newFakeServer(t)
	good := []byte("ok")
	fake.set("/good", &fakeResource{body: good})
	fake.set("/broken", &fakeResource{body: []byte("boom"), status: http.StatusInternalServerError})
	// A server that is gone yields no response at all
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	idx := &index.Index{Datasets: []portal.Dataset{
		{ID: "D1", Resources: []portal.Resource{{Format: "CSV", URL: fake.URL + "/broken"}}},
		{ID: "D2", Resources: []portal.Resource{{Format: "CSV", URL: gone.URL + "/away"}}},
		{ID: "D3", Resources: []portal.Resource{{Format: "CSV", URL: fake.URL + "/good"}}},
	}}
	dir := t.TempDir()

	at(t, day)
	summary, err := Run(context.Background(), http.DefaultClient, idx, dir)
	if err != nil {
		t.Fatalf("failed downloads aborted the run: %v", err)
	}
	entries := readManifest(t, summary.Manifest)
	if summary.Failed != 2 || summary.Resources != 4 || len(entries) != 4 {
		t.Errorf("summary %+v with %d entries, want 2 of 4 failed", summary, len(entries))
	}

	broken := entry(t, entries, fake.URL+"/broken")
	if broken.Error == "" || broken.Status != 500 || broken.SHA256 != "" || broken.Dataset != "D1" {
		t.Errorf("HTTP error entry %+v", broken)
	}
	// The error page is no version of the resource
	boom := sum([]byte("boom"))
	if _, err := os.Stat(filepath.Join(dir, "objects", boom[:2], boom[2:]+".zst")); !os.IsNotExist(err) {
		t.Errorf("the error body was stored: %v", err)
	}
	away := entry(t, entries, gone.URL+"/away")
	if away.Error == "" || away.Status != 0 || away.SHA256 != "" || away.Dataset != "D2" {
		t.Errorf("connection failure entry %+v", away)
	}
	if e := entry(t, entries, fake.URL+"/good"); e.Error != "" || e.SHA256 != sum(good) {
		t.Errorf("good entry %+v, want it archived despite the failures", e)
	}

	// Once the resource recovers it is archived normally
	fake.set("/broken", &fakeResource{body: []byte("fixed")})
	at(t, day.Add(24*time.Hour))
	second := run(t, idx, dir)
	if e := entry(t, readManifest(t, second.Manifest), fake.URL+"/broken"); e.Error != "" || e.Status != 200 || e.SHA256 != sum([]byte("fixed")) {
		t.Errorf("recovered entry %+v", e)
	}
	if second.Failed != 1 {
		t.Errorf("second run %+v, want only the vanished server failing", second)
	}
}

func TestRunManifestsSortChronologically(t *testing.T) {
	fake := newFakeServer(t)
	fake.set("/a", &fakeResource{body: []byte("a")})
	idx := &index.Index{Datasets: []portal.Dataset{{ID: "D1", Resources: []portal.Resource{
		{Format: "CSV", URL: fake.URL + "/a"},
	}}}}
	dir := t.TempDir()

	// Stamps taken in local time would put the second run after the third,
	// unpadded ones the fourth before the fifth
	times := []time.Time{
		time.Date(2026, 9, 30, 23, 0, 0, 0, time.UTC),
		time.Date(2026, 10, 1, 1, 30, 0, 0, time.FixedZone("CEST", 2*3600)),
		time.Date(2026, 9, 30, 23, 45, 0, 0, time.UTC),
		time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC),
	}
	var manifests []string
	for _, stamp := range times {
		at(t, stamp)
		manifests = append(manifests, run(t, idx, dir).Manifest)
	}
	if !slices.IsSorted(manifests) || len(slices.Compact(slices.Clone(manifests))) != len(times) {
		t.Errorf("manifests %v do not sort in the order of their runs", manifests)
	}
	completed, _ := filepath.Glob(filepath.Join(dir, "runs", "*.jsonl"))
	if !reflect.DeepEqual(completed, manifests) {
		t.Errorf("runs directory %v, want %v", completed, manifests)
	}
}

func TestRunCatalogUnchanged(t *testing.T) {
	idx := &index.Index{Datasets: []portal.Dataset{{ID: "D1", Title: "Leer"}}}
	dir := t.TempDir()

	at(t, day)
	first := run(t, idx, dir)
	if first.Resources != 1 || first.NewObjects != 1 {
		t.Errorf("first run %+v, want only the catalog", first)
	}
	// The same catalog the next night is recorded but not stored again
	at(t, day.Add(24*time.Hour))
	second := run(t, idx, dir)
	if second.Resources != 1 || second.NewObjects != 0 {
		t.Errorf("second run %+v", second)
	}
	if a, b := catalogEntry(t, readManifest(t, first.Manifest)), catalogEntry(t, readManifest(t, second.Manifest)); a.SHA256 != b.SHA256 {
		t.Errorf("identical catalogs hashed differently: %s, %s", a.SHA256, b.SHA256)
	}
}

// A 304 only vouches for content the archive still holds. An object lost to a
// crash or a partial restore is downloaded again instead of carried forward.
func TestRunRefetchesMissingObjects(t *testing.T) {
	fake := newFakeServer(t)
	body := []byte("tagged")
	fake.set("/tagged", &fakeResource{body: body, etag: `"v1"`})
	idx := &index.Index{Datasets: []portal.Dataset{{ID: "D1", Resources: []portal.Resource{
		{Format: "CSV", URL: fake.URL + "/tagged"},
	}}}}
	dir := t.TempDir()

	at(t, day)
	run(t, idx, dir)
	sha := sum(body)
	if err := os.Remove(filepath.Join(dir, "objects", sha[:2], sha[2:]+".zst")); err != nil {
		t.Fatal(err)
	}

	fake.reset()
	at(t, day.Add(24*time.Hour))
	second := run(t, idx, dir)
	if r := fake.requestsTo("/tagged"); len(r) != 1 || r[0].ifNoneMatch != "" {
		t.Errorf("request %+v, want an unconditional one", r)
	}
	if e := entry(t, readManifest(t, second.Manifest), fake.URL+"/tagged"); e.Status != 200 || e.SHA256 != sha {
		t.Errorf("entry %+v", e)
	}
	if got := object(t, dir, sha); string(got) != string(body) {
		t.Errorf("stored %q", got)
	}
}

// A body that breaks off is no version of the resource
func TestRunRecordsTruncatedBodies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("only a part"))
	}))
	t.Cleanup(server.Close)
	idx := &index.Index{Datasets: []portal.Dataset{{ID: "D1", Resources: []portal.Resource{
		{Format: "CSV", URL: server.URL + "/cut"},
	}}}}
	dir := t.TempDir()

	at(t, day)
	summary := run(t, idx, dir)
	e := entry(t, readManifest(t, summary.Manifest), server.URL+"/cut")
	if e.Error == "" || e.SHA256 != "" || summary.Failed != 1 || summary.NewObjects != 1 {
		t.Errorf("entry %+v, summary %+v, want an error and only the catalog stored", e, summary)
	}
}

// A server that stops sending mid-body would otherwise hold a worker until
// systemd kills the whole run
func TestRunAbandonsStalledDownloads(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("start"))
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); server.Close() })
	defer func(original time.Duration) { idleTimeout = original }(idleTimeout)
	idleTimeout = 100 * time.Millisecond
	idx := &index.Index{Datasets: []portal.Dataset{{ID: "D1", Resources: []portal.Resource{
		{Format: "CSV", URL: server.URL + "/stall"},
	}}}}

	at(t, day)
	done := make(chan *Summary)
	go func() { done <- run(t, idx, t.TempDir()) }()
	select {
	case summary := <-done:
		if e := entry(t, readManifest(t, summary.Manifest), server.URL+"/stall"); e.Error == "" {
			t.Errorf("entry %+v, want the stall recorded as an error", e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the run is still waiting for the stalled download")
	}
}

// Readers of the archive need not run as the user that wrote it
func TestRunObjectsAreReadable(t *testing.T) {
	dir := t.TempDir()
	at(t, day)
	run(t, &index.Index{}, dir)
	for path := range objects(t, dir) {
		if info, err := os.Stat(path); err != nil || info.Mode().Perm()&0o044 != 0o044 {
			t.Errorf("%s: mode %v, err %v", path, info.Mode(), err)
		}
	}
}

// The portal builds a large layer's CSV before it answers, which takes longer
// than a stall in the middle of a download is allowed to
func TestRunWaitsForSlowResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte("built"))
	}))
	t.Cleanup(server.Close)
	defer func(idle, response time.Duration) { idleTimeout, responseTimeout = idle, response }(idleTimeout, responseTimeout)
	idleTimeout, responseTimeout = 50*time.Millisecond, 5*time.Second
	idx := &index.Index{Datasets: []portal.Dataset{{ID: "D1", Resources: []portal.Resource{
		{Format: "CSV", URL: server.URL + "/slow"},
	}}}}

	at(t, day)
	summary := run(t, idx, t.TempDir())
	if e := entry(t, readManifest(t, summary.Manifest), server.URL+"/slow"); e.Error != "" || e.SHA256 != sum([]byte("built")) {
		t.Errorf("entry %+v, want the slow response archived", e)
	}
}

// A server that never answers is given up on, whatever client the run uses
func TestRunAbandonsSilentServers(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); server.Close() })
	defer func(original time.Duration) { responseTimeout = original }(responseTimeout)
	responseTimeout = 100 * time.Millisecond
	idx := &index.Index{Datasets: []portal.Dataset{{ID: "D1", Resources: []portal.Resource{
		{Format: "CSV", URL: server.URL + "/silent"},
	}}}}

	at(t, day)
	done := make(chan *Summary)
	go func() { done <- run(t, idx, t.TempDir()) }()
	select {
	case summary := <-done:
		if e := entry(t, readManifest(t, summary.Manifest), server.URL+"/silent"); e.Error == "" {
			t.Errorf("entry %+v, want the silence recorded as an error", e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the run is still waiting for a server that never answers")
	}
}
