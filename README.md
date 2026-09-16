# Dresden OpenData MCP Server

A Model Context Protocol (MCP) server that provides AI agents (or [your toaster](https://worksonmymachine.ai/p/mcp-an-accidentally-universal-plugin)) with access to [Dresden's OpenData Portal](https://opendata.dresden.de), enabling these systems to automatically discover, fetch, and process open civic datasets published by the city or other providers.

I personally find Dresden's OpenData portal extremely useful, but cumbersome to work with and find fitting datasets. This tries to make that a bit easier. [Here](https://claude.ai/share/41f6ee24-1f5d-4e54-9d34-1645ad55b457)'s an example interaction with Claude using this MCP server to find a dataset on Dresden's streets.

## Installation

### Prerequisites

- Go 1.24 or higher, or [Nix](https://nixos.org) with flakes enabled
- An MCP-compatible AI client (e.g. Claude Desktop)

### Building from Source

```bash
git clone https://github.com/kiliankoe/dresden-opendata-mcp.git
cd dresden-opendata-mcp

make build

make install
```

With Nix, `nix build` produces the binary at `result/bin/dresden-opendata-mcp`, and `nix develop` (or [direnv](https://direnv.net) with `direnv allow`) provides a shell with Go and its tooling.

## Configuration

The server can be configured using environment variables:

```bash
REQUEST_TIMEOUT_MS=30000
```

## MCP Tools

The server provides the following MCP tools:

### `search_datasets`
Search for datasets. An empty query lists all datasets. Every result carries its ID and the available resources (CSV, JSON, GeoJSON, WMS, WFS, tables, charts, ...) with their URLs.

**Important search tips:**
- Use German search terms for better results (e.g., "Straßenbahn" instead of "tram")
- The portal's search is fuzzy and has limited ranking - try multiple queries with:
  - Different synonyms (e.g., "ÖPNV", "Nahverkehr", "öffentlicher Verkehr")
  - Various forms (singular/plural, abbreviations)
  - More bureaucratic/official terms
- Consider broader or narrower terms if initial searches don't yield results

```js
{
  "query": "Straßenbahn",
  "limit": 30,  // Optional, default 30
  "offset": 0   // Optional, for pagination
}
```

### `get_dataset_info`
Get a dataset with all its resources by ID.

```js
{
  "id": "0F6996E7-26AB-4585-81BD-1EDA4381B1BC"
}
```

### `fetch_dataset`
Download one of a dataset's resources. CSV, JSON and GeoJSON return data, WMS and WFS return the service's capabilities document.

Geodata layers return all their features by default. The portal offers no paging, so `limit` and `bbox` are the only ways to narrow a large layer.

```js
{
  "id": "0F6996E7-26AB-4585-81BD-1EDA4381B1BC",
  "format": "GEOJSON",
  "limit": 50,                          // Optional, geodata layers only
  "bbox": "13.72,51.04,13.76,51.07"     // Optional, WGS84 minLon,minLat,maxLon,maxLat
}
```

## Command line usage

The same three operations are available as commands, for agents or scripts that prefer running a binary over speaking MCP. Search and info print JSON, fetch prints the resource as is.

```bash
dresden-opendata-mcp search --limit 5 Straßenbahn
dresden-opendata-mcp info 0F6996E7-26AB-4585-81BD-1EDA4381B1BC
dresden-opendata-mcp fetch --limit 50 --bbox 13.72,51.04,13.76,51.07 0F6996E7-26AB-4585-81BD-1EDA4381B1BC GEOJSON
```

Without arguments the binary runs the MCP server on stdio.

## Usage with Claude Desktop

Add the server to your Claude Desktop configuration:

```js
{
  "mcpServers": {
    "dresden-opendata": {
      "command": "/path/to/dresden-opendata-mcp"
    }
  }
}
```

## Example Usage

```typescript
// Search for tram datasets (use German terms)
const { datasets } = await client.call('search_datasets', {
  query: 'Straßenbahn'
});

// Fetch the first one as GeoJSON, limited to 50 features
const geojson = await client.call('fetch_dataset', {
  id: datasets[0].id,
  format: 'GEOJSON',
  limit: 50
});
```

## Development

```bash
make build

make test

make test-integration  # also runs tests against the live portal

make fmt
```

**Note:** The server communicates via stdin/stdout using the MCP protocol. Running `make run` directly will cause the server to wait for MCP protocol messages, which may appear as if it's hanging. This is normal - the server should be used with an MCP client. You can use [MCP Inspector](https://modelcontextprotocol.io/legacy/tools/inspector) to directly introspect this server or set up a client like Claude Desktop, see above.

## Architecture

The server consists of several key components:

- **Portal Client**: Talks to the search backend of the portal's web app, which is the only place listing every dataset with all its resources
- **MCP Tools** and **CLI**: Two thin front ends over the portal client
- **Configuration**: Manages environment-based configuration
