import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";

// The build goes to dist/, which the Go package in this folder embeds (embed.go).
// 'npm run dev' proxies the APIs to a Dolly that runs on 8650 (dolly serve).
export default defineConfig({
  plugins: [svelte()],
  base: "./",
  build: { outDir: "dist", emptyOutDir: true },
  server: {
    proxy: {
      "/_dolly": "http://127.0.0.1:8650",
      "/v1.0": "http://127.0.0.1:8650",
    },
  },
});
