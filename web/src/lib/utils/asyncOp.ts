// Async operation helper — eliminates repeated loading/error/try-catch boilerplate.
// In components, wrap with $state() for reactivity: `let op = $state(createAsyncOp())`

import { getErrorMessage } from './apiError';

export interface AsyncOp {
	loading: boolean;
	error: string | null;
	run<T>(
		fn: () => Promise<T>,
		onError?: (msg: string) => void,
		fallback?: string
	): Promise<T | undefined>;
}

export function createAsyncOp(initialLoading = false): AsyncOp {
	return {
		loading: initialLoading,
		error: null,
		async run<T>(
			this: AsyncOp,
			fn: () => Promise<T>,
			onError?: (msg: string) => void,
			fallback = 'An error occurred'
		): Promise<T | undefined> {
			this.loading = true;
			this.error = null;
			try {
				return await fn();
			} catch (e: unknown) {
				this.error = getErrorMessage(e, fallback);
				if (onError) onError(this.error!);
				return undefined;
			} finally {
				this.loading = false;
			}
		}
	};
}
