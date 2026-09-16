import { Suspense } from "react";
import { DatasetMap, Facts } from "./DatasetPage";
import { type Dataset, resource, sortedResources } from "./datasets";

// Result summarizes a dataset in the list and unfolds its details in place,
// for a quick look at several datasets; the page is for a closer one
export function Result({
  dataset,
  open,
  onToggle,
}: {
  dataset: Dataset;
  open: boolean;
  onToggle: () => void;
}) {
  return (
    <li className="result">
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
          {[...new Set(sortedResources(dataset).map((r) => r.format))].map(
            (format) => (
              <span key={format}>{format}</span>
            ),
          )}
        </p>
      </div>
      {open && (
        <div className="detail">
          <Facts dataset={dataset}>
            <li>
              <a href={`#${dataset.id}`}>Detailseite</a>
            </li>
          </Facts>
          {(resource(dataset, "GEOJSON") || resource(dataset, "WMS")) && (
            <Suspense fallback={<div className="map-canvas" />}>
              <DatasetMap dataset={dataset} />
            </Suspense>
          )}
        </div>
      )}
    </li>
  );
}
