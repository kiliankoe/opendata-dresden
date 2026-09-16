import { execSync } from "node:child_process";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// The index carries no timestamp of its own, so that unchanged rebuilds stay
// byte-identical; its last change is what git recorded
function indexUpdated(): string {
  try {
    return execSync("git log -1 --format=%cs -- ../data/index.json", {
      cwd: import.meta.dirname,
    })
      .toString()
      .trim();
  } catch {
    return "";
  }
}

export default defineConfig({
  plugins: [react()],
  // Relative asset paths work wherever GitHub Pages mounts the site
  base: "./",
  // The dataset index lives outside the web root
  server: { fs: { allow: [".."] } },
  define: { __INDEX_UPDATED__: JSON.stringify(indexUpdated()) },
});
