import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vitest/config';

export default defineConfig({
	plugins: [
		// SvelteKit 3: configuration lives here (svelte.config.js is no longer supported).
		sveltekit({
			adapter: adapter({ pages: 'build', assets: 'build', fallback: 'index.html', strict: true })
		})
	],
	server: { proxy: { '/api': 'http://127.0.0.1:7878' } },
	test: { include: ['src/**/*.{test,spec}.{js,ts}'], environment: 'node' }
});
