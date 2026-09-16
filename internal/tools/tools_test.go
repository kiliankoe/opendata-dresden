package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kiliankoe/opendatadresdenmcp/internal/config"
	"github.com/kiliankoe/opendatadresdenmcp/internal/portal"
	"github.com/kiliankoe/opendatadresdenmcp/internal/portal/portaltest"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connect wires the tools into a server backed by a fake portal and returns a
// client session talking to it over an in-memory transport.
func connect(t *testing.T) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()

	fake := portaltest.New(t)

	server := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	RegisterTools(server, &config.Config{PortalURL: fake.URL})

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "client"}, nil).Connect(ctx, clientTransport)
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
