// See https://svelte.dev/docs/kit/types#app.d.ts
declare global {
	namespace App {
		interface Error {
			code?: string;
			message: string;
		}
	}
}

export {};
