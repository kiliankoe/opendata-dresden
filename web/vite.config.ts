import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react()],
  // Relative asset paths work wherever GitHub Pages mounts the site
  base: "./",
  // The dataset index lives outside the web root
  server: { fs: { allow: [".."] } },
  // The dependency optimizer loses MapLibre's web worker in dev mode
  optimizeDeps: { exclude: ["maplibre-gl"] },
});
