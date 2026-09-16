import { useEffect, useMemo, useRef, useState } from "react";
import {
  createSearch,
  type Dataset,
  loadDatasets,
  updatedTime,
} from "./datasets";
import { Result } from "./Result";

const PAGE = 50;

// The query lives in the URL's search part and the open dataset in its hash,
// so both can be shared as links
function readUrl() {
  return {
    query: new URLSearchParams(location.search).get("q") ?? "",
    openId: decodeURIComponent(location.hash.slice(1)),
  };
}

export default function App() {
  const [datasets, setDatasets] = useState<Dataset[]>();
  const [error, setError] = useState<string>();
  const [query, setQuery] = useState(() => readUrl().query);
  const [topic, setTopic] = useState("");
  const [format, setFormat] = useState("");
  const [openId, setOpenId] = useState(() => readUrl().openId);
  const [shown, setShown] = useState(PAGE);
  const linkedId = useRef(readUrl().openId);

  useEffect(() => {
    loadDatasets().then(setDatasets, (e: Error) => setError(e.message));
  }, []);

  useEffect(() => {
    const url = new URL(location.href);
    if (query) url.searchParams.set("q", query);
    else url.searchParams.delete("q");
    url.hash = openId;
    history.replaceState(null, "", url);
  }, [query, openId]);

  // Bring a dataset opened from a shared link into view once the list exists
  useEffect(() => {
    if (datasets && linkedId.current) {
      document.getElementById(linkedId.current)?.scrollIntoView();
    }
  }, [datasets]);

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

  // A shared link opens its dataset even when it is not among the results
  const linked = openId && byId.get(openId);
  const listed = results.slice(0, shown);
  const visible =
    linked && !listed.includes(linked) ? [linked, ...listed] : listed;

  return (
    <>
      <header>
        <h1>Dresden Open Data</h1>
        <p>
          Alle {datasets?.length ?? ""} Datensätze des{" "}
          <a href="https://opendata.dresden.de">
            Open-Data-Portals der Landeshauptstadt Dresden
          </a>{" "}
          durchsuchen, mit Karte für Geodaten.
        </p>
        <input
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
      </header>

      <main>
        {error && (
          <p className="error">
            Der Datensatzindex konnte nicht geladen werden: {error}
          </p>
        )}
        <ul className="results">
          {visible.map((dataset) => (
            <Result
              key={dataset.id}
              dataset={dataset}
              open={dataset.id === openId}
              onToggle={() =>
                setOpenId(dataset.id === openId ? "" : dataset.id)
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
      </main>

      <footer>
        <a href="https://github.com/kiliankoe/opendata-dresden">Quellcode</a>,
        auch als CLI und MCP-Server. Daten werden nächtlich aktualisiert. Karten
        von <a href="https://openfreemap.org">OpenFreeMap</a>.
      </footer>
    </>
  );
}
