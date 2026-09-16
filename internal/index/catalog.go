package index

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kiliankoe/opendata-dresden/internal/arcgis"
	"github.com/kiliankoe/opendata-dresden/internal/config"
	"github.com/kiliankoe/opendata-dresden/internal/portal"
)

// maxAge is how long a cached index is used before a fresh one is downloaded
const maxAge = 24 * time.Hour

// Catalog answers searches and lookups from the index, which is loaded on
// first use. Datasets the index does not know yet are looked up in the
// portal, and resources are always fetched from their source.
type Catalog struct {
	cfg    *config.Config
	client *portal.Client
	arcgis *arcgis.Client

	mu    sync.Mutex
	index *Index
	byID  map[string]*portal.Dataset
}

func NewCatalog(cfg *config.Config) *Catalog {
	return &Catalog{cfg: cfg, client: portal.NewClient(cfg), arcgis: arcgis.NewClient(cfg)}
}

func (c *Catalog) Search(ctx context.Context, query string, limit, offset int) (*portal.SearchResult, error) {
	if err := c.load(ctx); err != nil {
		return nil, err
	}
	return c.index.Search(query, limit, offset), nil
}

func (c *Catalog) Get(ctx context.Context, id string) (*portal.Dataset, error) {
	if err := c.load(ctx); err != nil {
		return nil, err
	}
	if ds, ok := c.byID[id]; ok {
		return ds, nil
	}
	return c.client.GetDataset(ctx, id)
}

func (c *Catalog) Fetch(ctx context.Context, id, format string, opts portal.FetchOptions) ([]byte, error) {
	ds, err := c.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if ds.Portal == arcgis.Portal {
		return c.arcgis.FetchResource(ctx, ds, format, opts)
	}
	return c.client.FetchResource(ctx, ds, format, opts)
}

func (c *Catalog) load(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.index != nil {
		return nil
	}
	idx, err := c.open(ctx)
	if err != nil {
		return fmt.Errorf("loading dataset index: %w", err)
	}
	c.index = idx
	c.byID = make(map[string]*portal.Dataset, len(idx.Datasets))
	for i := range idx.Datasets {
		c.byID[idx.Datasets[i].ID] = &idx.Datasets[i]
	}
	return nil
}

// open reads the index from the cache while it is fresh, otherwise downloads
// it. A stale cache still serves when the download fails.
func (c *Catalog) open(ctx context.Context) (*Index, error) {
	if !strings.HasPrefix(c.cfg.IndexURL, "http") {
		return Read(c.cfg.IndexURL)
	}
	var cache string
	if c.cfg.CacheDir != "" {
		cache = filepath.Join(c.cfg.CacheDir, "index.json")
		if info, err := os.Stat(cache); err == nil && time.Since(info.ModTime()) < maxAge {
			return Read(cache)
		}
	}
	data, err := c.download(ctx)
	if err != nil {
		if cache != "" {
			if idx, readErr := Read(cache); readErr == nil && len(idx.Datasets) > 0 {
				return idx, nil
			}
		}
		return nil, err
	}
	if cache != "" {
		// A failed cache write only costs the next download
		if err := os.MkdirAll(c.cfg.CacheDir, 0o755); err == nil {
			_ = os.WriteFile(cache, data, 0o644)
		}
	}
	return parse(data)
}

func (c *Catalog) download(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.IndexURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: c.cfg.RequestTimeout}).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(c.cfg.IndexURL + " returned status " + resp.Status)
	}
	return io.ReadAll(resp.Body)
}
