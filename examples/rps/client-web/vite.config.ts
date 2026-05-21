import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";

// The vfx TypeScript SDK lives in this repo rather than on a registry,
// so alias the package name to its source. A published build would
// drop this alias and depend on @averak/vfx-client normally.
export default defineConfig({
  resolve: {
    alias: {
      "@averak/vfx-client": fileURLToPath(
        new URL("../../../sdk/client/ts/src/index.ts", import.meta.url),
      ),
    },
  },
  server: {
    port: 5173,
  },
});
