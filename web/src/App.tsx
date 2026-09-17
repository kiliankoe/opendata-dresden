import { useEffect, useMemo, useRef, useState } from "react";
import { DatasetPage } from "./DatasetPage";
import {
  byDate,
  createSearch,
  type Dataset,
  type DateField,
  loadDatasets,
} from "./datasets";
import { Result } from "./Result";

const PAGE = 50;

// An empty sort leaves the results in the order the search ranked them
type Sort = "" | DateField;

const germanDate = (iso: string) => iso.split("-").reverse().join(".");

// Naming the open dataset keeps tabs, bookmarks and history entries apart
export function pageTitle(dataset?: Dataset): string {
  return dataset ? `OD3 - ${dataset.title}` : "Dresden Open Data Suche";
}

// Place is where the hash points: the open dataset, the viewer it shows and,
// for the map, which of its maps is drawn
type Place = { openId: string; viewer: string; mapMode: string };

export function parseHash(hash: string): Place {
  const [openId = "", viewer = "", mapMode = ""] = hash
    .replace(/^#/, "")
    .split("/");
  return {
    openId: decodeURIComponent(openId),
    viewer: decodeURIComponent(viewer),
    mapMode: decodeURIComponent(mapMode),
  };
}

// Labels are encoded because some carry a slash of their own, and the mode
// is dropped without a viewer, where it would leave an empty segment
export function formatHash({ openId, viewer, mapMode }: Place): string {
  const path = viewer ? [openId, viewer, mapMode] : [openId];
  const hash = path.filter(Boolean).map(encodeURIComponent).join("/");
  return hash && `#${hash}`;
}

// The query lives in the URL's search part and the place in the hash, so all
// of it can be shared as links. Dataset links are plain hash links, which
// gives them browser history for free.
function readUrl() {
  return {
    query: new URLSearchParams(location.search).get("q") ?? "",
    ...parseHash(location.hash),
  };
}

export default function App() {
  const [datasets, setDatasets] = useState<Dataset[]>();
  const [error, setError] = useState<string>();
  const [query, setQuery] = useState(() => readUrl().query);
  const [topic, setTopic] = useState("");
  const [format, setFormat] = useState("");
  const [sort, setSort] = useState<Sort>("");
  const [openId, setOpenId] = useState(() => readUrl().openId);
  const [viewer, setViewer] = useState(() => readUrl().viewer);
  const [mapMode, setMapMode] = useState(() => readUrl().mapMode);
  const [unfolded, setUnfolded] = useState("");
  const [shown, setShown] = useState(PAGE);
  const input = useRef<HTMLInputElement>(null);

  useEffect(() => {
    loadDatasets().then(setDatasets, (e: Error) => setError(e.message));
  }, []);

  useEffect(() => {
    const onHash = () => {
      const { openId, viewer, mapMode } = readUrl();
      setOpenId(openId);
      setViewer(viewer);
      setMapMode(mapMode);
      window.scrollTo(0, 0);
    };
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  // "/" focuses the search like on many sites
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (
        e.key !== "/" ||
        (e.target as HTMLElement).closest("input, textarea, select")
      )
        return;
      e.preventDefault();
      input.current?.focus();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  useEffect(() => {
    const url = new URL(location.href);
    if (query) url.searchParams.set("q", query);
    else url.searchParams.delete("q");
    url.hash = formatHash({ openId, viewer, mapMode });
    history.replaceState(null, "", url);
  }, [query, openId, viewer, mapMode]);

  // The map mode belongs to the map, so any other viewer leaves it behind
  const changeViewer = (viewer: string, mapMode = "") => {
    setViewer(viewer);
    setMapMode(mapMode);
  };

  // Changing the search or filters starts the list over
  const filter =
    <T,>(set: (value: T) => void) =>
    (value: T) => {
      set(value);
      setShown(PAGE);
    };
  const changeQuery = filter(setQuery);
  const changeTopic = filter(setTopic);
  const changeFormat = filter(setFormat);
  const changeSort = filter(setSort);

  const search = useMemo(() => datasets && createSearch(datasets), [datasets]);
  const byId = useMemo(
    () => new Map(datasets?.map((d) => [d.id, d])),
    [datasets],
  );

  const { topics, formats } = useMemo(() => {
    const topics = new Set<string>();
    const formats = new Map<string, number>();
    for (const dataset of datasets ?? []) {
      for (const t of dataset.topics ?? []) topics.add(t);
      for (const r of dataset.resources)
        formats.set(r.format, (formats.get(r.format) ?? 0) + 1);
    }
    return {
      topics: [...topics].sort(),
      // Rare formats would only clutter the filter
      formats: [...formats]
        .filter(([, n]) => n >= 10)
        .sort((a, b) => b[1] - a[1])
        .map(([f]) => f),
    };
  }, [datasets]);

  const results = useMemo(() => {
    if (!datasets || !search) return [];
    let list = query.trim()
      ? search.search(query).flatMap((hit) => byId.get(hit.id) ?? [])
      : datasets;
    if (topic) list = list.filter((d) => d.topics?.includes(topic));
    if (format)
      list = list.filter((d) => d.resources.some((r) => r.format === format));
    // There is nothing to rank without a query, so that list goes by date too
    const order: Sort = sort || (query.trim() ? "" : "updated");
    return order ? byDate(list, order) : list;
  }, [datasets, search, byId, query, topic, format, sort]);

  const open = openId && datasets && byId.get(openId);
  // Clearing the hash goes through the hashchange handler like the links do,
  // so the page stays in the browser history
  const back = () => {
    location.hash = "";
  };

  useEffect(() => {
    document.title = pageTitle(open || undefined);
  }, [open]);

  const reset = () => {
    changeQuery("");
    setTopic("");
    setFormat("");
    setSort("");
    setOpenId("");
    changeViewer("");
    window.scrollTo(0, 0);
  };

  const select = "max-w-full rounded-md border border-line px-2 py-1 text-sm";

  return (
    // A dataset page needs the width for its viewer and fits the viewport: only
    // the facts column scrolls, so the viewer and the footer stay in place. The
    // search page instead grows with its results and keeps the footer below.
    <div
      className={`mx-auto flex w-full flex-col px-4 pt-8 ${
        open
          ? "min-h-[36rem] max-w-[88rem] pb-5 pane:h-screen"
          : "min-h-screen max-w-[52rem] pb-16"
      }`}
    >
      <header>
        <h1 className="text-[1.75rem] font-bold">
          <a
            href="./"
            onClick={(e) => {
              e.preventDefault();
              reset();
            }}
          >
            Dresden Open Data Suche
          </a>
        </h1>
        {openId && (
          // Above the dataset page rather than in its facts column, which the
          // narrow layout puts below the viewer
          <button
            type="button"
            className="mt-1 mb-3 cursor-pointer text-sm text-accent"
            onClick={back}
          >
            ← Zur Suche
          </button>
        )}
        {!openId && (
          <>
            <div className="text-sm text-muted">
              <p className="mt-1 mb-5 text-sm text-muted">
                Dieses Projekt vereint die Datensätze und ihre Beschreibungen
                aus dem{" "}
                <a
                  className="text-accent underline"
                  href="https://opendata.dresden.de"
                >
                  Open-Data-Portal der Landeshauptstadt Dresden
                </a>{" "}
                mit einer Instant-Suche, damit man in den{" "}
                {datasets?.length ?? ""} Datensätzen schneller den passenden
                findet.
              </p>
            </div>{" "}
            <input
              ref={input}
              type="search"
              // biome-ignore lint/a11y/noAutofocus: searching is all this page is for
              autoFocus
              className="w-full rounded-lg border border-line px-4 py-3 text-lg focus:border-transparent focus:outline-2 focus:outline-accent"
              placeholder="Suche, z. B. Straßenbahn, Bäume, Einwohner …"
              value={query}
              onChange={(e) => changeQuery(e.target.value)}
            />
            <div className="mt-3 mb-6 flex flex-wrap items-center gap-2">
              <select
                className={select}
                value={topic}
                onChange={(e) => changeTopic(e.target.value)}
                aria-label="Thema"
              >
                <option value="">Alle Themen</option>
                {topics.map((t) => (
                  <option key={t}>{t}</option>
                ))}
              </select>
              <select
                className={select}
                value={format}
                onChange={(e) => changeFormat(e.target.value)}
                aria-label="Format"
              >
                <option value="">Alle Formate</option>
                {formats.map((f) => (
                  <option key={f}>{f}</option>
                ))}
              </select>
              <select
                className={select}
                value={sort}
                onChange={(e) => changeSort(e.target.value as Sort)}
                aria-label="Sortierung"
              >
                <option value="">Relevanz</option>
                <option value="updated">Neuester Stand</option>
                <option value="changed">Zuletzt geändert</option>
              </select>
              {datasets && (
                <span className="ml-auto text-sm text-muted">
                  {results.length}{" "}
                  {results.length === 1 ? "Datensatz" : "Datensätze"}
                </span>
              )}
            </div>
          </>
        )}
      </header>

      <main className="flex min-h-0 flex-1 flex-col">
        {error && (
          <p className="text-red-700">
            Der Datensatzindex konnte nicht geladen werden: {error}
          </p>
        )}
        {open && (
          <DatasetPage
            dataset={open}
            viewer={viewer}
            mapMode={mapMode}
            onViewer={changeViewer}
          />
        )}
        {openId && datasets && !open && (
          <p>Diesen Datensatz gibt es nicht mehr.</p>
        )}
        {!openId && (
          <>
            <ul className="border-t border-line">
              {results.slice(0, shown).map((dataset) => (
                <Result
                  key={dataset.id}
                  dataset={dataset}
                  open={dataset.id === unfolded}
                  onToggle={() =>
                    setUnfolded(dataset.id === unfolded ? "" : dataset.id)
                  }
                />
              ))}
            </ul>
            {results.length > shown && (
              <button
                type="button"
                className="mx-auto mt-6 block cursor-pointer rounded-full border border-accent px-5 py-1.5 text-accent"
                onClick={() => setShown(shown + PAGE)}
              >
                Weitere anzeigen
              </button>
            )}
          </>
        )}
      </main>

      <footer
        className={`text-[0.8125rem] text-muted ${open ? "mt-4" : "mt-12"}`}
      >
        <p className="mb-1">
          Dies ist ein privates Projekt ohne Verbindung zur Landeshauptstadt
          Dresden. Die Rechte an den Daten liegen bei den jeweiligen
          Rechteinhabern, die Lizenz steht bei jedem Datensatz.
        </p>
        <p className="mb-1">
          <a
            className="text-accent underline"
            href="https://github.com/kiliankoe/opendata-dresden"
          >
            Quellcode zum Projekt
          </a>
          , auch verfügbar als CLI und MCP-Server. Daten werden nächtlich
          aktualisiert
          {__INDEX_UPDATED__ && `, zuletzt am ${germanDate(__INDEX_UPDATED__)}`}
          . Karten von{" "}
          <a className="text-accent underline" href="https://openfreemap.org">
            OpenFreeMap
          </a>
          .
        </p>
      </footer>
    </div>
  );
}
