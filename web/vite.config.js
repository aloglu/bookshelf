import { writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";

// The build goes into the Go program's embed folder. Emptying it removes the placeholder the
// embed needs when nothing is built, so it is written back afterwards.
const outDir = fileURLToPath(new URL("../cmd/bookshelf/webdist", import.meta.url));
const placeholder = {
  name: "webdist-placeholder",
  closeBundle() {
    writeFileSync(`${outDir}/README`, "The built frontend is copied here before compiling; see docs/build-from-source.md.\n");
  },
};

// `npm run dev` serves the frontend and passes the API to a Bookshelf running on port 8080. The
// server only accepts changes from its own address, so the proxy presents them as coming from it.
const server = process.env.BOOKSHELF_DEV_SERVER || "http://localhost:8080";
const proxy = {
  target: server,
  changeOrigin: true,
  configure: (p) => p.on("proxyReq", (req) => { if (req.getHeader("origin")) req.setHeader("origin", server); }),
};

export default defineConfig({
  plugins: [svelte(), placeholder],
  build: { outDir, emptyOutDir: true, assetsInlineLimit: 0 },
  server: { proxy: { "/api": proxy, "/covers": proxy } },
});
