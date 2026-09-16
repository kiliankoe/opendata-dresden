package main

import (
	"context"
	"log"

	"github.com/kiliankoe/opendatadresdenmcp/internal/config"
	"github.com/kiliankoe/opendatadresdenmcp/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// version is also parsed by flake.nix, keep the declaration on one line.
const version = "0.1.0"

func main() {
	ctx := context.Background()

	// Load configuration
	cfg := config.LoadConfig()

	// Create MCP server
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "dresden-opendata",
		Version: version,
	}, nil)

	// Register tools
	tools.RegisterTools(server, cfg)

	// Run server with stdio transport
	log.Printf("Starting Dresden OpenData MCP server %s...", version)
	if err := server.Run(ctx, mcp.NewStdioTransport()); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
