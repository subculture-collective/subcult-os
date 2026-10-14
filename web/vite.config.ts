import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
export default defineConfig({
  define: { "import.meta.env.VITE_PUBLIC_WEB_URL": JSON.stringify(process.env.PUBLIC_WEB_URL || 'https://os.subcult.tv') },
  plugins: [react(), tailwindcss()],
  server: { proxy: { '/api': process.env.DEV_API_TARGET || 'http://localhost:8080' } },
});
