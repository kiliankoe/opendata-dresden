# Dresden OpenData

A CLI, [Model Context Protocol](https://modelcontextprotocol.io) (MCP) server and [search page](https://kiliankoe.github.io/opendata-dresden/) for [Dresden's OpenData portal](https://opendata.dresden.de). The portal publishes a lot of useful civic data, but finding a fitting dataset is cumbersome. This project snapshots the whole catalog nightly and answers searches from that index, for humans in the browser and for AI agents (or [your toaster](https://worksonmymachine.ai/p/mcp-an-accidentally-universal-plugin)) over MCP. [Here](https://claude.ai/share/41f6ee24-1f5d-4e54-9d34-1645ad55b457)'s an example interaction with Claude using the MCP server to find a dataset on Dresden's streets.

## Installation

The CLI and the MCP server are the same binary, `od3`. Pick one of:

```bash
# Homebrew, tapping this repository directly
brew tap kiliankoe/opendata-dresden https://github.com/kiliankoe/opendata-dresden
brew install od3

# Nix, run once or add to your profile
nix run github:kiliankoe/opendata-dresden -- search Straßenbahn
nix profile add github:kiliankoe/opendata-dresden

# From source, with Go 1.26 or newer
git clone https://github.com/kiliankoe/opendata-dresden.git
cd opendata-dresden
make install   # builds od3 into $(go env GOPATH)/bin
```

The flake also exports an overlay that adds `od3` to `pkgs`, for NixOS or Home Manager configurations that take the repository as a flake input: `nixpkgs.overlays = [ opendata-dresden.overlays.default ];`

## Usage

### MCP server

Without arguments `od3` runs the MCP server on stdio. Register it with your client, for example in Claude Desktop's configuration. Use the full path to the binary if the client does not inherit your shell's `PATH`.

```json
{
  "mcpServers": {
    "dresden-opendata": {
      "command": "od3"
    }
  }
}
```

The server offers three tools:

- `search_datasets` searches the index. Every word of the query has to appear in a dataset's title, topics, source or description; matches in the title rank first, and umlauts may be spelled out ("strasse" finds "Straße"). Use German terms. An empty query lists all datasets. Results carry the dataset's ID and its resources (CSV, JSON, GeoJSON, WMS, WFS, tables, charts, ...) with their URLs. `limit` and `offset` paginate.
- `get_dataset_info` returns a dataset with all its resources by ID.
- `fetch_dataset` downloads one resource by dataset ID and format. CSV, JSON and GeoJSON return data, WMS and WFS return the service's capabilities document. Geodata layers return all their features by default. The portal offers no paging, so `limit` and a `bbox` (WGS84 `minLon,minLat,maxLon,maxLat`) are the only ways to narrow a large layer.

You can introspect the server with the [MCP Inspector](https://modelcontextprotocol.io/legacy/tools/inspector).

### CLI

The same three operations are available as commands. Search and info print a readable summary by default and the same JSON as the MCP tools with `--output json`. Fetch prints the resource as is.

```bash
od3 search --limit 5 Straßenbahn
od3 search --output json Straßenbahn
od3 info 0F6996E7-26AB-4585-81BD-1EDA4381B1BC
od3 fetch --limit 50 --bbox 13.72,51.04,13.76,51.07 0F6996E7-26AB-4585-81BD-1EDA4381B1BC GEOJSON
```

Two environment variables apply to both the CLI and the server: `REQUEST_TIMEOUT_MS` (default 30000) and `INDEX_URL`, a URL or path of the dataset index described below.

### Web

[kiliankoe.github.io/opendata-dresden](https://kiliankoe.github.io/opendata-dresden/) searches the index in the browser and shows geodata layers on a map, either as their features for the visible area or as the portal's own map service with its legend. Statistics datasets only link to their downloads and the portal's tables, since their host allows no cross-origin requests.

## Dataset index

[`data/index.json`](data/index.json) is a snapshot of every dataset in the portal, refreshed nightly by a GitHub Action. It adds the descriptions from the metadata pages of the geodata layers, which the portal's search does not return; the statistics datasets have no description anywhere. Its git history records how the portal's catalog changes over time.

Search and info answer from this index rather than the portal, whose search has no useful ranking. The binary downloads the index on first use and keeps it in the user's cache directory for a day. Datasets not in the index yet are looked up in the portal. `od3 index` rebuilds the file, fetching details only for new or changed datasets.

## Development

`nix develop`, or [direnv](https://direnv.net) with `direnv allow`, provides Go and Node tooling.

```bash
make build
make test
make test-integration  # also runs tests against the live portal
make fmt

cd web
pnpm install
pnpm dev
pnpm check   # lint and format check with Biome, `pnpm fix` applies
```

The web app is built from `web/` with Vite and React and deployed by GitHub Pages after every index refresh.

`make release VERSION=x.y.z` bumps the version in the binary and the Homebrew formula, commits and tags. Push with `git push --follow-tags`.
