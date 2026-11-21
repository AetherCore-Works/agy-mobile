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
