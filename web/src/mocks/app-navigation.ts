// Mock for $app/navigation used in vitest
export function goto() {
	return Promise.resolve();
}

export function invalidateAll() {
	return Promise.resolve();
}

export function beforeNavigate() {}
export function afterNavigate() {}
