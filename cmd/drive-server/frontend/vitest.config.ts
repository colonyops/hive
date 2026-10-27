import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import icons from 'unplugin-icons/vite'

export default defineConfig({
  plugins: [vue(), icons({ compiler: 'vue3' })],
  test: {
    pool: 'threads',
    environment: 'happy-dom',
    globals: true,
    include: ['src/**/*.spec.ts'],
    setupFiles: ['./src/test-setup.ts'],
  },
})
