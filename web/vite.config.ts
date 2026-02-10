import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [sveltekit()],
	server: {
		proxy: {
			// Proxy API calls to the Go backend during development.
			'/api': 'http://localhost:8080',
			'/ws': {
				target: 'ws://localhost:8081',
				ws: true
			}
		}
	}
});
