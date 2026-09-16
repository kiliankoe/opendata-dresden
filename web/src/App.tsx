import { useEffect, useMemo, useRef, useState } from "react";
import { DatasetPage } from "./DatasetPage";
import {
  createSearch,
  type Dataset,
  loadDatasets,
  updatedTime,
} from "./datasets";
import { Result } from "./Result";

const PAGE = 50;

const germanDate = (iso: string) => iso.split("-").reverse().join(".");

// The query lives in the URL's search part and the open dataset with its
// chosen viewer in the hash, so all of it can be shared as links. Dataset
// links are plain hash links, which gives them browser history for free.
function readUrl() {
  const [openId, ...viewer] = location.hash.slice(1).split("/");
  return {
    query: new URLSearchParams(location.search).get("q") ?? "",
    openId: decodeURIComponent(openId),
    viewer: decodeURIComponent(viewer.join("/")),
  };
}

export default function App() {
  const [datasets, setDatasets] = useState<Dataset[]>();
  const [error, setError] = useState<string>();
  const [query, setQuery] = useState(() => readUrl().query);
  const [topic, setTopic] = useState("");
  const [format, setFormat] = useState("");
  const [openId, setOpenId] = useState(() => readUrl().openId);
  const [viewer, setViewer] = useState(() => readUrl().viewer);
  const [unfolded, setUnfolded] = useState("");
  const [shown, setShown] = useState(PAGE);
  const input = useRef<HTMLInputElement>(null);

  useEffect(() => {
    loadDatasets().then(setDatasets, (e: Error) => setError(e.message));
  }, []);

  useEffect(() => {
    const onHash = () => {
      const { openId, viewer } = readUrl();
      setOpenId(openId);
      setViewer(viewer);
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
    url.hash = viewer ? `${openId}/${encodeURIComponent(viewer)}` : openId;
    history.replaceState(null, "", url);
  }, [query, openId, viewer]);

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
      : [...datasets].sort((a, b) => updatedTime(b) - updatedTime(a));
    if (topic) list = list.filter((d) => d.topics?.includes(topic));
    if (format)
      list = list.filter((d) => d.resources.some((r) => r.format === format));
    return list;
  }, [datasets, search, byId, query, topic, format]);

  const open = openId && datasets && byId.get(openId);
  // Clearing the hash goes through the hashchange handler like the links do,
  // so the page stays in the browser history
  const back = () => {
    location.hash = "";
  };

  const reset = () => {
    changeQuery("");
    setTopic("");
    setFormat("");
    setOpenId("");
    setViewer("");
    window.scrollTo(0, 0);
  };

  return (
    <>
      <header>
        <h1>
          <a
            href="./"
            onClick={(e) => {
              e.preventDefault();
              reset();
            }}
          >
            Dresden Open Data
          </a>
        </h1>
        {!openId && (
          <>
            <p>
              Durchsuche alle {datasets?.length ?? ""} Datensätze des{" "}
              <a href="https://opendata.dresden.de">
                Open-Data-Portals der Landeshauptstadt Dresden
              </a>
              .
            </p>
            <input
              ref={input}
              type="search"
              // biome-ignore lint/a11y/noAutofocus: searching is all this page is for
              autoFocus
              placeholder="Suche, z. B. Straßenbahn, Bäume, Einwohner …"
              value={query}
              onChange={(e) => changeQuery(e.target.value)}
            />
            <div className="filters">
              <select
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
                value={format}
                onChange={(e) => changeFormat(e.target.value)}
                aria-label="Format"
              >
                <option value="">Alle Formate</option>
                {formats.map((f) => (
                  <option key={f}>{f}</option>
                ))}
              </select>
              {datasets && (
                <span className="count">
                  {results.length}{" "}
                  {results.length === 1 ? "Datensatz" : "Datensätze"}
                </span>
              )}
            </div>
          </>
        )}
      </header>

      <main>
        {error && (
          <p className="error">
            Der Datensatzindex konnte nicht geladen werden: {error}
          </p>
        )}
        {open && (
          <DatasetPage
            dataset={open}
            viewer={viewer}
            onViewer={setViewer}
            onBack={back}
          />
        )}
        {openId && datasets && !open && (
          <p>
            Diesen Datensatz gibt es nicht mehr.{" "}
            <button type="button" className="back" onClick={back}>
              Zur Suche
            </button>
          </p>
        )}
        {!openId && (
          <>
            <ul className="results">
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
                className="more"
                onClick={() => setShown(shown + PAGE)}
              >
                Weitere anzeigen
              </button>
            )}
          </>
        )}
      </main>

      <footer>
        <a href="https://github.com/kiliankoe/opendata-dresden">Quellcode</a>,
        auch als CLI und MCP-Server. Daten werden nächtlich aktualisiert
        {__INDEX_UPDATED__ && `, zuletzt am ${germanDate(__INDEX_UPDATED__)}`}.
        Karten von <a href="https://openfreemap.org">OpenFreeMap</a>.
      </footer>
    </>
  );
}
