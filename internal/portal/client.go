package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/kiliankoe/opendatadresdenmcp/internal/config"
)

// Client handles communication with Dresden's OpenData portal
type Client struct {
	config     *config.Config
	httpClient *http.Client
}

func NewClient(cfg *config.Config) *Client {
	return &Client{
		config: cfg,
		httpClient: &http.Client{
			Timeout: cfg.RequestTimeout,
		},
	}
}

// SearchDatasets searches for datasets using client-side filtering on OGC collections
func (c *Client) SearchDatasets(ctx context.Context, query string, limit int) ([]Dataset, error) {
	// Fetch all collections
	allDatasets, err := c.fetchOGCCollections(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching collections for search: %w", err)
	}

	if query == "" {
		// Return all datasets up to limit
		if limit > 0 && limit < len(allDatasets) {
			return allDatasets[:limit], nil
		}
		return allDatasets, nil
	}

	queryLower := strings.ToLower(query)
	queryTerms := strings.Fields(queryLower)

	// Filter datasets based on query
	var filtered []Dataset
	for _, dataset := range allDatasets {
		if matchesQuery(dataset, queryTerms) {
			filtered = append(filtered, dataset)
			if limit > 0 && len(filtered) >= limit {
				break
			}
		}
	}

	return filtered, nil
}

// ListDatasets returns all available datasets from OGC API
func (c *Client) ListDatasets(ctx context.Context) ([]Dataset, error) {
	return c.fetchOGCCollections(ctx)
}

// FetchDataset fetches a dataset in the specified format
func (c *Client) FetchDataset(ctx context.Context, dataset *Dataset, format string) (interface{}, error) {
	dataURL, exists := dataset.DataURLs[strings.ToLower(format)]
	if !exists {
		return nil, fmt.Errorf("format %s not available for dataset %s", format, dataset.ID)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", dataURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating fetch request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching dataset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse based on format
	switch strings.ToLower(format) {
	case "json", "geojson":
		var data interface{}
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			return nil, fmt.Errorf("decoding JSON data: %w", err)
		}
		return data, nil
	case "csv":
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("reading CSV data: %w", err)
		}
		return string(data), nil
	default:
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("reading data: %w", err)
		}
		return data, nil
	}
}

// FetchOGCAPIData fetches data from OGC API Features endpoint
func (c *Client) FetchOGCAPIData(ctx context.Context, layerID string, limit int, offset int) (*OGCAPIResponse, error) {
	ogcURL := fmt.Sprintf("%s%s/collections/%s/items", config.KommisDDURL, config.OGCAPIPath, layerID)

	params := url.Values{}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", offset))
	}

	if len(params) > 0 {
		ogcURL += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, "GET", ogcURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating OGC API request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing OGC API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OGC API returned status %d: %s", resp.StatusCode, string(body))
	}

	var ogcResp OGCAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&ogcResp); err != nil {
		return nil, fmt.Errorf("decoding OGC API response: %w", err)
	}

	return &ogcResp, nil
}

// GetDatasetInfo retrieves detailed information about a specific dataset
func (c *Client) GetDatasetInfo(ctx context.Context, datasetID string) (*Dataset, error) {
	// Try to fetch specific collection from OGC API
	ogcURL := fmt.Sprintf("%s%s/collections/%s", config.KommisDDURL, config.OGCAPIPath, datasetID)

	req, err := http.NewRequestWithContext(ctx, "GET", ogcURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating collection request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching collection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var collection OGCCollection
		if err := json.NewDecoder(resp.Body).Decode(&collection); err != nil {
			return nil, fmt.Errorf("decoding collection response: %w", err)
		}
		dataset := c.convertOGCCollection(collection)
		return &dataset, nil
	}

	// Fallback: search through all collections
	datasets, err := c.fetchOGCCollections(ctx)
	if err != nil {
		return nil, err
	}

	for _, ds := range datasets {
		if ds.ID == datasetID || ds.LayerID == datasetID {
			return &ds, nil
		}
	}

	return nil, fmt.Errorf("dataset %s not found", datasetID)
}

// fetchOGCCollections fetches all collections from OGC API
func (c *Client) fetchOGCCollections(ctx context.Context) ([]Dataset, error) {
	ogcURL := fmt.Sprintf("%s%s/collections", config.KommisDDURL, config.OGCAPIPath)

	req, err := http.NewRequestWithContext(ctx, "GET", ogcURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating collections request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching collections: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OGC collections API returned status %d: %s", resp.StatusCode, string(body))
	}

	var collectionsResp OGCCollectionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&collectionsResp); err != nil {
		return nil, fmt.Errorf("decoding collections response: %w", err)
	}

	datasets := make([]Dataset, 0, len(collectionsResp.Collections))
	for _, collection := range collectionsResp.Collections {
		datasets = append(datasets, c.convertOGCCollection(collection))
	}

	return datasets, nil
}

// convertOGCCollection converts an OGC collection to a Dataset
func (c *Client) convertOGCCollection(collection OGCCollection) Dataset {
	dataset := Dataset{
		ID:          collection.ID,
		LayerID:     collection.ID,
		Title:       collection.Title,
		Description: collection.Description,
		DataURLs:    make(map[string]string),
		Formats:     []string{"GeoJSON", "JSON"},
	}

	// Extract data URLs from links
	for _, link := range collection.Links {
		if link.Rel == "items" {
			dataset.DataURLs["geojson"] = link.Href
			dataset.DataURLs["json"] = link.Href
		}
	}

	// Set spatial extent if available
	if collection.Extent != nil && collection.Extent.Spatial != nil {
		dataset.SpatialExtent = &SpatialExtent{
			BBox: collection.Extent.Spatial.BBox,
			CRS:  collection.CRS,
		}
	}

	return dataset
}

// matchesQuery checks if a dataset matches the search query terms
func matchesQuery(dataset Dataset, queryTerms []string) bool {
	searchText := strings.ToLower(dataset.Title + " " + dataset.Description + " " + dataset.ID)

	for _, term := range queryTerms {
		if !strings.Contains(searchText, term) {
			return false
		}
	}

	return true
}
