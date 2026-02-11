/** @type {import('tailwindcss').Config} */
export default {
	content: ['./src/**/*.{html,js,svelte,ts}'],
	darkMode: 'class',
	theme: {
		extend: {
			colors: {
				// AmityVox brand palette — dark theme by default (like Discord).
				brand: {
					50: '#e8eaf6',
					100: '#c5cae9',
					200: '#9fa8da',
					300: '#7986cb',
					400: '#5c6bc0',
					500: '#3f51b5',
					600: '#3949ab',
					700: '#303f9f',
					800: '#283593',
					900: '#1a237e'
				},
				bg: {
					primary: '#1e1f22',
					secondary: '#2b2d31',
					tertiary: '#313338',
					modifier: '#383a40',
					floating: '#111214'
				},
				text: {
					primary: '#f2f3f5',
					secondary: '#b5bac1',
					muted: '#949ba4',
					link: '#00a8fc'
				},
				status: {
					online: '#23a55a',
					idle: '#f0b232',
					dnd: '#f23f43',
					offline: '#80848e'
				}
			},
			fontSize: {
				'2xs': '0.625rem'
			}
		}
	},
	plugins: []
};
