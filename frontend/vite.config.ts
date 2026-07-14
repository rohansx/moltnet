import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { fileURLToPath, URL } from 'node:url';

// Dev: Vite serves the app on :5174 and proxies the registry API to the Go
// server on :8787, so the React app talks to the real backend with cookies.
// Prod: `pnpm build` → dist/, served by moltnetd with SPA fallback.
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      // The browser verifier is NOT reimplemented here — it is the same
      // @moltnet/client library the Node/TS consumers use, which is pinned to
      // the Go reference by spec/conformance/*.json. One implementation, one
      // set of vectors: canonicalization and MoltScore cannot drift apart.
      '@moltnet/client': fileURLToPath(new URL('../clients/ts/src/index.ts', import.meta.url)),
    },
  },
  server: {
    port: 5174,
    proxy: {
      '/v1': { target: 'http://localhost:8787', changeOrigin: false },
      '/federation': { target: 'http://localhost:8787', changeOrigin: false },
      '/.well-known': { target: 'http://localhost:8787', changeOrigin: false },
      '/openapi.json': { target: 'http://localhost:8787', changeOrigin: false },
      '/healthz': { target: 'http://localhost:8787', changeOrigin: false },
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
  },
});