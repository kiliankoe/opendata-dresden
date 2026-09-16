import {
  lazy,
  type ReactNode,
  Suspense,
  useEffect,
  useMemo,
  useState,
} from "react";
import {
  type Dataset,
  portalUrl,
  type Resource,
  resource,
  sortedResources,
} from "./datasets";
import {
  AUTO_BYTES,
  download,
  isAbort,
  narrowed,
  TooLargeError,
} from "./features";

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
  return (
    <article className="dataset">
      <div className="about">
        <button type="button" className="back" onClick={onBack}>
          ← Zur Suche
        </button>
        <h2>{dataset.title}</h2>
        {dataset.topics && (
          <p className="formats">
            {dataset.topics.map((t) => (
              <span key={t}>{t}</span>
            ))}
          </p>
        )}
        <Facts dataset={dataset} />
      </div>
      <div className="viewer">
        {options.length > 1 && (
          <div className="modes picker">
            {options.map((v) => (
              <button
                key={v.label}
                type="button"
                className={v === active ? "active" : ""}
                onClick={() => onViewer(v.label)}
              >
                {v.label}
              </button>
            ))}
          </div>
        )}
        {!active && (
          <p className="bar">Für diesen Datensatz gibt es nur Downloads.</p>
        )}
        {active?.kind === "map" && (
          <Suspense fallback={<div className="map" />}>
            <DatasetMap dataset={dataset} />
          </Suspense>
        )}
        {active?.kind === "table" && <FeatureTable url={active.url} />}
        {active?.kind === "embed" && (
          <iframe key={active.url} src={active.url} title={active.label} />
        )}
      </div>
    </article>
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
    ["Portal", dataset.portal],
  ];

  return (
    <div className="facts">
      {dataset.description && (
        <p className="description">{dataset.description}</p>
      )}
      <dl>
        {facts
          .filter(([, value]) => value)
          .map(([label, value]) => (
            <div key={label}>
              <dt>{label}</dt>
              <dd>{value}</dd>
            </div>
          ))}
      </dl>
      <ul className="links">
        {sortedResources(dataset).map((r) => (
          <li key={r.url}>
            <a href={r.url} target="_blank" rel="noreferrer">
              {r.format}
            </a>
          </li>
        ))}
        <li>
          <a href={portalUrl(dataset)} target="_blank" rel="noreferrer">
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
    const target = narrowed(url, TABLE_ROWS);
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
      <p className="bar">{status}</p>
      {rows && (
        <div className="table">
          <table>
            <thead>
              <tr>
                {columns.map((c) => (
                  <th key={c}>{c}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((row, i) => (
                // Rows have no reliable key of their own
                // biome-ignore lint/suspicious/noArrayIndexKey: static list
                <tr key={i}>
                  {columns.map((c) => (
                    <td key={c}>{String(row[c] ?? "")}</td>
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
