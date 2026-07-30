import { fileURLToPath, URL } from 'node:url';
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { viteSingleFile } from 'vite-plugin-singlefile';

const outputDirectories: Record<string, string> = {
  observability: '../web',
  'proxy-pool': '../../proxy-pool/web',
  'oauth-model-policy': '../../oauth-model-policy/web',
};

export default defineConfig(({ mode }) => ({
  plugins: [react(), viteSingleFile()],
  define: {
    __PLUGIN_MANAGEMENT_PAGE__: JSON.stringify(mode),
  },
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  css: {
    modules: { localsConvention: 'camelCase' },
    preprocessorOptions: {
      scss: { additionalData: '@use "@/styles/variables.scss" as *;' },
    },
  },
  build: {
    target: 'es2022',
    outDir: outputDirectories[mode] || outputDirectories.observability,
    emptyOutDir: true,
    cssCodeSplit: false,
    assetsInlineLimit: 100000000,
  },
}));
