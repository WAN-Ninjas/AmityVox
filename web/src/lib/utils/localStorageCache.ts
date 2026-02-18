// Cached localStorage Record<string, V> with lazy loading and auto-save.
// Eliminates duplicated load/save/cache boilerplate across audio utilities.

export function createLocalStorageCache<V>(key: string) {
	let cached: Record<string, V> | null = null;

	function load(): Record<string, V> {
		if (cached !== null) return cached;
		try {
			const raw = localStorage.getItem(key);
			cached = raw ? JSON.parse(raw) : {};
		} catch {
			cached = {};
		}
		return cached!;
	}

	function save() {
		if (cached === null) return;
		try {
			localStorage.setItem(key, JSON.stringify(cached));
		} catch {
			// Ignore storage quota/availability errors.
		}
	}

	return {
		get(entryKey: string): V | undefined {
			return load()[entryKey];
		},
		set(entryKey: string, value: V) {
			load()[entryKey] = value;
			save();
		},
		remove(entryKey: string) {
			delete load()[entryKey];
			save();
		}
	};
}
