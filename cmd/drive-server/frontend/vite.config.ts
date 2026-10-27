import { writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import icons from 'unplugin-icons/vite'

// The workspace package resolves through a symlink to a directory outside this root.
const hiveUi = fileURLToPath(new URL('../../../web/packages/ui', import.meta.url))

export default defineConfig({
  server: {
    host: '127.0.0.1',
    port: 5173,
    fs: { allow: ['.', hiveUi] },
    proxy: {
      // Anchored: a bare '/s' would also proxy the dev server's own /src modules.
      '^/api/': 'http://127.0.0.1:8080',
      '^/s/': 'http://127.0.0.1:8080',
    },
  },
  plugins: [
    vue(),
    tailwindcss(),
    icons({ compiler: 'vue3' }),
    {
      // Vite empties dist/ on build, and the Go embed needs the directory to hold a tracked file.
      name: 'keep-dist-gitkeep',
      closeBundle: () => writeFileSync(fileURLToPath(new URL('./dist/.gitkeep', import.meta.url)), ''),
    },
  ],
})
