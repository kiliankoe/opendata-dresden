import MiniSearch from 'minisearch'
import indexUrl from '../../data/index.json?url'

// Dataset mirrors portal.Dataset of the Go code, see internal/portal/types.go
export interface Dataset {
  id: string
  title: string
  updated?: string
  source?: string
  license?: string
  topics?: string[]
  regions?: string[]
  years?: string[]
  layerId?: string
  resources: Resource[]
  description?: string
  origin?: string
}

export interface Resource {
  format: string
  url: string
}

export async function loadDatasets(): Promise<Dataset[]> {
  const response = await fetch(indexUrl)
  if (!response.ok) throw new Error(`index: ${response.status}`)
  const index: { datasets: Dataset[] } = await response.json()
  return index.datasets
}

// fold spells out umlauts like the Go search does, so "strasse" finds "Straße"
const fold = (term: string) =>
  term.toLowerCase().replace(/ä/g, 'ae').replace(/ö/g, 'oe').replace(/ü/g, 'ue').replace(/ß/g, 'ss')

export function createSearch(datasets: Dataset[]) {
  const search = new MiniSearch<Dataset>({
    fields: ['title', 'topics', 'source', 'description', 'origin'],
    extractField: (dataset, field) => {
      const value = dataset[field as keyof Dataset]
      return Array.isArray(value) ? value.join(' ') : ((value as string) ?? '')
    },
    processTerm: fold,
    searchOptions: {
      prefix: true,
      fuzzy: 0.2,
      combineWith: 'AND',
      boost: { title: 3, topics: 2, source: 2 },
    },
  })
  search.addAll(datasets)
  return search
}

// resourceOrder puts the formats people can use directly first; the portal
// lists resources in no particular order
const resourceOrder = ['GEOJSON', 'CSV', 'JSON', 'WMS', 'WFS', 'Tabelle']

export function sortedResources(dataset: Dataset): Resource[] {
  const rank = (r: Resource) => {
    const i = resourceOrder.indexOf(r.format)
    return i === -1 ? resourceOrder.length : i
  }
  return [...dataset.resources].sort((a, b) => rank(a) - rank(b) || a.format.localeCompare(b.format))
}

export function resource(dataset: Dataset, format: string): string | undefined {
  return dataset.resources.find((r) => r.format === format)?.url
}

// The portal dates datasets as dd.mm.yyyy
export function updatedTime(dataset: Dataset): number {
  const [day, month, year] = (dataset.updated ?? '').split('.').map(Number)
  return year ? Date.UTC(year, month - 1, day) : 0
}

export function portalUrl(dataset: Dataset): string {
  return `https://opendata.dresden.de/informationsportal/?open=1&result=${dataset.id}#app/mainpage`
}
