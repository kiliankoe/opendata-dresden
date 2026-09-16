package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/kiliankoe/opendata-dresden/internal/cli"
	"github.com/kiliankoe/opendata-dresden/internal/config"
	"github.com/kiliankoe/opendata-dresden/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	ctx := context.Background()
	cfg := config.LoadConfig()

	// Without arguments the binary is an MCP server, which is how MCP clients launch it
	if len(os.Args) > 1 {
		if err := cli.Run(ctx, cfg, os.Args[1:], os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "dresden-opendata",
		Version: config.Version,
	}, nil)
	tools.RegisterTools(server, cfg)

	log.Printf("Starting Dresden OpenData MCP server %s...", config.Version)
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
