import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import wails from "@wailsio/runtime/plugins/vite";
import tailwindcss from "@tailwindcss/vite";
import icons from "unplugin-icons/vite";

// Per-node-type documentation lives with the Go schema that validates it
// (cmd/desktop/internal/app/flow/docs/<type>.md) because the prompts service
// renders the same markdown into the flows authoring prompt. The node drawer
// and palette import those files directly through this alias rather than
// keeping a second copy under nodes/*/help.md — one file, two readers.
const nodeDocs = fileURLToPath(new URL("../internal/app/flow/docs", import.meta.url));

// https://vitejs.dev/config/
export default defineConfig({
  resolve: { alias: { "@nodedocs": nodeDocs } },
  server: {
    host: process.env.WAILS_VITE_HOST || "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
    // The dev server refuses to serve files outside its root; node docs are
    // deliberately outside it.
    fs: { allow: [".", nodeDocs] },
  },
  plugins: [vue(), tailwindcss(), wails("./bindings"), icons({ compiler: "vue3" })],
});
