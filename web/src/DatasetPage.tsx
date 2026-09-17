import { lazy, Suspense, useEffect, useMemo, useState } from "react";
import { Changes } from "./Changes";
import type { WmsLayer } from "./DatasetMap";
import {
  type Dataset,
  mirrorPath,
  mirrorUrl,
  parseCsv,
  portalUrl,
  type Resource,
  resource,
  sortedResources,
} from "./datasets";
import { AUTO_BYTES, download, isAbort, TooLargeError } from "./features";
import { Badges, DatesHelp, Modes, pill } from "./ui";

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
  | { label: "Tabelle"; kind: "stats"; url: string }
  | { label: "Änderungen"; kind: "changes"; path: string }
  | { label: string; kind: "embed"; url: string };

function viewers(dataset: Dataset, resources: Resource[]): Viewer[] {
  const list: Viewer[] = [];
  const geojson = resource(dataset, "GEOJSON");
  if (geojson || resource(dataset, "WMS"))
    list.push({ label: "Karte", kind: "map" });
  if (geojson) list.push({ label: "Attribute", kind: "table", url: geojson });
  // Pushed before the frames so the dedup below drops the portal's own table
  const mirror = mirrorUrl(dataset);
  const path = mirrorPath(dataset);
  if (mirror) list.push({ label: "Tabelle", kind: "stats", url: mirror });
  if (path) list.push({ label: "Änderungen", kind: "changes", path });
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
  mapMode,
  onViewer,
}: {
  dataset: Dataset;
  viewer: string;
  mapMode: string;
  // The map reports its mode alongside its own label, so that both end up in
  // the URL and a shared link opens the map the sender saw
  onViewer: (label: string, mapMode?: string) => void;
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
            <DatasetMap
              dataset={dataset}
              mode={mapMode}
              onMode={(mode) => onViewer(active.label, mode)}
              onLegend={setLegend}
            />
          </Suspense>
        )}
        {active?.kind === "table" && <FeatureTable url={active.url} />}
        {active?.kind === "stats" && <StatsTable url={active.url} />}
        {active?.kind === "changes" && <Changes path={active.path} />}
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

// Facts shows a dataset's description, metadata and resource links
export function Facts({ dataset }: { dataset: Dataset }) {
  const years = dataset.years ?? [];
  // The third element marks the fact whose label carries the explanation of
  // the two dates
  const facts: [string, string | undefined, boolean?][] = [
    ["Quelle", dataset.source],
    ["Stand", dataset.updated],
    ["Geändert", dataset.changed, true],
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
          .map(([label, value, help]) => (
            <div key={label} className="grid grid-cols-[6rem_1fr] gap-2">
              <dt className="flex items-center gap-1 text-muted">
                {label}
                {help && <DatesHelp />}
              </dt>
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
    setStatus("Lade \u2026");
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
  const cells = useMemo(
    () => rows?.map((row) => columns.map((c) => String(row[c] ?? ""))),
    [rows, columns],
  );

  return <DataTable status={status} columns={columns} rows={cells} />;
}

// StatsTable shows a statistics dataset from our mirrored copy. The portal's
// own download allows no cross-origin request, which is why it is mirrored.
function StatsTable({ url }: { url: string }) {
  const [table, setTable] = useState<string[][]>();
  const [status, setStatus] = useState("");

  useEffect(() => {
    const controller = new AbortController();
    setTable(undefined);
    setStatus("Lade \u2026");
    download(new URL(url), controller.signal, AUTO_BYTES)
      .then((body) => {
        const rows = parseCsv(body);
        setTable(rows);
        setStatus(
          rows.length - 1 > TABLE_ROWS
            ? `Die ersten ${TABLE_ROWS} von ${rows.length - 1} Zeilen`
            : `${rows.length - 1} Zeilen`,
        );
      })
      .catch((e) => {
        if (e instanceof TooLargeError)
          setStatus("Mehr als 10 MB, bitte den Download nutzen");
        else if (!isAbort(e)) setStatus("Tabelle konnte nicht geladen werden");
      });
    return () => controller.abort();
  }, [url]);

  return (
    <DataTable
      status={status}
      columns={table?.[0] ?? []}
      rows={table?.slice(1, TABLE_ROWS + 1)}
    />
  );
}

// DataTable renders rows that are already strings, with the header staying
// put while the body scrolls
function DataTable({
  status,
  columns,
  rows,
}: {
  status: string;
  columns: string[];
  rows: string[][] | undefined;
}) {
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
                  {row.map((value, c) => (
                    // Cells line up with the header, whose names are unique
                    <td
                      key={columns[c]}
                      className="border-b border-line px-2.5 py-1 text-left"
                    >
                      {value}
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
