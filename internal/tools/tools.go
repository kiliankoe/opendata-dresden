package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kiliankoe/opendata-dresden/internal/config"
	"github.com/kiliankoe/opendata-dresden/internal/portal"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var portalClient *portal.Client

// RegisterTools registers all MCP tools with the server
func RegisterTools(server *mcp.Server, cfg *config.Config) {
	portalClient = portal.NewClient(cfg)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_datasets",
		Description: "Search Dresden's OpenData portal for datasets. An empty query lists all datasets. Use German search terms (e.g. 'Straßenbahn' not 'tram'); the search is fuzzy, so try synonyms and official terms. Each dataset lists its resources (CSV, JSON, GeoJSON, WMS, WFS, ...) with their URLs.",
	}, SearchDatasets)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_dataset_info",
		Description: "Get a dataset with all its resources by ID",
	}, GetDatasetInfo)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "fetch_dataset",
		Description: "Download a dataset resource in one of its formats. CSV, JSON and GeoJSON return data; WMS and WFS return the service's capabilities document. Geodata layers return all features unless narrowed by limit or bbox.",
	}, FetchDataset)
}

// SearchDatasetsParams parameters for searching datasets
type SearchDatasetsParams struct {
	Query  string `json:"query,omitempty" jsonschema:"Search query in German (e.g. 'Straßenbahn' or 'Bevölkerung'); empty lists all datasets"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum number of results (default: 30)"`
	Offset int    `json:"offset,omitempty" jsonschema:"Number of results to skip for pagination"`
}

// SearchDatasets searches for datasets
func SearchDatasets(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[SearchDatasetsParams]) (*mcp.CallToolResultFor[any], error) {
	args := params.Arguments
	if args.Limit == 0 {
		args.Limit = 30
	}

	result, err := portalClient.SearchDatasets(ctx, args.Query, args.Limit, args.Offset)
	if err != nil {
		return nil, fmt.Errorf("searching datasets: %w", err)
	}
	return jsonResult(result)
}

// GetDatasetInfoParams parameters for getting dataset info
type GetDatasetInfoParams struct {
	ID string `json:"id" jsonschema:"required,Dataset ID as returned by search_datasets"`
}

// GetDatasetInfo gets detailed dataset information
func GetDatasetInfo(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[GetDatasetInfoParams]) (*mcp.CallToolResultFor[any], error) {
	dataset, err := portalClient.GetDataset(ctx, params.Arguments.ID)
	if err != nil {
		return nil, fmt.Errorf("getting dataset info: %w", err)
	}
	return jsonResult(dataset)
}

// FetchDatasetParams parameters for fetching dataset
type FetchDatasetParams struct {
	ID     string `json:"id" jsonschema:"required,Dataset ID as returned by search_datasets"`
	Format string `json:"format" jsonschema:"required,One of the dataset's resource formats, e.g. CSV, JSON, GEOJSON, WFS, WMS"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum number of features for geodata layers (CSV and GEOJSON)"`
	BBox   string `json:"bbox,omitempty" jsonschema:"Only return features of geodata layers inside this WGS84 bounding box: minLon,minLat,maxLon,maxLat"`
}

// FetchDataset fetches dataset content
func FetchDataset(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[FetchDatasetParams]) (*mcp.CallToolResultFor[any], error) {
	args := params.Arguments

	dataset, err := portalClient.GetDataset(ctx, args.ID)
	if err != nil {
		return nil, fmt.Errorf("getting dataset for fetch: %w", err)
	}
	data, err := portalClient.FetchResource(ctx, dataset, args.Format, portal.FetchOptions{Limit: args.Limit, BBox: args.BBox})
	if err != nil {
		return nil, fmt.Errorf("fetching dataset: %w", err)
	}
	return textResult(string(data)), nil
}

func jsonResult(v any) (*mcp.CallToolResultFor[any], error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshaling result: %w", err)
	}
	return textResult(string(data)), nil
}

func textResult(text string) *mcp.CallToolResultFor[any] {
	return &mcp.CallToolResultFor[any]{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}
