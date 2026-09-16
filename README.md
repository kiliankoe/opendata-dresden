# Dresden OpenData CLI and MCP server

A CLI and Model Context Protocol (MCP) server that provides AI agents (or [your toaster](https://worksonmymachine.ai/p/mcp-an-accidentally-universal-plugin)) with access to [Dresden's OpenData Portal](https://opendata.dresden.de), enabling these systems to automatically discover, fetch, and process open civic datasets published by the city or other providers.

I personally find Dresden's OpenData portal extremely useful, but cumbersome to work with and find fitting datasets. This tries to make that a bit easier. [Here](https://claude.ai/share/41f6ee24-1f5d-4e54-9d34-1645ad55b457)'s an example interaction with Claude using this MCP server to find a dataset on Dresden's streets.

## Installation

### Prerequisites

- Go 1.26 or higher, or [Nix](https://nixos.org) with flakes enabled
- An MCP-compatible AI client (e.g. Claude Desktop)

### Building from Source

```bash
git clone https://github.com/kiliankoe/opendata-dresden.git
cd opendata-dresden

make build

make install
```

With Nix, `nix build` produces the binary at `result/bin/od3`, and `nix develop` (or [direnv](https://direnv.net) with `direnv allow`) provides a shell with Go and its tooling.

## Configuration

The server can be configured using environment variables:

```bash
REQUEST_TIMEOUT_MS=30000
INDEX_URL=data/index.json   # a URL or path of the dataset index, see below
```

## MCP Tools

The server provides the following MCP tools:

### `search_datasets`
Search for datasets. Every word of the query has to appear in a dataset's title, topics, source or description; matches in the title rank first, and umlauts may be spelled out ("strasse" finds "Straße"). Use German terms. An empty query lists all datasets. Every result carries its ID and the available resources (CSV, JSON, GeoJSON, WMS, WFS, tables, charts, ...) with their URLs.

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

The same three operations are available as commands, for humans as well as for agents or scripts that prefer running a binary over speaking MCP. Search and info print a readable summary by default and the same JSON as the MCP tools with `--output json`. Fetch prints the resource as is.

```bash
od3 search --limit 5 Straßenbahn
od3 search --output json Straßenbahn
od3 info 0F6996E7-26AB-4585-81BD-1EDA4381B1BC
od3 fetch --limit 50 --bbox 13.72,51.04,13.76,51.07 0F6996E7-26AB-4585-81BD-1EDA4381B1BC GEOJSON
```

Without arguments the binary runs the MCP server on stdio.

## Dataset index

[`data/index.json`](data/index.json) is a snapshot of every dataset in the portal, refreshed nightly by a GitHub Action. It adds the descriptions from the metadata pages of the geodata layers, which the portal's search does not return; the statistics datasets have no description anywhere. Its git history records how the portal's catalog changes over time.

Search and info answer from this index rather than the portal, whose search has no useful ranking. The binary downloads the index on first use and keeps it in the user's cache directory for a day. Datasets not in the index yet are looked up in the portal.

```bash
od3 index   # rebuild data/index.json, fetching details only for new or changed datasets
```

## Usage with Claude Desktop

Add the server to your Claude Desktop configuration:

```js
{
  "mcpServers": {
    "dresden-opendata": {
      "command": "/path/to/od3"
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
- **Index**: Builds the nightly snapshot from the portal client and answers searches from it
- **MCP Tools** and **CLI**: Two thin front ends over the index
- **Configuration**: Manages environment-based configuration
