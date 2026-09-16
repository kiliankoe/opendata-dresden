import {
  type GeoJSONSource,
  Map as MaplibreMap,
  NavigationControl,
  Popup,
  setWorkerUrl,
} from "maplibre-gl";
import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import "maplibre-gl/dist/maplibre-gl.css";
import "./map.css";
import workerUrl from "maplibre-gl/dist/maplibre-gl-worker.mjs?worker&url";
import { type Dataset, resource } from "./datasets";
import { AUTO_BYTES, download, isAbort, TooLargeError } from "./features";
import { Modes, pill } from "./ui";

// MapLibre resolves its worker relative to its own script URL, which bundling
// loses; this hands it the worker as bundled by Vite
setWorkerUrl(workerUrl);

const DRESDEN: [number, number] = [13.74, 51.05];
// Layers can hold over a hundred thousand features, so only the visible
// part of a layer is fetched, capped at this many features
const FEATURE_LIMIT = 2000;

// Basemap and feature colors follow the page's color scheme. The accents
// match the palette in style.css, the outline the basemap's own background.
type Theme = {
  style: string;
  accent: string;
  outline: string;
  water?: string;
};

const LIGHT: Theme = {
  style: "https://tiles.openfreemap.org/styles/positron",
  accent: "#0f766e",
  outline: "#fff",
};

const DARK: Theme = {
  style: "https://tiles.openfreemap.org/styles/dark",
  accent: "#2dd4bf",
  outline: "#0c0c0c",
  // The style draws water nearly in its background color, which loses the
  // Elbe, the one landmark that orients a map of Dresden
  water: "#24343d",
};

const darkScheme = matchMedia("(prefers-color-scheme: dark)");

const watchScheme = (onChange: () => void) => {
  darkScheme.addEventListener("change", onChange);
  return () => darkScheme.removeEventListener("change", onChange);
};

type Mode = "geojson" | "wms";

const MODES: { label: string; value: Mode }[] = [
  { label: "Objekte", value: "geojson" },
  { label: "Kartendienst", value: "wms" },
];

// WmsLayer is one drawable layer of a map service with its legend graphic
type WmsLayer = { name: string; title: string; legend?: string };

export default function DatasetMap({ dataset }: { dataset: Dataset }) {
  const geojsonUrl = resource(dataset, "GEOJSON");
  const wmsUrl = resource(dataset, "WMS");
  const [mode, setMode] = useState<Mode>(geojsonUrl ? "geojson" : "wms");
  const [status, setStatus] = useState("");
  const [tooLarge, setTooLarge] = useState(false);
  const [legend, setLegend] = useState<WmsLayer[]>([]);
  const container = useRef<HTMLDivElement>(null);
  const loadAnyway = useRef<() => void>(() => {});
  const theme = useSyncExternalStore(watchScheme, () => darkScheme.matches)
    ? DARK
    : LIGHT;

  useEffect(() => {
    setStatus("");
    setTooLarge(false);
    setLegend([]);
    if (!container.current) return;
    const map = new MaplibreMap({
      container: container.current,
      style: theme.style,
      center: DRESDEN,
      zoom: 11,
    });
    map.addControl(new NavigationControl(), "top-right");
    const controller = new AbortController();
    map.on("load", () => {
      if (theme.water) {
        map.setPaintProperty("water", "fill-color", theme.water);
        map.setPaintProperty("waterway", "line-color", theme.water);
      }
      if (mode === "geojson" && geojsonUrl) {
        loadAnyway.current = showFeatures(
          map,
          geojsonUrl,
          theme,
          controller.signal,
          setStatus,
          setTooLarge,
        );
      } else if (wmsUrl) {
        showWms(map, wmsUrl, dataset.layerId, controller.signal)
          .then(setLegend)
          .catch(() => setStatus("Kartendienst nicht erreichbar"));
      }
    });
    return () => {
      controller.abort();
      map.remove();
    };
  }, [dataset, mode, geojsonUrl, wmsUrl, theme]);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="mb-1.5 flex items-center gap-3 text-[0.8125rem] text-muted">
        {geojsonUrl && wmsUrl && (
          <Modes options={MODES} active={mode} onSelect={setMode} />
        )}
        <span>{status}</span>
        {tooLarge && (
          <button
            type="button"
            className={pill}
            onClick={() => loadAnyway.current()}
          >
            Trotzdem laden
          </button>
        )}
      </div>
      <div
        ref={container}
        className="min-h-0 flex-1 overflow-hidden rounded-lg"
      />
      {legend.length > 0 && (
        <ul className="mt-1.5 max-h-[30%] shrink-0 overflow-y-auto text-[0.8125rem]">
          {legend.map((layer) => (
            <li key={layer.name} className="flex items-center gap-2">
              <img src={layer.legend} alt="" />
              {legend.length > 1 && layer.title}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

// showFeatures draws the layer's GeoJSON for the current view and reloads it
// whenever the view changes; features show their attributes on click. It
// returns a function that loads the current view without the size cutoff.
function showFeatures(
  map: MaplibreMap,
  url: string,
  theme: Theme,
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
    paint: { "fill-color": theme.accent, "fill-opacity": 0.2 },
  });
  map.addLayer({
    id: "line",
    type: "line",
    source: "data",
    filter: ["in", ["geometry-type"], ["literal", ["Polygon", "LineString"]]],
    paint: { "line-color": theme.accent, "line-width": 2 },
  });
  map.addLayer({
    id: "point",
    type: "circle",
    source: "data",
    filter: ["==", ["geometry-type"], "Point"],
    paint: {
      "circle-radius": 5,
      "circle-color": theme.accent,
      "circle-stroke-color": theme.outline,
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
      } else if (!isAbort(e)) {
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

// showWms overlays the portal's own rendering of the layer and returns the
// drawn layers that have a legend graphic. Datasets with a GeoJSON resource
// name their layer, the others get the service's top-level layer, which draws
// all layers of the service.
async function showWms(
  map: MaplibreMap,
  url: string,
  layerId: string | undefined,
  signal: AbortSignal,
): Promise<WmsLayer[]> {
  const capabilities = await fetchCapabilities(url, signal);
  const layer =
    layerId ?? layerName(capabilities.getElementsByTagName("Layer")[0]);
  const base = new URL(url);
  for (const key of [...base.searchParams.keys()]) {
    if (["service", "request", "version"].includes(key.toLowerCase()))
      base.searchParams.delete(key);
  }
  const params =
    `service=WMS&version=1.3.0&request=GetMap&layers=${encodeURIComponent(layer)}&styles=` +
    "&format=image/png&transparent=true&crs=EPSG:3857&width=256&height=256&bbox={bbox-epsg-3857}";
  const tiles = `${base}${base.search ? "&" : "?"}${params}`;
  if (signal.aborted) return [];
  map.addSource("wms", { type: "raster", tiles: [tiles], tileSize: 256 });
  map.addLayer({
    id: "wms",
    type: "raster",
    source: "wms",
    paint: { "raster-opacity": 0.85 },
  });
  return leafLayers(capabilities).filter(
    (l) => l.legend && (!layerId || l.name === layerId),
  );
}

async function fetchCapabilities(
  url: string,
  signal: AbortSignal,
): Promise<Document> {
  const response = await fetch(url, { signal });
  return new DOMParser().parseFromString(await response.text(), "text/xml");
}

function layerName(layer: Element | undefined): string {
  const name = layer?.getElementsByTagName("Name")[0]?.textContent;
  if (!name) throw new Error("no layer in capabilities");
  return name;
}

// leafLayers lists the layers that draw something. Group layers only serve
// to request their children together and return no usable legend.
function leafLayers(capabilities: Document): WmsLayer[] {
  return [...capabilities.getElementsByTagName("Layer")]
    .filter((layer) => layer.getElementsByTagName("Layer").length === 0)
    .map((layer) => ({
      name: layerName(layer),
      title: layer.getElementsByTagName("Title")[0]?.textContent ?? "",
      legend:
        layer
          .getElementsByTagName("LegendURL")[0]
          ?.getElementsByTagName("OnlineResource")[0]
          ?.getAttributeNS("http://www.w3.org/1999/xlink", "href") ?? undefined,
    }));
}
