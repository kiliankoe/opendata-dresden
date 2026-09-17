import { Suspense, useState } from "react";
import type { WmsLayer } from "./DatasetMap";
import { DatasetMap, Facts, Legend } from "./DatasetPage";
import { type Dataset, resource, sortedResources } from "./datasets";
import { Badges, pill } from "./ui";

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
  const [legend, setLegend] = useState<WmsLayer[]>([]);
  // Only a dataset page shares its map mode through the URL, a result in the
  // list keeps its own
  const [mapMode, setMapMode] = useState("");
  return (
    <li className="border-b border-line">
      <div className="group relative py-3">
        <h2 className="text-base font-semibold">
          {/* The ::after covers the summary, so all of it opens the dataset */}
          <a
            className="after:absolute after:inset-0 group-hover:text-accent"
            href={`#${dataset.id}`}
            onClick={(e) => {
              e.preventDefault();
              onToggle();
            }}
          >
            {dataset.title}
          </a>
        </h2>
        <p className="mt-0.5 text-[0.8125rem] text-muted">
          {[
            dataset.source,
            dataset.updated && `Stand ${dataset.updated}`,
            dataset.changed && `Geändert ${dataset.changed}`,
            ...(dataset.topics ?? []),
          ]
            .filter(Boolean)
            .join(" · ")}
        </p>
        <Badges
          items={[...new Set(sortedResources(dataset).map((r) => r.format))]}
        />
      </div>
      {open && (
        <div className="pb-5">
          <Facts dataset={dataset}>
            <li>
              <a className={pill} href={`#${dataset.id}`}>
                Detailseite
              </a>
            </li>
          </Facts>
          {(resource(dataset, "GEOJSON") || resource(dataset, "WMS")) && (
            <>
              <div className="flex h-[26rem] flex-col">
                <Suspense fallback={null}>
                  <DatasetMap
                    dataset={dataset}
                    mode={mapMode}
                    onMode={setMapMode}
                    onLegend={setLegend}
                  />
                </Suspense>
              </div>
              <Legend layers={legend} />
            </>
          )}
        </div>
      )}
    </li>
  );
}
