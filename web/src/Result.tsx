import { lazy, Suspense } from "react";
import { type Dataset, portalUrl, resource, sortedResources } from "./datasets";

// The map library is by far the largest dependency and only geodata needs it
const DatasetMap = lazy(() => import("./DatasetMap"));

export function Result({
  dataset,
  open,
  onToggle,
}: {
  dataset: Dataset;
  open: boolean;
  onToggle: () => void;
}) {
  const resources = sortedResources(dataset);

  return (
    <li id={dataset.id} className={open ? "result open" : "result"}>
      <div className="summary">
        <h2>
          <a
            className="title"
            href={`#${dataset.id}`}
            onClick={(e) => {
              e.preventDefault();
              onToggle();
            }}
          >
            {dataset.title}
          </a>
        </h2>
        <p className="meta">
          {[
            dataset.source,
            dataset.updated && `Stand ${dataset.updated}`,
            ...(dataset.topics ?? []),
          ]
            .filter(Boolean)
            .join(" · ")}
        </p>
        <p className="formats">
          {[...new Set(resources.map((r) => r.format))].map((format) => (
            <span key={format}>{format}</span>
          ))}
        </p>
      </div>
      {open && <Detail dataset={dataset} resources={resources} />}
    </li>
  );
}

function Detail({
  dataset,
  resources,
}: {
  dataset: Dataset;
  resources: Dataset["resources"];
}) {
  const years = dataset.years ?? [];
  const facts: [string, string | undefined][] = [
    ["Quelle", dataset.source],
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
    <div className="detail">
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
        {resources.map((r) => (
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
      </ul>
      {(resource(dataset, "GEOJSON") || resource(dataset, "WMS")) && (
        <Suspense fallback={<div className="map-canvas" />}>
          <DatasetMap dataset={dataset} />
        </Suspense>
      )}
    </div>
  );
}
