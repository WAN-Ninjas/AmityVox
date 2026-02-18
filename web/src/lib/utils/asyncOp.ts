// Async operation helper — eliminates repeated loading/error/try-catch boilerplate.
// In components, wrap with $state() for reactivity: `let op = $state(createAsyncOp())`

export interface AsyncOp {
	loading: boolean;
	error: string | null;
	run<T>(fn: () => Promise<T>, onError?: (msg: string) => void): Promise<T | undefined>;
}

export function createAsyncOp(): AsyncOp {
	return {
		loading: false,
		error: null,
		async run<T>(this: AsyncOp, fn: () => Promise<T>, onError?: (msg: string) => void): Promise<T | undefined> {
			this.loading = true;
			this.error = null;
			try {
				return await fn();
			} catch (e: any) {
				this.error = e?.message || 'An error occurred';
				if (onError) onError(this.error!);
				return undefined;
			} finally {
				this.loading = false;
			}
		}
	};
}
