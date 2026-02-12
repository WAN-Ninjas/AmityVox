import { defineConfig } from 'vitest/config';
import { svelte } from '@sveltejs/vite-plugin-svelte';

export default defineConfig({
	plugins: [svelte({ hot: !process.env.VITEST })],
	test: {
		include: ['src/**/*.test.ts'],
		environment: 'happy-dom',
		globals: true,
		setupFiles: ['src/test-setup.ts'],
		alias: {
			$lib: '/src/lib',
			$components: '/src/lib/components'
		}
	}
});
