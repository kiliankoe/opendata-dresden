// A few layers consist of enormous polygons, where even a capped request
// runs to tens of megabytes. The server announces no size, so downloads are
// cut off past this many bytes unless the user asks for more.
export const AUTO_BYTES = 10 * 1024 * 1024;

export class TooLargeError extends Error {}

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
