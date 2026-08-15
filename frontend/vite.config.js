import { defineConfig } from 'vite'
import path from 'path'

export default defineConfig({
  root: '.',
  publicDir: 'public',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    rollupOptions: {
      input: {
        index: path.resolve(__dirname, 'index.html'),
      },
    },
    target: 'es2020',
  },
  server: {
    port: 34115,
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, 'src'),
    },
  },
  esbuild: {
    target: 'es2020',
  },
  plugins: [
    {
      name: 'inline-script',
      transformIndexHtml(html) {
        return html.replace(
          '</body>',
          `<script>
  // Notify Go that the frontend is ready
  async function notifyGo() {
    try {
      if (window.go && window.go.main && window.go.main.App && window.go.main.App.CreateApp) {
        await window.go.main.App.CreateApp();
      }
    } catch (e) {
      console.error('Failed to notify Go:', e);
    }
  }
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', notifyGo);
  } else {
    notifyGo();
  }
</script>
</body>`
        );
      }
    }
  ],
})