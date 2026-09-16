import {
  lazy,
  type ReactNode,
  Suspense,
  useEffect,
  useMemo,
  useState,
} from "react";
import type { WmsLayer } from "./DatasetMap";
import {
  type Dataset,
  portalUrl,
  type Resource,
  resource,
  sortedResources,
} from "./datasets";
import { AUTO_BYTES, download, isAbort, TooLargeError } from "./features";
import { Badges, Modes, pill } from "./ui";

// The map library is by far the largest dependency and only geodata needs it
export const DatasetMap = lazy(() => import("./DatasetMap"));

// The attribute table shows the layer from the start rather than for a map
// view, so it needs a limit of its own; the portal offers no paging
const TABLE_ROWS = 500;

// Resources the portal serves as pages that allow embedding. Its downloads
// and the statistics datasets' information pages forbid it, as does the
// statistics host cross-origin requests, so those are only linked.
const EMBEDDABLE = new Set([
  "Tabelle",
  "Säulen-/Balkendiagramm",
  "Liniendiagramm",
  "Kreisdiagramm",
  "Pyramidendiagramm",
  "Themenstadtplan",
  "Bundestagswahlatlas",
  "Kommunalwahlatlas",
  "Landtagswahlatlas",
  "HTML-Bericht",
  "pdf-Datei",
  "Legende (PDF)",
  "Dashboard",
]);

const embeddable = (r: Resource) =>
  r.format === "Information"
    ? /service=ikx/i.test(r.url)
    : EMBEDDABLE.has(r.format);

type Viewer =
  | { label: "Karte"; kind: "map" }
  | { label: "Attribute"; kind: "table"; url: string }
  | { label: string; kind: "embed"; url: string };

function viewers(dataset: Dataset, resources: Resource[]): Viewer[] {
  const list: Viewer[] = [];
  const geojson = resource(dataset, "GEOJSON");
  if (geojson || resource(dataset, "WMS"))
    list.push({ label: "Karte", kind: "map" });
  if (geojson) list.push({ label: "Attribute", kind: "table", url: geojson });
  const embeds = resources
    .filter(embeddable)
    .map((r) => ({ label: r.format, kind: "embed" as const, url: r.url }));
  // The metadata page repeats what the left column shows, so it comes last
  list.push(
    ...embeds.filter((v) => v.label !== "Information"),
    ...embeds.filter((v) => v.label === "Information"),
  );
  return list.filter(
    (v, i) => list.findIndex((o) => o.label === v.label) === i,
  );
}

export function DatasetPage({
  dataset,
  viewer,
  onViewer,
  onBack,
}: {
  dataset: Dataset;
  viewer: string;
  onViewer: (label: string) => void;
  onBack: () => void;
}) {
  const resources = sortedResources(dataset);
  const options = viewers(dataset, resources);
  const active = options.find((v) => v.label === viewer) ?? options[0];
  // The map reports the legend of its map service; it belongs with the facts
  // rather than below the map, where it would eat into the map's height
  const [legend, setLegend] = useState<WmsLayer[]>([]);
  return (
    <article className="grid min-h-0 flex-1 grid-cols-1 grid-rows-[minmax(0,1fr)] gap-8 pane:grid-cols-[minmax(0,22rem)_minmax(0,1fr)]">
      <div className="min-h-0 pane:overflow-y-auto pane:pr-2">
        <button
          type="button"
          className="mb-3 inline-block cursor-pointer text-sm text-accent"
          onClick={onBack}
        >
          ← Zur Suche
        </button>
        <h2 className="mb-1 text-xl font-bold">{dataset.title}</h2>
        {dataset.topics && <Badges items={dataset.topics} />}
        <Legend layers={legend} />
        <Facts dataset={dataset} />
      </div>
      {/* Descriptions can be long, so on narrow screens the viewer comes first
          and the page scrolls as a whole */}
      <div className="-order-1 flex h-[60vh] min-h-[20rem] flex-col pane:order-none pane:h-auto pane:min-h-0">
        {options.length > 1 && (
          <Modes
            className="mb-2"
            options={options.map((v) => ({ label: v.label, value: v.label }))}
            active={active?.label}
            onSelect={onViewer}
          />
        )}
        {!active && (
          <p className="text-[0.8125rem] text-muted">
            Für diesen Datensatz gibt es nur Downloads.
          </p>
        )}
        {active?.kind === "map" && (
          <Suspense fallback={<div className="flex-1" />}>
            {/* The map picks its mode from what the dataset offers, so it has
                to start over rather than carry the last dataset's choice */}
            <DatasetMap
              key={dataset.id}
              dataset={dataset}
              onLegend={setLegend}
            />
          </Suspense>
        )}
        {active?.kind === "table" && <FeatureTable url={active.url} />}
        {active?.kind === "embed" && (
          <iframe
            key={active.url}
            className="w-full flex-1 rounded-lg border border-line bg-white"
            src={active.url}
            title={active.label}
          />
        )}
      </div>
    </article>
  );
}

// Legend shows what the map service draws. Its graphics label themselves,
// so layer titles only help when there is more than one.
export function Legend({ layers }: { layers: WmsLayer[] }) {
  if (layers.length === 0) return null;
  return (
    <>
      <h3 className="mt-4 mb-1 text-sm font-semibold">Legende</h3>
      <ul className="text-[0.8125rem]">
        {layers.map((layer) => (
          <li key={layer.name} className="flex items-center gap-2">
            <img src={layer.legend} alt="" />
            {layers.length > 1 && layer.title}
          </li>
        ))}
      </ul>
    </>
  );
}

// Facts shows a dataset's description, metadata and resource links; children
// are added to the links
export function Facts({
  dataset,
  children,
}: {
  dataset: Dataset;
  children?: ReactNode;
}) {
  const years = dataset.years ?? [];
  const facts: [string, string | undefined][] = [
    ["Quelle", dataset.source],
    ["Stand", dataset.updated],
    ["Lizenz", dataset.license],
    [
      "Zeitraum",
      years.length > 1
        ? `${years[0]} bis ${years[years.length - 1]}`
        : years[0],
    ],
    ["Raumbezug", dataset.regions?.join(", ")],
    ["Herkunft", dataset.origin],
  ];

  return (
    <div>
      {dataset.description && (
        <p className="my-3 whitespace-pre-line">{dataset.description}</p>
      )}
      <dl className="mb-3 text-sm">
        {facts
          .filter(([, value]) => value)
          .map(([label, value]) => (
            <div key={label} className="grid grid-cols-[6rem_1fr] gap-2">
              <dt className="text-muted">{label}</dt>
              <dd>{value}</dd>
            </div>
          ))}
      </dl>
      <ul className="mb-4 flex flex-wrap gap-1.5 text-sm">
        {sortedResources(dataset).map((r) => (
          <li key={r.url}>
            <a className={pill} href={r.url} target="_blank" rel="noreferrer">
              {r.format}
            </a>
          </li>
        ))}
        <li>
          <a
            className={pill}
            href={portalUrl(dataset)}
            target="_blank"
            rel="noreferrer"
          >
            Im Portal öffnen
          </a>
        </li>
        {children}
      </ul>
    </div>
  );
}

type Row = Record<string, unknown>;

// FeatureTable lists the attributes of the first rows of a geodata layer
function FeatureTable({ url }: { url: string }) {
  const [rows, setRows] = useState<Row[]>();
  const [status, setStatus] = useState("");

  useEffect(() => {
    const controller = new AbortController();
    const target = new URL(url);
    target.searchParams.set("limit", String(TABLE_ROWS));
    setRows(undefined);
    setStatus("Lade …");
    download(target, controller.signal, AUTO_BYTES)
      .then((body) => {
        const features: { properties: Row }[] = JSON.parse(body).features;
        setRows(features.map((f) => f.properties));
        setStatus(
          features.length >= TABLE_ROWS
            ? `Die ersten ${features.length} Objekte`
            : `${features.length} Objekte`,
        );
      })
      .catch((e) => {
        if (e instanceof TooLargeError)
          setStatus("Mehr als 10 MB, bitte den Download nutzen");
        else if (!isAbort(e)) setStatus("Daten konnten nicht geladen werden");
      });
    return () => controller.abort();
  }, [url]);

  const columns = useMemo(
    () => [...new Set(rows?.flatMap(Object.keys))],
    [rows],
  );

  return (
    <>
      <p className="mb-1.5 text-[0.8125rem] text-muted">{status}</p>
      {rows && (
        <div className="flex-1 overflow-auto rounded-lg border border-line text-[0.8125rem]">
          <table className="whitespace-nowrap">
            <thead>
              <tr>
                {columns.map((c) => (
                  <th
                    key={c}
                    className="sticky top-0 border-b border-line bg-surface px-2.5 py-1 text-left font-semibold"
                  >
                    {c}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((row, i) => (
                // Rows have no reliable key of their own
                // biome-ignore lint/suspicious/noArrayIndexKey: static list
                <tr key={i}>
                  {columns.map((c) => (
                    <td
                      key={c}
                      className="border-b border-line px-2.5 py-1 text-left"
                    >
                      {String(row[c] ?? "")}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}
