// A few layers consist of enormous polygons, where even a capped request
// runs to tens of megabytes. The server announces no size, so downloads are
// cut off past this many bytes unless the user asks for more.
export const AUTO_BYTES = 10 * 1024 * 1024;

export class TooLargeError extends Error {}

// narrowed limits a GeoJSON request to a feature count and bounding box in
// the dialect of its host: the portal's OGC API, or an ArcGIS feature
// service, which takes the box as a filter geometry
export function narrowed(url: string, limit: number, bbox?: string): URL {
  const target = new URL(url);
  const arcgis = target.pathname.includes("/FeatureServer/");
  target.searchParams.set(
    arcgis ? "resultRecordCount" : "limit",
    String(limit),
  );
  if (bbox && arcgis) {
    target.searchParams.set("geometry", bbox);
    target.searchParams.set("geometryType", "esriGeometryEnvelope");
    target.searchParams.set("inSR", "4326");
  } else if (bbox) {
    target.searchParams.set("bbox", bbox);
  }
  return target;
}

export const isAbort = (e: unknown) =>
  e instanceof DOMException && e.name === "AbortError";

// download reads the response as it arrives and gives up once it exceeds
// maxBytes, so an oversized layer costs at most that much traffic
export async function download(
  url: URL,
  signal: AbortSignal,
  maxBytes: number,
): Promise<string> {
  const response = await fetch(url, { signal });
  if (!response.body) throw new Error("empty response");
  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let received = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    chunks.push(value);
    received += value.length;
    if (received > maxBytes) {
      await reader.cancel();
      throw new TooLargeError();
    }
  }
  const body = new Uint8Array(received);
  let offset = 0;
  for (const chunk of chunks) {
    body.set(chunk, offset);
    offset += chunk.length;
  }
  return new TextDecoder().decode(body);
}
