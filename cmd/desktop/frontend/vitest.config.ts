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
    // Each file still gets a fresh module registry, which 132 of 134 files need,
    // but it reuses one loaded happy-dom per thread. With `threads`, standing up
    // the environment for every file was the largest single cost of the suite.
    pool: 'vmThreads',
    environment: 'happy-dom',
    globals: true,
    include: ['src/**/*.spec.ts'],
    setupFiles: ['./src/test-setup.ts'],
  },
})
