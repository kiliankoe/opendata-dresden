// Package arcgis lists the public feature services of the city's ArcGIS
// Online organization as datasets and fetches their layers. The organization
// publishes data the OpenData portal lacks, such as road closures, carsharing
// stations and property market figures.
package arcgis

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kiliankoe/opendata-dresden/internal/config"
	"github.com/kiliankoe/opendata-dresden/internal/portal"
	"golang.org/x/sync/errgroup"
)

const (
	// Portal is the value of Dataset.Portal for datasets from this source
	Portal = "ArcGIS Online"
	// FeatureServer is the resource format of a layer's REST endpoint
	FeatureServer = "FeatureServer"
	// orgID and orgName identify the organization of the Amt für Geodaten und
	// Kataster; orgName serves as the source of items without credits
	orgID   = "ORpvigFPJUhb8RDF"
	orgName = "Amt für Geodaten und Kataster"
	// workers bounds the parallel service requests while listing
	workers = 4
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(cfg *config.Config) *Client {
	return &Client{baseURL: cfg.ArcGISURL, httpClient: &http.Client{Timeout: cfg.RequestTimeout}}
}

// item is the subset of an ArcGIS Online item the datasets are made of
type item struct {
	ID                string `json:"id"`
	Title             string `json:"title"`
	Snippet           string `json:"snippet"`
	Description       string `json:"description"`
	LicenseInfo       string `json:"licenseInfo"`
	AccessInformation string `json:"accessInformation"`
	Modified          int64  `json:"modified"` // milliseconds since the epoch
	URL               string `json:"url"`
}

// layer is one feature layer of a service, with the date of its last edit
// once the layer's own endpoint has been read
type layer struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	LastEdit int64  // milliseconds since the epoch, zero if unknown
}

// ListDatasets returns one dataset per layer of every public feature service
// of the organization
func (c *Client) ListDatasets(ctx context.Context) ([]portal.Dataset, error) {
	items, err := c.searchItems(ctx)
	if err != nil {
		return nil, err
	}
	perItem := make([][]portal.Dataset, len(items))
	group, ctx := errgroup.WithContext(ctx)
	group.SetLimit(workers)
	for i, it := range items {
		group.Go(func() error {
			layers, err := c.layers(ctx, it.URL)
			if err != nil {
				return fmt.Errorf("listing layers of %s: %w", it.Title, err)
			}
			perItem[i] = convert(it, layers)
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	var datasets []portal.Dataset
	for _, ds := range perItem {
		datasets = append(datasets, ds...)
	}
	return datasets, nil
}

// searchItems pages through the organization's public feature services
func (c *Client) searchItems(ctx context.Context) ([]item, error) {
	var items []item
	for start := 1; start > 0; {
		query := url.Values{
			"q":     {fmt.Sprintf(`orgid:%s type:"Feature Service"`, orgID)},
			"f":     {"json"},
			"num":   {"100"},
			"start": {strconv.Itoa(start)},
		}
		var page struct {
			Results   []item `json:"results"`
			NextStart int    `json:"nextStart"`
		}
		if err := c.getJSON(ctx, c.baseURL+"/sharing/rest/search?"+query.Encode(), &page); err != nil {
			return nil, fmt.Errorf("searching ArcGIS Online: %w", err)
		}
		items = append(items, page.Results...)
		start = page.NextStart
	}
	return items, nil
}

// layers reads the service's layer list and each layer's last edit date,
// which only the layer's own endpoint reports
func (c *Client) layers(ctx context.Context, serviceURL string) ([]layer, error) {
	var service struct {
		Layers []layer `json:"layers"`
	}
	if err := c.getJSON(ctx, serviceURL+"?f=json", &service); err != nil {
		return nil, err
	}
	for i := range service.Layers {
		var info struct {
			EditingInfo struct {
				LastEditDate int64 `json:"lastEditDate"`
			} `json:"editingInfo"`
		}
		if err := c.getJSON(ctx, fmt.Sprintf("%s/%d?f=json", serviceURL, service.Layers[i].ID), &info); err != nil {
			return nil, err
		}
		service.Layers[i].LastEdit = info.EditingInfo.LastEditDate
	}
	return service.Layers, nil
}

// convert makes a dataset of every layer. Layers of a service with several
// carry the layer name in their title to stay distinguishable.
func convert(it item, layers []layer) []portal.Dataset {
	datasets := make([]portal.Dataset, 0, len(layers))
	for _, l := range layers {
		title := it.Title
		if len(layers) > 1 {
			title += " - " + l.Name
		}
		updated := it.Modified
		if l.LastEdit > 0 {
			updated = l.LastEdit
		}
		source := it.AccessInformation
		if source == "" {
			source = orgName
		}
		description := plainText(it.Description)
		if description == "" {
			description = plainText(it.Snippet)
		}
		layerURL := fmt.Sprintf("%s/%d", it.URL, l.ID)
		datasets = append(datasets, portal.Dataset{
			ID:      fmt.Sprintf("%s-%d", it.ID, l.ID),
			Title:   title,
			Updated: time.UnixMilli(updated).UTC().Format("02.01.2006"),
			Source:  source,
			License: plainText(it.LicenseInfo),
			Portal:  Portal,
			Resources: []portal.Resource{
				{Format: "GEOJSON", URL: layerURL + "/query?where=1%3D1&outFields=*&f=geojson"},
				{Format: FeatureServer, URL: layerURL},
			},
			Description: description,
		})
	}
	return datasets
}

var tags = regexp.MustCompile(`<[^>]*>`)

// plainText strips the HTML that item texts are written in
func plainText(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(tags.ReplaceAllString(s, " "))), " ")
}

// FetchResource downloads the dataset's resource in the given format. GeoJSON
// is collected across the service's result pages unless a limit is given;
// the layer endpoint returns the layer's description.
func (c *Client) FetchResource(ctx context.Context, dataset *portal.Dataset, format string, opts portal.FetchOptions) ([]byte, error) {
	for _, r := range dataset.Resources {
		if !strings.EqualFold(r.Format, format) {
			continue
		}
		if r.Format == FeatureServer {
			return c.get(ctx, r.URL+"?f=json")
		}
		return c.queryFeatures(ctx, r.URL, opts)
	}
	return nil, fmt.Errorf("format %s not available for dataset %s", format, dataset.ID)
}

func (c *Client) queryFeatures(ctx context.Context, queryURL string, opts portal.FetchOptions) ([]byte, error) {
	target, err := url.Parse(queryURL)
	if err != nil {
		return nil, err
	}
	params := target.Query()
	if opts.BBox != "" {
		params.Set("geometry", opts.BBox)
		params.Set("geometryType", "esriGeometryEnvelope")
		params.Set("inSR", "4326")
	}
	if opts.Limit > 0 {
		params.Set("resultRecordCount", strconv.Itoa(opts.Limit))
	}
	var features []json.RawMessage
	for {
		target.RawQuery = params.Encode()
		var page struct {
			Features   []json.RawMessage `json:"features"`
			Properties struct {
				ExceededTransferLimit bool `json:"exceededTransferLimit"`
			} `json:"properties"`
		}
		if err := c.getJSON(ctx, target.String(), &page); err != nil {
			return nil, err
		}
		features = append(features, page.Features...)
		if opts.Limit > 0 || !page.Properties.ExceededTransferLimit || len(page.Features) == 0 {
			break
		}
		params.Set("resultOffset", strconv.Itoa(len(features)))
	}
	if features == nil {
		features = []json.RawMessage{}
	}
	return json.Marshal(struct {
		Type     string            `json:"type"`
		Features []json.RawMessage `json:"features"`
	}{"FeatureCollection", features})
}

func (c *Client) getJSON(ctx context.Context, target string, v any) error {
	body, err := c.get(ctx, target)
	if err != nil {
		return err
	}
	// ArcGIS reports errors with status 200 and an error object
	var failure struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &failure) == nil && failure.Error != nil {
		return fmt.Errorf("%s: %s", target, failure.Error.Message)
	}
	return json.Unmarshal(body, v)
}

func (c *Client) get(ctx context.Context, target string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
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
		return nil, fmt.Errorf("%s returned status %d", target, resp.StatusCode)
	}
	return body, nil
}
