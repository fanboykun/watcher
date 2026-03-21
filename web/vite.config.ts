import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	server: {
		proxy: {
			// Proxy /api requests to the Go watcher backend during dev
			'/api': {
				target: 'http://localhost:8080',
				changeOrigin: true
			}
		}
	}
});
