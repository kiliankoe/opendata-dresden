package portal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/kiliankoe/opendatadresdenmcp/internal/config"
)

const (
	// guestGroupID is the user group of the portal's anonymous "opendata" user.
	// It is what /service/app/user/groupids returns for the guest user ID in
	// /service/app/config. Without it the search returns nothing.
	guestGroupID = "19"
	// datasetIDField is the search index field holding the dataset ID
	datasetIDField = "ergebnis_id"
)

// searchFields are the index fields the portal's web app searches by default
var searchFields = []string{"ergebnis_bezeichnung_ngram", "themen", "doc_content_nwst_ngram", "text", "content"}

var layerIDPattern = regexp.MustCompile(`/ogcapi/collections/(L\d+)$`)

// Client handles communication with Dresden's OpenData portal
type Client struct {
	portalURL  string
	httpClient *http.Client
}

func NewClient(cfg *config.Config) *Client {
	return &Client{
		portalURL: cfg.PortalURL,
		httpClient: &http.Client{
			Timeout: cfg.RequestTimeout,
		},
	}
}

// SearchDatasets runs a full-text search. An empty query lists all datasets.
func (c *Client) SearchDatasets(ctx context.Context, query string, limit, offset int) (*SearchResult, error) {
	return c.search(ctx, query, searchFields, limit, offset)
}

// GetDataset looks up a single dataset by its ID
func (c *Client) GetDataset(ctx context.Context, id string) (*Dataset, error) {
	result, err := c.search(ctx, id, []string{datasetIDField}, 1, 0)
	if err != nil {
		return nil, err
	}
	if len(result.Datasets) == 0 {
		return nil, fmt.Errorf("dataset %s not found", id)
	}
	return &result.Datasets[0], nil
}

// FetchResource downloads the dataset's resource in the given format. The
// query only applies to OGC API resources; they accept limit and bbox and
// otherwise return whole layers.
func (c *Client) FetchResource(ctx context.Context, dataset *Dataset, format string, query url.Values) ([]byte, error) {
	for _, r := range dataset.Resources {
		if !strings.EqualFold(r.Format, format) {
			continue
		}
		resourceURL := r.URL
		if len(query) > 0 && strings.Contains(resourceURL, "/ogcapi/") {
			sep := "?"
			if strings.Contains(resourceURL, "?") {
				sep = "&"
			}
			resourceURL += sep + query.Encode()
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, resourceURL, nil)
		if err != nil {
			return nil, err
		}
		return c.do(req)
	}
	return nil, fmt.Errorf("format %s not available for dataset %s", format, dataset.ID)
}

func (c *Client) search(ctx context.Context, text string, fields []string, limit, offset int) (*SearchResult, error) {
	request := searchRequest{
		TextSearch:   text,
		NumOfResults: limit,
		PagingStart:  offset,
		UserGroupIDs: []string{guestGroupID},
	}
	for _, field := range fields {
		request.TextSearchConfig.Attribute = append(request.TextSearchConfig.Attribute, searchAttribute{Name: field})
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.portalURL+"/service/app/search/all", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	respBody, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("searching portal: %w", err)
	}

	var response searchResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("decoding search response: %w", err)
	}

	result := &SearchResult{Total: response.NumOfResults, Datasets: make([]Dataset, 0, len(response.Results))}
	for _, r := range response.Results {
		result.Datasets = append(result.Datasets, convertResult(r))
	}
	return result, nil
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned status %d", req.URL, resp.StatusCode)
	}
	return body, nil
}

func convertResult(r searchResult) Dataset {
	dataset := Dataset{
		Title:     r.Title,
		Updated:   r.Updated,
		Source:    r.DataSource.Name,
		License:   r.License.Name,
		Topics:    unique(r.Topics),
		Regions:   unique(r.Regions),
		Years:     unique(r.Years),
		Resources: make([]Resource, 0, len(r.Presentations)),
	}

	for _, p := range r.Presentations {
		dataset.ID = p.DatasetID
		resourceURL := p.URL
		if p.BaseURL != "" {
			// Interactive views (tables, charts) carry a name to append to a base URL
			resourceURL = p.BaseURL + (&url.URL{Path: p.URL}).EscapedPath()
		}
		if m := layerIDPattern.FindStringSubmatch(resourceURL); m != nil {
			// The GeoJSON link points at the collection description; the features live under /items
			dataset.LayerID = m[1]
			resourceURL += "/items"
		}
		dataset.Resources = append(dataset.Resources, Resource{Format: p.Format, URL: resourceURL})
	}

	return dataset
}

// unique drops repeated values; the portal lists every topic and year twice
func unique(values []string) []string {
	var result []string
	seen := make(map[string]bool, len(values))
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}
