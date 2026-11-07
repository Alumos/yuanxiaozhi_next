import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'node:path'

const pages = ['index', 'admin', 'login', 'jisuan/index', 'jisuan/screen', 'shuchu/index', 'shuchu/screen', 'shuru/index', 'shuru/screen']

export default defineConfig({
  plugins: [vue()],
  base: '/',
  build: {
    outDir: resolve(__dirname, '../server/internal/core/dist'),
    emptyOutDir: true,
    target: ['es2019', 'safari12'],
    cssTarget: ['safari12'],
    assetsInlineLimit: 8192,
    rollupOptions: { input: Object.fromEntries(pages.map((p) => [p, resolve(__dirname, p + '.html')])) },
  },
  server: { proxy: { '/api': 'http://localhost:8080', '/events': { target: 'http://localhost:8080', ws: false }, '/uploads': 'http://localhost:8080' } },
})
