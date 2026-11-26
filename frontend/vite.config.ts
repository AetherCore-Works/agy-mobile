import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';

const gatewayTarget = process.env.VITE_GATEWAY_URL || 'http://127.0.0.1:8088';

export default defineConfig({
  plugins: [tailwindcss(), svelte()],
  build: {
    outDir: '../backend-go/internal/web/static/dist',
    emptyOutDir: true,
    sourcemap: false,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('node_modules/@xterm')) return 'xterm';
          if (id.includes('node_modules/highlight.js') || id.includes('node_modules/marked')) {
            return 'markdown';
          }
          if (
            id.includes('node_modules/bits-ui') ||
            id.includes('node_modules/svelte-toolbelt') ||
            id.includes('node_modules/runed')
          ) {
            return 'ui-vendor';
          }
        },
      },
    },
  },
  server: {
    proxy: {
      '/api': gatewayTarget,
      '/terminal/ws': {
        target: gatewayTarget.replace(/^http/, 'ws'),
        ws: true,
      },
    },
  },
});
