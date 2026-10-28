// Package archive keeps every version of every resource the portal publishes.
// Each run downloads all archivable resources and records them in a manifest;
// the downloads themselves are stored once per distinct content, compressed
// with zstd and named by the SHA-256 of their uncompressed bytes.
//
// Layout below the archive directory:
//
//	index.json                 catalog of the latest run, kept by `od3 archive`
//	objects/ab/cdef….zst       content, addressed by sha256 "abcdef…"
//	runs/<stamp>.jsonl         manifest of a completed run, one Entry per line
//	runs/<stamp>.jsonl.partial manifest of a run that did not finish
package archive

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kiliankoe/opendata-dresden/internal/index"
	"github.com/kiliankoe/opendata-dresden/internal/portal"
	"github.com/klauspost/compress/zstd"
	"golang.org/x/sync/errgroup"
)

// FormatCatalog labels the manifest entry of the run's catalog, the index
// file as written by index.Index.Write.
const FormatCatalog = "Katalog"

// FormatCollection labels a geodata layer's OGC API collection description,
// which lists the layer's columns with their types. It is archived in place of
// the layer's GeoJSON, whose features the CSV already holds and which carries
// a timestamp that makes every download differ.
const FormatCollection = "Collection"

// workers matches the mirror's limit, beyond which the portal starts refusing
// connections
const workers = 2

// stampLayout names the manifests. UTC and fixed width make them sort in the
// order of their runs.
const stampLayout = "20060102T150405Z"

// viewerFormats are the formats that present data published elsewhere in the
// dataset, or apps whose pages hold none
var viewerFormats = []string{
	"Themenstadtplan", "Tabelle",
	"Säulen-/Balkendiagramm", "Liniendiagramm", "Kreisdiagramm", "Pyramidendiagramm",
	"Bundestagswahlatlas", "Kommunalwahlatlas", "Landtagswahlatlas",
	"OParl-Schnittstelle", "3D-SceneLayer (I3S)",
}

// Entry is one line of a run's manifest
type Entry struct {
	Dataset string `json:"dataset,omitempty"` // empty for the catalog
	Format  string `json:"format"`
	URL     string `json:"url,omitempty"`
	Fetched string `json:"fetched"` // RFC 3339, UTC
	// Status is the HTTP status, 0 when no response arrived. 304 means the
	// content is unchanged since the previous run and SHA256 is carried over.
	Status       int    `json:"status,omitempty"`
	SHA256       string `json:"sha256,omitempty"`
	Size         int64  `json:"size,omitempty"` // uncompressed bytes
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"lastModified,omitempty"`
	Error        string `json:"error,omitempty"` // set when no content was stored
}

// Summary describes a finished run
type Summary struct {
	Manifest   string // path of the run's manifest
	Resources  int    // entries written, the catalog included
	NewObjects int    // contents not stored by any earlier run
	NewBytes   int64  // compressed size of those new objects
	Failed     int    // entries with an Error
}

// now stamps runs and entries. Tests replace it.
var now = time.Now

// responseTimeout bounds the wait for a response. The portal builds a layer's
// CSV before it answers, which takes minutes for the largest. Tests shorten it.
var responseTimeout = 10 * time.Minute

// idleTimeout abandons a download whose server stopped sending. Tests
// shorten it.
var idleTimeout = 2 * time.Minute

// Resources lists the resources of a dataset that a run downloads. Viewers
// that hold no data of their own (Themenstadtplan, the statistics tables and
// charts, election atlases, the OParl API, 3D scene layers) are left out. A
// geodata layer's GeoJSON is replaced by its collection description.
func Resources(ds *portal.Dataset) []portal.Resource {
	var resources []portal.Resource
	for _, r := range ds.Resources {
		switch {
		case presentsOnly(r.Format):
		case strings.EqualFold(r.Format, "GEOJSON"):
			resources = append(resources, portal.Resource{Format: FormatCollection, URL: strings.TrimSuffix(r.URL, "/items")})
		default:
			resources = append(resources, r)
		}
	}
	return resources
}

func presentsOnly(format string) bool {
	for _, v := range viewerFormats {
		if strings.EqualFold(v, format) {
			return true
		}
	}
	return false
}

// Run archives the catalog and every resource listed by Resources for each
// dataset of idx into dir. Resources whose previous entry carries an ETag or
// Last-Modified are requested conditionally. A failed download becomes an
// entry with Error and does not stop the run; only errors that leave the
// archive unusable, such as a full disk, are returned. An interrupted run
// leaves a .partial manifest and can simply be run again.
func Run(ctx context.Context, client *http.Client, idx *index.Index, dir string) (*Summary, error) {
	store, err := openStore(dir)
	if err != nil {
		return nil, err
	}
	previous, err := latestRun(filepath.Join(dir, "runs"))
	if err != nil {
		return nil, err
	}

	started := now().UTC()
	manifest := filepath.Join(dir, "runs", started.Format(stampLayout)+".jsonl")
	file, err := os.Create(manifest + ".partial")
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	summary := &Summary{Manifest: manifest}
	var mu sync.Mutex
	encoder := json.NewEncoder(file)
	record := func(e Entry) error {
		mu.Lock()
		defer mu.Unlock()
		summary.Resources++
		if e.Error != "" {
			summary.Failed++
		}
		return encoder.Encode(e)
	}

	catalog, err := idx.Marshal()
	if err != nil {
		return nil, err
	}
	sha, size, err := store.put(bytes.NewReader(catalog))
	if err != nil {
		return nil, err
	}
	if err := record(Entry{Format: FormatCatalog, Fetched: started.Format(time.RFC3339), SHA256: sha, Size: size}); err != nil {
		return nil, err
	}

	group, ctx := errgroup.WithContext(ctx)
	group.SetLimit(workers)
	for i := range idx.Datasets {
		ds := &idx.Datasets[i]
		for _, r := range Resources(ds) {
			group.Go(func() error {
				e, err := download(ctx, client, store, r.URL, previous[r.URL])
				if err != nil {
					return fmt.Errorf("archiving %s: %w", r.URL, err)
				}
				e.Dataset, e.Format, e.URL = ds.ID, r.Format, r.URL
				return record(e)
			})
		}
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	// A completed manifest must not name objects a power loss could take back
	if err := store.sync(); err != nil {
		return nil, err
	}
	if err := syncClose(file); err != nil {
		return nil, err
	}
	if err := os.Rename(manifest+".partial", manifest); err != nil {
		return nil, err
	}
	summary.NewObjects, summary.NewBytes = store.added, store.addedBytes
	return summary, nil
}

// download fetches one resource into the store. Failures of the server or the
// network become the entry's Error; only failures to store are returned.
func download(ctx context.Context, client *http.Client, store *store, url string, previous Entry) (Entry, error) {
	fetched := now().UTC().Format(time.RFC3339)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Entry{Fetched: fetched, Error: err.Error()}, nil
	}
	// A 304 only vouches for content the archive still holds
	if previous.SHA256 != "" && store.has(previous.SHA256) {
		if previous.ETag != "" {
			req.Header.Set("If-None-Match", previous.ETag)
		}
		if previous.LastModified != "" {
			req.Header.Set("If-Modified-Since", previous.LastModified)
		}
	}
	timeout := time.AfterFunc(responseTimeout, cancel)
	defer timeout.Stop()
	resp, err := client.Do(req)
	if err != nil {
		return Entry{Fetched: fetched, Error: err.Error()}, nil
	}
	defer func() { _ = resp.Body.Close() }()
	timeout.Reset(idleTimeout)

	e := Entry{Fetched: fetched, Status: resp.StatusCode, ETag: resp.Header.Get("ETag"), LastModified: resp.Header.Get("Last-Modified")}
	switch {
	case resp.StatusCode == http.StatusNotModified && req.Header.Get("If-None-Match")+req.Header.Get("If-Modified-Since") != "":
		e.SHA256, e.Size = previous.SHA256, previous.Size
		// A 304 need not repeat the validators, and the next run needs them
		if e.ETag == "" {
			e.ETag = previous.ETag
		}
		if e.LastModified == "" {
			e.LastModified = previous.LastModified
		}
	case resp.StatusCode != http.StatusOK:
		e.Error = "status " + resp.Status
	default:
		e.SHA256, e.Size, err = store.put(&idleReader{resp.Body, timeout})
		var read *readError
		if errors.As(err, &read) {
			e.Error = read.Error()
		} else if err != nil {
			return Entry{}, err
		}
	}
	if e.Error != "" {
		e.SHA256, e.Size, e.ETag, e.LastModified = "", 0, "", ""
	}
	return e, nil
}

// latestRun maps each URL of the latest completed run to its entry. Interrupted
// runs are skipped, since their objects may never have been written.
func latestRun(dir string) (map[string]Entry, error) {
	manifests, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil || len(manifests) == 0 {
		return nil, err
	}
	// Glob returns the names sorted, and the stamps sort chronologically
	file, err := os.Open(manifests[len(manifests)-1])
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	entries := map[string]Entry{}
	decoder := json.NewDecoder(file)
	for {
		var e Entry
		if err := decoder.Decode(&e); errors.Is(err, io.EOF) {
			return entries, nil
		} else if err != nil {
			return nil, fmt.Errorf("reading %s: %w", file.Name(), err)
		}
		if e.URL != "" {
			entries[e.URL] = e
		}
	}
}

// store holds the archive's objects
type store struct {
	dir, tmp   string
	mu         sync.Mutex
	claimed    map[string]bool // objects written or being written by this run
	added      int
	addedBytes int64
}

func (s *store) path(sha string) string {
	return filepath.Join(s.dir, sha[:2], sha[2:]+".zst")
}

func (s *store) has(sha string) bool {
	_, err := os.Stat(s.path(sha))
	return err == nil
}

// sync flushes the directories new objects were renamed into
func (s *store) sync() error {
	dirs, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	for _, d := range dirs {
		dir, err := os.Open(filepath.Join(s.dir, d.Name()))
		if err != nil {
			return err
		}
		if err := syncClose(dir); err != nil {
			return err
		}
	}
	return nil
}

func openStore(dir string) (*store, error) {
	s := &store{dir: filepath.Join(dir, "objects"), tmp: filepath.Join(dir, "tmp"), claimed: map[string]bool{}}
	// Downloads and objects an interrupted run left behind are of no use
	if err := os.RemoveAll(s.tmp); err != nil {
		return nil, err
	}
	for _, d := range []string{s.dir, s.tmp, filepath.Join(dir, "runs")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// readError marks a download that broke off, as opposed to a failing disk
type readError struct{ err error }

func (e *readError) Error() string { return "reading response: " + e.err.Error() }

// put stores content unless an object with the same hash exists. The content
// is hashed into a temporary file first, so that the far more frequent
// unchanged downloads cost no compression.
func (s *store) put(content io.Reader) (string, int64, error) {
	tmp, err := os.CreateTemp(s.tmp, "download-")
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = tmp.Close(); _ = os.Remove(tmp.Name()) }()

	hash := sha256.New()
	src := &trackingReader{Reader: content}
	size, err := io.Copy(io.MultiWriter(tmp, hash), src)
	if src.err != nil {
		return "", 0, &readError{src.err}
	}
	if err != nil {
		return "", 0, err
	}
	sha := hex.EncodeToString(hash.Sum(nil))

	s.mu.Lock()
	exists := s.claimed[sha] || s.has(sha)
	s.claimed[sha] = true
	s.mu.Unlock()
	if exists {
		return sha, size, nil
	}

	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return "", 0, err
	}
	written, err := s.compress(tmp, s.path(sha))
	if err != nil {
		return "", 0, err
	}
	s.mu.Lock()
	s.added++
	s.addedBytes += written
	s.mu.Unlock()
	return sha, size, nil
}

// compress writes src to path with zstd and returns the compressed size. It
// writes to the temporary directory and renames, so an object is either
// complete or absent.
func (s *store) compress(src io.Reader, path string) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, err
	}
	dst, err := os.CreateTemp(s.tmp, "object-")
	if err != nil {
		return 0, err
	}
	defer func() { _ = dst.Close(); _ = os.Remove(dst.Name()) }()
	// Only new content gets here, a few hundred MB a night at most, so the
	// slowest level pays off: it shrinks the largest layer forty-fold
	encoder, err := zstd.NewWriter(dst, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
	if err != nil {
		return 0, err
	}
	if _, err := io.Copy(encoder, src); err != nil {
		_ = encoder.Close()
		return 0, err
	}
	if err := encoder.Close(); err != nil {
		return 0, err
	}
	info, err := dst.Stat()
	if err != nil {
		return 0, err
	}
	// CreateTemp's 0600 would lock out readers other than the archive's user
	if err := dst.Chmod(0o644); err != nil {
		return 0, err
	}
	if err := syncClose(dst); err != nil {
		return 0, err
	}
	return info.Size(), os.Rename(dst.Name(), path)
}

// trackingReader remembers its own failure, which io.Copy would otherwise
// report alike to one of the disk
type trackingReader struct {
	io.Reader
	err error
}

func (r *trackingReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err != nil && err != io.EOF {
		r.err = err
	}
	return n, err
}

// idleReader extends its timer with every read, so that only a server that
// stopped sending runs into it, not a large download
type idleReader struct {
	io.Reader
	timer *time.Timer
}

func (r *idleReader) Read(p []byte) (int, error) {
	r.timer.Reset(idleTimeout)
	return r.Reader.Read(p)
}

func syncClose(f *os.File) error {
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
