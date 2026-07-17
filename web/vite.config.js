import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [
		sveltekit({
			compilerOptions: {
				// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},

			// Static build embedded into the Go binary via go:embed.
			adapter: adapter()
		})
	],
	server: {
		// Dev mode: the Go backend runs on :8080; the SPA talks to it via proxy.
		proxy: {
			'/api': 'http://127.0.0.1:8080',
			'/events': 'http://127.0.0.1:8080'
		}
	}
});
