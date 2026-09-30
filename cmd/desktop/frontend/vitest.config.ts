import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import icons from 'unplugin-icons/vite'

// Mirrors vite.config.ts — see the note there on why node docs live in the Go tree.
const nodeDocs = fileURLToPath(new URL('../internal/app/flow/docs', import.meta.url))

export default defineConfig({
  resolve: { alias: { '@nodedocs': nodeDocs } },
  server: { fs: { allow: ['.', nodeDocs] } },
  plugins: [vue(), icons({ compiler: 'vue3' })],
  test: {
    // Worker threads rather than the default forked processes. The suite pays
    // ~87s of per-file setup against ~26s of actual assertions in CI, and a
    // thread is the cheaper of the two to stand up. Not `isolate: false`, which
    // would save far more: 132 of 134 files fail without a fresh module
    // registry each.
    pool: 'threads',
    environment: 'happy-dom',
    globals: true,
    include: ['src/**/*.spec.ts'],
    setupFiles: ['./src/test-setup.ts'],
  },
})
