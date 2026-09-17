# Dresden OpenData

A CLI, [Model Context Protocol](https://modelcontextprotocol.io) (MCP) server and [search page](https://kiliankoe.github.io/opendata-dresden/) for [Dresden's OpenData portal](https://opendata.dresden.de). The portal publishes a lot of useful civic data, but finding a fitting dataset is cumbersome. This project snapshots the whole catalog nightly and answers searches from that index, for humans in the browser and for AI agents (or [your toaster](https://worksonmymachine.ai/p/mcp-an-accidentally-universal-plugin)) over MCP. [Here](https://claude.ai/share/41f6ee24-1f5d-4e54-9d34-1645ad55b457)'s an example interaction with Claude using the MCP server to find a dataset on Dresden's streets.

This is a private project and not affiliated with the City of Dresden. The rights to the data stay with their respective holders; each dataset carries its license.

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

The server offers four tools:

- `search_datasets` searches the index. Every word of the query has to appear in a dataset's title, topics, source or description; matches in the title rank first, and umlauts may be spelled out ("strasse" finds "Straße"). Use German terms. An empty query lists all datasets. Results carry the dataset's ID, its two dates and its resources (CSV, JSON, GeoJSON, WMS, WFS, tables, charts, ...) with their URLs. `limit` and `offset` paginate.
- `get_dataset_info` returns a dataset with all its resources by ID.
- `fetch_dataset` downloads one resource by dataset ID and format. CSV, JSON and GeoJSON return data, WMS and WFS return the service's capabilities document. Geodata layers return all their features by default. The portal offers no paging, so `limit` and a `bbox` (WGS84 `minLon,minLat,maxLon,maxLat`) are the only ways to narrow a large layer.
- `dataset_history` reports how a statistics dataset's numbers changed over time, newest first, from the mirrored tables described below. `diff` adds the rows that moved. Geodata layers are not mirrored.

You can introspect the server with the [MCP Inspector](https://modelcontextprotocol.io/legacy/tools/inspector).

### CLI

The same operations are available as commands. Search and info print a readable summary by default and the same JSON as the MCP tools with `--output json`. Fetch prints the resource as is.

```bash
od3 search --limit 5 Straßenbahn
od3 search --output json Straßenbahn
od3 info 0F6996E7-26AB-4585-81BD-1EDA4381B1BC
od3 fetch --limit 50 --bbox 13.72,51.04,13.76,51.07 0F6996E7-26AB-4585-81BD-1EDA4381B1BC GEOJSON
od3 history --diff 001CCBCE-C798-4445-8680-9C8BD1FEC2DC
```

These environment variables apply to both the CLI and the server: `REQUEST_TIMEOUT_MS` (default 30000), `INDEX_URL`, a URL or path of the dataset index described below, `REPO_API` for the repository whose history `history` reads, and `GITHUB_TOKEN` to raise that API's rate limit.

### Web

[kiliankoe.github.io/opendata-dresden](https://kiliankoe.github.io/opendata-dresden/) searches the index in the browser. Each dataset has a page with its metadata next to a viewer for its resources: geodata layers as features on a map for the visible area, as the portal's own map service with its legend, or as an attribute table; the portal's charts, thematic maps, reports and PDFs embedded as they are. Statistics datasets show their mirrored table and the changes recorded for it. Downloads are only linked, since the portal allows no cross-origin requests.

## Dataset index

[`data/index.json`](data/index.json) is a snapshot of every dataset in the portal, refreshed nightly by a GitHub Action. It adds the descriptions from the metadata pages of the geodata layers, which the portal's search does not return; the statistics datasets have no description anywhere. Its git history records how the portal's catalog changes over time.

Search and info answer from this index rather than the portal, whose search has no useful ranking. The binary downloads the index on first use and keeps it in the user's cache directory for a day. Datasets not in the index yet are looked up in the portal. `od3 index` rebuilds the file, fetching details only for new or changed datasets.

## Mirrored statistics tables

[`data/statistics/`](data/statistics) holds the CSV download of every statistics dataset, copied byte for byte and refreshed by the same Action with `od3 index --data data/statistics`. The portal publishes only the current version of each table, so this git history is the only record of how the numbers change. The whole set is about 39 MB and roughly 8 of the 323 tables change in a month.

A table is downloaded again when its update date moves or its file is missing, and removed when the portal stops listing the dataset. Geodata layers stay out: their attribute tables come to roughly 1.3 GB per snapshot.

Comparing a fresh download against the mirrored copy also gives every dataset a second date. `updated` is the portal's own, which moves whenever the city republishes a dataset, whether or not the numbers moved with it. `changed` is the day the rows last actually differed, and stays empty until this project has seen that happen. Both are written dd.mm.yyyy and both are in [`data/index.json`](data/index.json), so the CLI, the MCP tools and the search page can sort and show them. This is why `od3 index --data` mirrors the tables before it writes the index.

`od3 history <id>` and the `dataset_history` tool report what that history recorded, newest first, reading the commits through GitHub's API. `--diff` adds the rows that moved, at the cost of one request per change. Unauthenticated that API allows 60 requests per hour; set `GITHUB_TOKEN` to raise it. The web app reads the same API for its "Änderungen" view and the tables from `raw.githubusercontent.com`, which sends the CORS headers the portal does not.

Fetching a dataset always goes to the portal, so you get the city's current numbers and never the mirror's copy of them. The portal's own charts stay as they are: the tables mix counts with rates, averages and cumulative figures, so a chart built without knowing which is which would show wrong numbers.

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
pnpm test    # unit tests with Vitest
pnpm check   # lint and format check with Biome, `pnpm fix` applies
```

The web app is built from `web/` with Vite, React and Tailwind and deployed by GitHub Pages after every index refresh.

`make release VERSION=x.y.z` bumps the version in the binary and the Homebrew formula, commits and tags. Push with `git push --follow-tags`.
