package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kiliankoe/opendatadresdenmcp/internal/config"
	"github.com/kiliankoe/opendatadresdenmcp/internal/portal"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var portalClient *portal.Client

// RegisterTools registers all MCP tools with the server
func RegisterTools(server *mcp.Server, cfg *config.Config) {
	portalClient = portal.NewClient(cfg)

	// List datasets tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_datasets",
		Description: "List all available datasets from Dresden OpenData portal",
	}, ListDatasets)

	// Search datasets tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_datasets",
		Description: "Search for datasets using keywords. Note: Use German search terms (e.g., 'Straßenbahn' not 'tram'). The underlying API has limited search capabilities - try multiple queries with synonyms, different forms, and bureaucratic terms for best results.",
	}, SearchDatasets)

	// Get dataset info tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_dataset_info",
		Description: "Get detailed information about a specific dataset",
	}, GetDatasetInfo)

	// Fetch dataset tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "fetch_dataset",
		Description: "Fetch dataset content in specified format",
	}, FetchDataset)

	// Query dataset tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "query_dataset",
		Description: "Query dataset with OGC API parameters",
	}, QueryDataset)
}

// ListDatasetsParams parameters for listing datasets
type ListDatasetsParams struct {
	Limit int `json:"limit,omitempty" jsonschema:"Maximum number of datasets to return (default: 100)"`
}

// ListDatasets lists available datasets
func ListDatasets(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[ListDatasetsParams]) (*mcp.CallToolResultFor[any], error) {
	args := params.Arguments
	if args.Limit == 0 {
		args.Limit = 100
	}

	datasets, err := portalClient.ListDatasets(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing datasets: %w", err)
	}

	// Limit results
	if len(datasets) > args.Limit {
		datasets = datasets[:args.Limit]
	}

	// Convert to JSON for response
	jsonData, err := json.Marshal(datasets)
	if err != nil {
		return nil, fmt.Errorf("marshaling datasets: %w", err)
	}

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: string(jsonData),
			},
		},
	}, nil
}

// SearchDatasetsParams parameters for searching datasets
type SearchDatasetsParams struct {
	Query string `json:"query" jsonschema:"required,Search query keywords (use German terms like 'Straßenbahn' or 'Parkplatz')"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum number of results (default: 30)"`
}

// SearchDatasets searches for datasets
func SearchDatasets(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[SearchDatasetsParams]) (*mcp.CallToolResultFor[any], error) {
	args := params.Arguments
	if args.Limit == 0 {
		args.Limit = 30
	}

	datasets, err := portalClient.SearchDatasets(ctx, args.Query, args.Limit)
	if err != nil {
		return nil, fmt.Errorf("searching datasets: %w", err)
	}

	jsonData, err := json.Marshal(datasets)
	if err != nil {
		return nil, fmt.Errorf("marshaling search results: %w", err)
	}

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: string(jsonData),
			},
		},
	}, nil
}

// GetDatasetInfoParams parameters for getting dataset info
type GetDatasetInfoParams struct {
	ID string `json:"id" jsonschema:"required,Dataset ID or Node ID or Layer ID"`
}

// GetDatasetInfo gets detailed dataset information
func GetDatasetInfo(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[GetDatasetInfoParams]) (*mcp.CallToolResultFor[any], error) {
	args := params.Arguments

	dataset, err := portalClient.GetDatasetInfo(ctx, args.ID)
	if err != nil {
		return nil, fmt.Errorf("getting dataset info: %w", err)
	}

	jsonData, err := json.Marshal(dataset)
	if err != nil {
		return nil, fmt.Errorf("marshaling dataset info: %w", err)
	}

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: string(jsonData),
			},
		},
	}, nil
}

// FetchDatasetParams parameters for fetching dataset
type FetchDatasetParams struct {
	ID     string `json:"id" jsonschema:"required,Dataset ID or Node ID"`
	Format string `json:"format,omitempty" jsonschema:"Desired format: CSV JSON GeoJSON WMS WFS (default: JSON)"`
}

// FetchDataset fetches dataset content
func FetchDataset(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[FetchDatasetParams]) (*mcp.CallToolResultFor[any], error) {
	args := params.Arguments
	if args.Format == "" {
		args.Format = "JSON"
	}

	// Get dataset info first
	dataset, err := portalClient.GetDatasetInfo(ctx, args.ID)
	if err != nil {
		return nil, fmt.Errorf("getting dataset for fetch: %w", err)
	}

	// Fetch the data
	data, err := portalClient.FetchDataset(ctx, dataset, args.Format)
	if err != nil {
		return nil, fmt.Errorf("fetching dataset: %w", err)
	}

	// Convert response based on type
	var responseText string
	switch v := data.(type) {
	case string:
		responseText = v
	default:
		jsonData, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("marshaling fetched data: %w", err)
		}
		responseText = string(jsonData)
	}

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: responseText,
			},
		},
	}, nil
}

// QueryDatasetParams parameters for querying dataset
type QueryDatasetParams struct {
	LayerID string `json:"layerId" jsonschema:"required,OGC layer ID e.g. L124"`
	Limit   int    `json:"limit,omitempty" jsonschema:"Maximum number of features (default: 10)"`
	Offset  int    `json:"offset,omitempty" jsonschema:"Offset for pagination"`
}

// QueryDataset queries dataset with OGC API parameters
func QueryDataset(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[QueryDatasetParams]) (*mcp.CallToolResultFor[any], error) {
	args := params.Arguments
	if args.Limit == 0 {
		args.Limit = 10
	}

	// Fetch from OGC API
	ogcData, err := portalClient.FetchOGCAPIData(ctx, args.LayerID, args.Limit, args.Offset)
	if err != nil {
		return nil, fmt.Errorf("querying OGC API: %w", err)
	}

	jsonData, err := json.Marshal(ogcData)
	if err != nil {
		return nil, fmt.Errorf("marshaling query results: %w", err)
	}

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: string(jsonData),
			},
		},
	}, nil
}
