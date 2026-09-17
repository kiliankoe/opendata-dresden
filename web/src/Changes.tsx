import { useEffect, useState } from "react";
import { isAbort } from "./features";

// REPO_API reads the commits that touched a mirrored table. The portal keeps
// only the current version of each table, so this history is the only record
// of how its numbers were revised.
const REPO_API = "https://api.github.com/repos/kiliankoe/opendata-dresden";

// COUNT bounds both the listing and, since each expanded change costs another
// request, how far the API's 60 requests per hour can be spent here
const COUNT = 20;

interface Change {
  sha: string;
  date: string;
  message: string;
}

async function list(path: string, signal: AbortSignal): Promise<Change[]> {
  const url = `${REPO_API}/commits?path=${encodeURIComponent(path)}&per_page=${COUNT}`;
  const response = await fetch(url, { signal });
  if (!response.ok) throw new Error(`commits: ${response.status}`);
  const commits: {
    sha: string;
    commit: { message: string; committer: { date: string } };
  }[] = await response.json();
  return commits.map((c) => ({
    sha: c.sha,
    date: c.commit.committer.date.split("T")[0],
    message: c.commit.message.split("\n")[0],
  }));
}

async function patch(
  sha: string,
  path: string,
  signal: AbortSignal,
): Promise<string> {
  const response = await fetch(`${REPO_API}/commits/${sha}`, { signal });
  if (!response.ok) throw new Error(`commit: ${response.status}`);
  const commit: { files?: { filename: string; patch?: string }[] } =
    await response.json();
  const file = commit.files?.find((f) => f.filename === path);
  if (!file) return "In dieser Änderung wurde die Tabelle nicht verändert.";
  return file.patch ?? "Diese Änderung ist zu groß für einen Diff.";
}

// Changes lists when a dataset's table changed, newest first, and shows the
// rows of the change the reader opens
export function Changes({ path }: { path: string }) {
  const [changes, setChanges] = useState<Change[]>();
  const [status, setStatus] = useState("Lade …");
  const [open, setOpen] = useState<string>();

  useEffect(() => {
    const controller = new AbortController();
    setChanges(undefined);
    setOpen(undefined);
    setStatus("Lade …");
    list(path, controller.signal)
      .then((found) => {
        setChanges(found);
        setStatus(
          found.length === 0 ? "Noch keine Änderungen aufgezeichnet" : "",
        );
      })
      .catch((e) => {
        if (!isAbort(e)) setStatus("Änderungen konnten nicht geladen werden");
      });
    return () => controller.abort();
  }, [path]);

  return (
    <div className="flex-1 overflow-y-auto text-[0.8125rem]">
      {status && <p className="text-muted">{status}</p>}
      <ul>
        {changes?.map((change) => (
          <li key={change.sha} className="border-b border-line py-1.5">
            <button
              type="button"
              className="cursor-pointer text-left"
              onClick={() =>
                setOpen(open === change.sha ? undefined : change.sha)
              }
            >
              <span className="text-accent">{change.date}</span>{" "}
              <span className="text-muted">{change.message}</span>
            </button>
            {open === change.sha && <Patch sha={change.sha} path={path} />}
          </li>
        ))}
      </ul>
    </div>
  );
}

// Patch shows the rows one change added and removed, as GitHub reports them
function Patch({ sha, path }: { sha: string; path: string }) {
  const [text, setText] = useState("Lade …");

  useEffect(() => {
    const controller = new AbortController();
    setText("Lade …");
    patch(sha, path, controller.signal)
      .then(setText)
      .catch((e) => {
        if (!isAbort(e)) setText("Diff konnte nicht geladen werden");
      });
    return () => controller.abort();
  }, [sha, path]);

  return (
    <pre className="mt-1.5 overflow-x-auto rounded-lg border border-line p-2 font-mono text-xs">
      {text.split("\n").map((line, i) => (
        // Diff lines have no key of their own
        // biome-ignore lint/suspicious/noArrayIndexKey: static list
        <div key={i} className={lineColor(line)}>
          {line}
        </div>
      ))}
    </pre>
  );
}

const lineColor = (line: string) => {
  if (line.startsWith("+")) return "text-added";
  if (line.startsWith("-")) return "text-removed";
  return "text-muted";
};
