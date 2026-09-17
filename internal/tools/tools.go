package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kiliankoe/opendata-dresden/internal/config"
	"github.com/kiliankoe/opendata-dresden/internal/index"
	"github.com/kiliankoe/opendata-dresden/internal/portal"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var catalog *index.Catalog

// RegisterTools registers all MCP tools with the server
func RegisterTools(server *mcp.Server, cfg *config.Config) {
	catalog = index.NewCatalog(cfg)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_datasets",
		Description: "Search Dresden's OpenData datasets. Every word of the query has to appear in a dataset's title, topics, source or description; results with matches in the title rank first. Use German terms (e.g. 'Straßenbahn' not 'tram'). An empty query lists all datasets. Each dataset lists its resources (CSV, JSON, GeoJSON, WMS, WFS, ...) with their URLs.",
	}, SearchDatasets)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_dataset_info",
		Description: "Get a dataset with all its resources by ID",
	}, GetDatasetInfo)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "fetch_dataset",
		Description: "Download a dataset resource in one of its formats. CSV, JSON and GeoJSON return data; WMS and WFS return the service's capabilities document. Geodata layers return all features unless narrowed by limit or bbox.",
	}, FetchDataset)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "dataset_history",
		Description: "Show how a statistics dataset's published numbers changed over time, newest first. This repository mirrors those tables nightly, so its commit history is the only record of revisions; the portal itself publishes only the current version. Set diff to see the rows that moved. Geodata layers are not mirrored.",
	}, DatasetHistory)
}

// SearchDatasetsParams parameters for searching datasets
type SearchDatasetsParams struct {
	Query  string `json:"query,omitempty" jsonschema:"Search query in German (e.g. 'Straßenbahn' or 'Bevölkerung'); empty lists all datasets"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum number of results (default: 30)"`
	Offset int    `json:"offset,omitempty" jsonschema:"Number of results to skip for pagination"`
}

// SearchDatasets searches for datasets
func SearchDatasets(ctx context.Context, _ *mcp.CallToolRequest, args SearchDatasetsParams) (*mcp.CallToolResult, any, error) {
	if args.Limit == 0 {
		args.Limit = 30
	}

	result, err := catalog.Search(ctx, args.Query, args.Limit, args.Offset)
	if err != nil {
		return nil, nil, fmt.Errorf("searching datasets: %w", err)
	}
	return jsonResult(result)
}

// GetDatasetInfoParams parameters for getting dataset info
type GetDatasetInfoParams struct {
	ID string `json:"id" jsonschema:"Dataset ID as returned by search_datasets"`
}

// GetDatasetInfo gets detailed dataset information
func GetDatasetInfo(ctx context.Context, _ *mcp.CallToolRequest, args GetDatasetInfoParams) (*mcp.CallToolResult, any, error) {
	dataset, err := catalog.Get(ctx, args.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("getting dataset info: %w", err)
	}
	return jsonResult(dataset)
}

// FetchDatasetParams parameters for fetching dataset
type FetchDatasetParams struct {
	ID     string `json:"id" jsonschema:"Dataset ID as returned by search_datasets"`
	Format string `json:"format" jsonschema:"One of the dataset's resource formats, e.g. CSV, JSON, GEOJSON, WFS, WMS"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum number of features for geodata layers (CSV and GEOJSON)"`
	BBox   string `json:"bbox,omitempty" jsonschema:"Only return features of geodata layers inside this WGS84 bounding box: minLon,minLat,maxLon,maxLat"`
}

// FetchDataset fetches dataset content
func FetchDataset(ctx context.Context, _ *mcp.CallToolRequest, args FetchDatasetParams) (*mcp.CallToolResult, any, error) {
	data, err := catalog.Fetch(ctx, args.ID, args.Format, portal.FetchOptions{Limit: args.Limit, BBox: args.BBox})
	if err != nil {
		return nil, nil, fmt.Errorf("fetching dataset: %w", err)
	}
	return textResult(string(data)), nil, nil
}

// DatasetHistoryParams parameters for reading a dataset's recorded history
type DatasetHistoryParams struct {
	ID    string `json:"id" jsonschema:"Dataset ID as returned by search_datasets"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum number of changes to report (default: 10)"`
	Diff  bool   `json:"diff,omitempty" jsonschema:"Include the changed rows of each change in unified diff format"`
}

// DatasetHistory reports the recorded changes of a mirrored statistics table
func DatasetHistory(ctx context.Context, _ *mcp.CallToolRequest, args DatasetHistoryParams) (*mcp.CallToolResult, any, error) {
	if args.Limit == 0 {
		args.Limit = 10
	}
	history, err := catalog.History(ctx, args.ID, args.Limit, args.Diff)
	if err != nil {
		return nil, nil, fmt.Errorf("reading dataset history: %w", err)
	}
	return jsonResult(history)
}

func jsonResult(v any) (*mcp.CallToolResult, any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, nil, fmt.Errorf("marshaling result: %w", err)
	}
	return textResult(string(data)), nil, nil
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}
