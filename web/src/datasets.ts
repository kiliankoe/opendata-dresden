import MiniSearch from "minisearch";
import indexUrl from "../../data/index.json?url";

// Dataset mirrors portal.Dataset of the Go code, see internal/portal/types.go
export interface Dataset {
  id: string;
  title: string;
  updated?: string;
  changed?: string;
  source?: string;
  license?: string;
  topics?: string[];
  regions?: string[];
  years?: string[];
  layerId?: string;
  resources: Resource[];
  description?: string;
  origin?: string;
}

export interface Resource {
  format: string;
  url: string;
}

export async function loadDatasets(): Promise<Dataset[]> {
  const response = await fetch(indexUrl);
  if (!response.ok) throw new Error(`index: ${response.status}`);
  const index: { datasets: Dataset[] } = await response.json();
  return index.datasets;
}

// fold spells out umlauts like the Go search does, so "strasse" finds "Straße"
const fold = (term: string) =>
  term
    .toLowerCase()
    .replace(/ä/g, "ae")
    .replace(/ö/g, "oe")
    .replace(/ü/g, "ue")
    .replace(/ß/g, "ss");

export function createSearch(datasets: Dataset[]) {
  const search = new MiniSearch<Dataset>({
    fields: ["title", "topics", "source", "description", "origin"],
    extractField: (dataset, field) => {
      const value = dataset[field as keyof Dataset];
      return Array.isArray(value) ? value.join(" ") : ((value as string) ?? "");
    },
    processTerm: fold,
    searchOptions: {
      prefix: true,
      fuzzy: 0.2,
      combineWith: "AND",
      boost: { title: 3, topics: 2, source: 2 },
    },
  });
  search.addAll(datasets);
  return search;
}

// resourceOrder puts the formats people can use directly first; the portal
// lists resources in no particular order
const resourceOrder = ["GEOJSON", "CSV", "JSON", "WMS", "WFS", "Tabelle"];

export function sortedResources(dataset: Dataset): Resource[] {
  const rank = (r: Resource) => {
    const i = resourceOrder.indexOf(r.format);
    return i === -1 ? resourceOrder.length : i;
  };
  return [...dataset.resources].sort(
    (a, b) => rank(a) - rank(b) || a.format.localeCompare(b.format),
  );
}

export function resource(dataset: Dataset, format: string): string | undefined {
  return dataset.resources.find((r) => r.format === format)?.url;
}

// Both of a dataset's dates are written dd.mm.yyyy, see portal.Dataset
export type DateField = "updated" | "changed";

export function dateValue(date: string | undefined): number {
  const [day, month, year] = (date ?? "").split(".").map(Number);
  return year ? Date.UTC(year, month - 1, day) : 0;
}

// byDate leads with the newest datasets. Equal dates fall back to the portal's
// date, which is what keeps sorting by the change date readable while few
// datasets have one. The sort is stable, so whatever still ties keeps the
// order it came in, which is its relevance order when the list comes out of a
// search.
export function byDate(datasets: Dataset[], field: DateField): Dataset[] {
  return [...datasets].sort(
    (a, b) =>
      dateValue(b[field]) - dateValue(a[field]) ||
      dateValue(b.updated) - dateValue(a.updated),
  );
}

export function portalUrl(dataset: Dataset): string {
  return `https://opendata.dresden.de/informationsportal/?open=1&result=${dataset.id}#app/mainpage`;
}

// RAW serves the repository's files with CORS headers, which the portal's own
// downloads do not, so this is how the page reads a mirrored table
const RAW =
  "https://raw.githubusercontent.com/kiliankoe/opendata-dresden/main/";

// mirrorPath locates a dataset's mirrored table inside the repository,
// mirroring statsFile of the Go code, see internal/index/mirror.go. Geodata
// layers are not mirrored.
export function mirrorPath(dataset: Dataset): string | undefined {
  const csv = dataset.resources.find((r) => r.format === "CSV");
  const slug = csv?.url.match(
    /\/dcat-ap\/dataset\/([^/]+)\/content\.csv$/,
  )?.[1];
  return slug
    ? `data/statistics/${slug.replace(/^de-sn-dresden-/, "")}.csv`
    : undefined;
}

export function mirrorUrl(dataset: Dataset): string | undefined {
  const path = mirrorPath(dataset);
  return path && RAW + path;
}

// parseCsv splits the portal's semicolon separated tables. Fields are quoted
// only when they contain a semicolon, and none span several lines.
export function parseCsv(text: string): string[][] {
  return text
    .split(/\r?\n/)
    .filter((line) => line.trim() !== "")
    .map((line) => {
      const fields: string[] = [];
      let field = "";
      let quoted = false;
      for (let i = 0; i < line.length; i++) {
        const c = line[i];
        if (c === '"' && line[i + 1] === '"') {
          field += '"';
          i++;
        } else if (c === '"') {
          quoted = !quoted;
        } else if (c === ";" && !quoted) {
          fields.push(field.trim());
          field = "";
        } else {
          field += c;
        }
      }
      fields.push(field.trim());
      return fields;
    });
}
