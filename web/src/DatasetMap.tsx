import {
  type GeoJSONSource,
  Map as MaplibreMap,
  NavigationControl,
  Popup,
} from "maplibre-gl";
import { useEffect, useRef, useState } from "react";
import "maplibre-gl/dist/maplibre-gl.css";
import { type Dataset, resource } from "./datasets";

const STYLE = "https://tiles.openfreemap.org/styles/positron";
const DRESDEN: [number, number] = [13.74, 51.05];
// Layers can hold over a hundred thousand features, so only the visible
// part of a layer is fetched, capped at this many features
const FEATURE_LIMIT = 2000;
// A few layers consist of enormous polygons, where even a capped request
// runs to tens of megabytes. The server announces no size, so the download
// is cut off past this many bytes unless the user asks for it.
const AUTO_BYTES = 10 * 1024 * 1024;
const COLOR = "#0f766e";

type Mode = "geojson" | "wms";

export default function DatasetMap({ dataset }: { dataset: Dataset }) {
  const geojsonUrl = resource(dataset, "GEOJSON");
  const wmsUrl = resource(dataset, "WMS");
  const [mode, setMode] = useState<Mode>(geojsonUrl ? "geojson" : "wms");
  const [status, setStatus] = useState("");
  const [tooLarge, setTooLarge] = useState(false);
  const container = useRef<HTMLDivElement>(null);
  const loadAnyway = useRef<() => void>(() => {});

  useEffect(() => {
    setStatus("");
    setTooLarge(false);
    if (!container.current) return;
    const map = new MaplibreMap({
      container: container.current,
      style: STYLE,
      center: DRESDEN,
      zoom: 11,
    });
    map.addControl(new NavigationControl(), "top-right");
    const controller = new AbortController();
    map.on("load", () => {
      if (mode === "geojson" && geojsonUrl) {
        loadAnyway.current = showFeatures(
          map,
          geojsonUrl,
          controller.signal,
          setStatus,
          setTooLarge,
        );
      } else if (wmsUrl) {
        showWms(map, wmsUrl, dataset.layerId, controller.signal).catch(() =>
          setStatus("Kartendienst nicht erreichbar"),
        );
      }
    });
    return () => {
      controller.abort();
      map.remove();
    };
  }, [dataset, mode, geojsonUrl, wmsUrl]);

  return (
    <div className="map">
      <div className="map-bar">
        {geojsonUrl && wmsUrl && (
          <span className="modes">
            <button
              type="button"
              className={mode === "geojson" ? "active" : ""}
              onClick={() => setMode("geojson")}
            >
              Objekte
            </button>
            <button
              type="button"
              className={mode === "wms" ? "active" : ""}
              onClick={() => setMode("wms")}
            >
              Kartendienst
            </button>
          </span>
        )}
        <span className="status">{status}</span>
        {tooLarge && (
          <button
            type="button"
            className="load"
            onClick={() => loadAnyway.current()}
          >
            Trotzdem laden
          </button>
        )}
      </div>
      <div ref={container} className="map-canvas" />
    </div>
  );
}

class TooLargeError extends Error {}

// showFeatures draws the layer's GeoJSON for the current view and reloads it
// whenever the view changes; features show their attributes on click. It
// returns a function that loads the current view without the size cutoff.
function showFeatures(
  map: MaplibreMap,
  url: string,
  signal: AbortSignal,
  setStatus: (s: string) => void,
  setTooLarge: (b: boolean) => void,
) {
  map.addSource("data", {
    type: "geojson",
    data: { type: "FeatureCollection", features: [] },
  });
  map.addLayer({
    id: "fill",
    type: "fill",
    source: "data",
    filter: ["==", ["geometry-type"], "Polygon"],
    paint: { "fill-color": COLOR, "fill-opacity": 0.2 },
  });
  map.addLayer({
    id: "line",
    type: "line",
    source: "data",
    filter: ["in", ["geometry-type"], ["literal", ["Polygon", "LineString"]]],
    paint: { "line-color": COLOR, "line-width": 2 },
  });
  map.addLayer({
    id: "point",
    type: "circle",
    source: "data",
    filter: ["==", ["geometry-type"], "Point"],
    paint: {
      "circle-radius": 5,
      "circle-color": COLOR,
      "circle-stroke-color": "#fff",
      "circle-stroke-width": 1.5,
    },
  });

  let inflight: AbortController | undefined;
  let unlimited = false;
  const load = async () => {
    inflight?.abort();
    inflight = new AbortController();
    signal.addEventListener("abort", () => inflight?.abort());
    const bounds = map.getBounds();
    const target = new URL(url);
    target.searchParams.set(
      "bbox",
      bounds
        .toArray()
        .flat()
        .map((n) => n.toFixed(5))
        .join(","),
    );
    target.searchParams.set("limit", String(FEATURE_LIMIT));
    setStatus("Lade …");
    setTooLarge(false);
    try {
      const body = await download(
        target,
        inflight.signal,
        unlimited ? Infinity : AUTO_BYTES,
      );
      const data = JSON.parse(body);
      (map.getSource("data") as GeoJSONSource).setData(data);
      const n = data.features.length;
      setStatus(
        n >= FEATURE_LIMIT
          ? `Die ersten ${n} Objekte im Ausschnitt`
          : `${n} Objekte im Ausschnitt`,
      );
    } catch (e) {
      if (e instanceof TooLargeError) {
        setStatus("Mehr als 10 MB im Ausschnitt");
        setTooLarge(true);
      } else if (!(e instanceof DOMException && e.name === "AbortError")) {
        setStatus("Daten konnten nicht geladen werden");
      }
    }
  };
  map.on("moveend", load);
  void load();

  for (const layer of ["fill", "line", "point"]) {
    map.on(
      "mouseenter",
      layer,
      () => (map.getCanvas().style.cursor = "pointer"),
    );
    map.on("mouseleave", layer, () => (map.getCanvas().style.cursor = ""));
  }
  map.on("click", (e) => {
    const feature = map.queryRenderedFeatures(e.point, {
      layers: ["fill", "line", "point"],
    })[0];
    if (!feature) return;
    const table = document.createElement("table");
    for (const [key, value] of Object.entries(feature.properties)) {
      if (value === null || value === "") continue;
      const row = table.insertRow();
      row.insertCell().textContent = key;
      row.insertCell().textContent = String(value);
    }
    new Popup({ maxWidth: "360px" })
      .setLngLat(e.lngLat)
      .setDOMContent(table)
      .addTo(map);
  });

  return () => {
    unlimited = true;
    void load();
  };
}

// download reads the response as it arrives and gives up once it exceeds
// maxBytes, so an oversized layer costs at most that much traffic
async function download(
  url: URL,
  signal: AbortSignal,
  maxBytes: number,
): Promise<string> {
  const response = await fetch(url, { signal });
  if (!response.body) throw new Error("empty response");
  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let received = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    chunks.push(value);
    received += value.length;
    if (received > maxBytes) {
      await reader.cancel();
      throw new TooLargeError();
    }
  }
  const body = new Uint8Array(received);
  let offset = 0;
  for (const chunk of chunks) {
    body.set(chunk, offset);
    offset += chunk.length;
  }
  return new TextDecoder().decode(body);
}

// showWms overlays the portal's own rendering of the layer. Datasets with a
// GeoJSON resource name their layer, the others need the capabilities document.
async function showWms(
  map: MaplibreMap,
  url: string,
  layerId: string | undefined,
  signal: AbortSignal,
) {
  const layer = layerId ?? (await firstLayer(url, signal));
  const base = new URL(url);
  for (const key of [...base.searchParams.keys()]) {
    if (["service", "request", "version"].includes(key.toLowerCase()))
      base.searchParams.delete(key);
  }
  const params =
    `service=WMS&version=1.3.0&request=GetMap&layers=${encodeURIComponent(layer)}&styles=` +
    "&format=image/png&transparent=true&crs=EPSG:3857&width=256&height=256&bbox={bbox-epsg-3857}";
  const tiles = `${base}${base.search ? "&" : "?"}${params}`;
  if (signal.aborted) return;
  map.addSource("wms", { type: "raster", tiles: [tiles], tileSize: 256 });
  map.addLayer({
    id: "wms",
    type: "raster",
    source: "wms",
    paint: { "raster-opacity": 0.85 },
  });
}

async function firstLayer(
  capabilitiesUrl: string,
  signal: AbortSignal,
): Promise<string> {
  const response = await fetch(capabilitiesUrl, { signal });
  const doc = new DOMParser().parseFromString(
    await response.text(),
    "text/xml",
  );
  const name = doc
    .getElementsByTagName("Layer")[0]
    ?.getElementsByTagName("Name")[0]?.textContent;
  if (!name) throw new Error("no layer in capabilities");
  return name;
}
