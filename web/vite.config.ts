import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

// Link preview crawlers need an absolute image URL, so the build writes the deployment's browser origin into index.html.
const publicWebURL = (process.env.PUBLIC_WEB_URL || 'https://os.subcult.tv').replace(/\/+$/, '');

export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
    { name: 'public-web-url', transformIndexHtml: { order: 'pre', handler: (html) => html.replaceAll('%PUBLIC_WEB_URL%', publicWebURL) } },
  ],
  server: {
    proxy: {
      '/api': process.env.DEV_API_TARGET || 'http://localhost:8080',
    },
  },
});
