import { writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import vuetify from 'vite-plugin-vuetify'

const backend = process.env.BACKEND || 'http://127.0.0.1:8080'

// The build is embedded into the Go executable (scada/backend/internal/web).
const outDir = fileURLToPath(new URL('../backend/internal/web/dist', import.meta.url))

// go:embed needs at least one file in dist even before the first frontend
// build, so the placeholder is recreated after Vite empties the directory.
const keepPlaceholder = {
  name: 'keep-gitkeep',
  closeBundle() {
    writeFileSync(`${outDir}/.gitkeep`, '')
  },
}

export default defineConfig({
  plugins: [vue(), vuetify({ autoImport: true }), keepPlaceholder],
  build: {
    outDir,
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': backend,
      '/images': backend,
      '/ws': { target: backend.replace(/^http/, 'ws'), ws: true },
    },
  },
})
