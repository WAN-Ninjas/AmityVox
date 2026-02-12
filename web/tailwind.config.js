/** @type {import('tailwindcss').Config} */
export default {
	content: ['./src/**/*.{html,js,svelte,ts}'],
	darkMode: 'class',
	theme: {
		extend: {
			colors: {
				// Theme-aware colors using CSS custom properties.
				brand: {
					50: 'var(--brand-50)',
					100: 'var(--brand-100)',
					200: 'var(--brand-200)',
					300: 'var(--brand-300)',
					400: 'var(--brand-400)',
					500: 'var(--brand-500)',
					600: 'var(--brand-600)',
					700: 'var(--brand-700)',
					800: 'var(--brand-800)',
					900: 'var(--brand-900)'
				},
				bg: {
					primary: 'var(--bg-primary)',
					secondary: 'var(--bg-secondary)',
					tertiary: 'var(--bg-tertiary)',
					modifier: 'var(--bg-modifier)',
					floating: 'var(--bg-floating)'
				},
				text: {
					primary: 'var(--text-primary)',
					secondary: 'var(--text-secondary)',
					muted: 'var(--text-muted)',
					link: 'var(--text-link)'
				},
				status: {
					online: 'var(--status-online)',
					idle: 'var(--status-idle)',
					dnd: 'var(--status-dnd)',
					offline: 'var(--status-offline)'
				}
			},
			fontSize: {
				'2xs': '0.625rem'
			}
		}
	},
	plugins: []
};
