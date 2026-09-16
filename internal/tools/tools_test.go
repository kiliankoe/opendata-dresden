package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/kiliankoe/opendata-dresden/internal/config"
	"github.com/kiliankoe/opendata-dresden/internal/index"
	"github.com/kiliankoe/opendata-dresden/internal/portal"
	"github.com/kiliankoe/opendata-dresden/internal/portal/portaltest"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connect wires the tools into a server backed by a fake portal and returns a
// client session talking to it over an in-memory transport.
func connect(t *testing.T) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()

	fake := portaltest.New(t)
	cfg := &config.Config{PortalURL: fake.URL, IndexURL: filepath.Join(t.TempDir(), "index.json")}
	idx, _, err := index.Build(ctx, portal.NewClient(cfg), &index.Index{})
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.Write(cfg.IndexURL); err != nil {
		t.Fatal(err)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	RegisterTools(server, cfg)

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "client"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestRegisterTools(t *testing.T) {
	session := connect(t)
	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"search_datasets": true, "get_dataset_info": true, "fetch_dataset": true}
	if len(res.Tools) != len(want) {
		t.Errorf("got %d tools, want %d", len(res.Tools), len(want))
	}
	for _, tool := range res.Tools {
		if !want[tool.Name] {
			t.Errorf("unexpected tool %q", tool.Name)
		}
	}
}

func TestSearchDatasetsTool(t *testing.T) {
	session := connect(t)
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "search_datasets",
		Arguments: map[string]any{"query": "Wanderwege"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || len(res.Content) != 1 {
		t.Fatalf("unexpected result %+v", res)
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content is %T, want *mcp.TextContent", res.Content[0])
	}
	var result portal.SearchResult
	if err := json.Unmarshal([]byte(text.Text), &result); err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || len(result.Datasets) != 1 || result.Datasets[0].ID != "D1" {
		t.Errorf("got %+v, want only D1", result)
	}
}

// The SDK derives required properties from the absence of omitempty and uses
// the jsonschema tag verbatim as the description.
func TestFetchDatasetSchema(t *testing.T) {
	session := connect(t)
	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range res.Tools {
		if tool.Name != "fetch_dataset" {
			continue
		}
		schema, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		var got struct {
			Required   []string                  `json:"required"`
			Properties map[string]map[string]any `json:"properties"`
		}
		if err := json.Unmarshal(schema, &got); err != nil {
			t.Fatal(err)
		}
		if len(got.Required) != 2 || got.Required[0] != "id" || got.Required[1] != "format" {
			t.Errorf("required = %v, want [id format]", got.Required)
		}
		if desc, _ := got.Properties["id"]["description"].(string); desc != "Dataset ID as returned by search_datasets" {
			t.Errorf("id description = %q", desc)
		}
		return
	}
	t.Fatal("fetch_dataset not registered")
}
