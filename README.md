# Dresden OpenData MCP Server

A Model Context Protocol (MCP) server that provides AI agents (or [whatever](https://worksonmymachine.ai/p/mcp-an-accidentally-universal-plugin)) with access to Dresden's OpenData Portal (opendata.dresden.de), enabling these systems to automatically discover, fetch, and process open civic datasets published by the city or other providers through [Dresden's OpenData Portal](https://opendata.dresden.de).

I personally find Dresden's OpenData portal extremely useful, but cumbersome to work with and find fitting datasets. This tries to make that a bit easier. [Here](https://claude.ai/share/41f6ee24-1f5d-4e54-9d34-1645ad55b457)'s an example interaction with Claude using this MCP server to find a dataset on Dresden's streets.

## Installation

### Prerequisites

- Go 1.21 or higher
- An MCP-compatible AI client (e.g. Claude Desktop)

### Building from Source

```bash
git clone https://github.com/kiliankoe/dresden-opendata-mcp.git
cd dresden-opendata-mcp

make build

make install
```

## Configuration

The server can be configured using environment variables:

```bash
MAX_CONCURRENT_REQUESTS=3
REQUEST_TIMEOUT_MS=30000
RATE_LIMIT_PER_MINUTE=60
```

## MCP Tools

The server provides the following MCP tools:

### `list_datasets`
List all available datasets from the Dresden OpenData portal.

```js
{
  "limit": 100  // Optional, max datasets to return
}
```

### `search_datasets`
Search for datasets using keywords.

**Important search tips:**
- Use German search terms for better results (e.g., "Straßenbahn" instead of "tram")
- The underlying API has limited search capabilities - try multiple queries with:
  - Different synonyms (e.g., "ÖPNV", "Nahverkehr", "öffentlicher Verkehr")
  - Various forms (singular/plural, abbreviations)
  - More bureaucratic/official terms
- Consider broader or narrower terms if initial searches don't yield results

```js
{
  "query": "Straßenbahn",
  "limit": 30
}
```

### `get_dataset_info`
Get detailed information about a specific dataset.

```js
{
  "id": "dataset-id"  // Dataset ID, Node ID, or Layer ID
}
```

### `fetch_dataset`
Fetch dataset content in a specified format.

```js
{
  "id": "dataset-id",
  "format": "GeoJSON"  // CSV, JSON, GeoJSON, WMS, WFS
}
```

### `query_dataset`
Query datasets using OGC API parameters.

```js
{
  "layerId": "L124",
  "limit": 50,
  "offset": 0,
  "bbox": [403458.73, 5650402.65, 419770.49, 5666619.90]
}
```

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
// Search for tram routes (use German terms)
const datasets = await client.call('search_datasets', {
  query: 'Straßenbahn'
});

// Fetch specific dataset
const tramData = await client.call('fetch_dataset', {
  id: 'tram-network',
  format: 'GeoJSON'
});

// Query with spatial bounds
const results = await client.call('query_dataset', {
  layerId: 'L124',
  bbox: [403458.73, 5650402.65, 419770.49, 5666619.90],
  limit: 50
});
```

## Development

```bash
make build

make test

make fmt
```

**Note:** The server communicates via stdin/stdout using the MCP protocol. Running `make run` directly will cause the server to wait for MCP protocol messages, which may appear as if it's hanging. This is normal - the server should be used with an MCP client. You can use [MCP Inspector](https://modelcontextprotocol.io/legacy/tools/inspector) to directly introspect this server or set up a client like Claude Desktop, see above.

## Architecture

The server consists of several key components:

- **Portal Client**: Handles communication with Dresden's OpenData APIs
- **MCP Tools**: Implements the MCP protocol tools for dataset operations
- **Configuration**: Manages environment-based configuration
