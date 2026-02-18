// Map store helpers — eliminates repeated `update(map => { ...; return new Map(map); })` boilerplate.
// Compatible with Svelte's writable store interface ($store, derived, get, subscribe/set/update).

import { writable } from 'svelte/store';

export function createMapStore<K, V>() {
	const { subscribe, set, update } = writable<Map<K, V>>(new Map());

	return {
		subscribe,
		set,
		update,

		/** Replace the entire map with a new one built from entries. */
		setAll(entries: [K, V][] | Map<K, V>) {
			set(entries instanceof Map ? new Map(entries) : new Map(entries));
		},

		/** Set a single entry and trigger reactivity. */
		setEntry(key: K, value: V) {
			update(map => { map.set(key, value); return new Map(map); });
		},

		/** Update an existing entry by merging partial fields. Does nothing if key doesn't exist. */
		updateEntry(key: K, updater: (existing: V) => V) {
			update(map => {
				const existing = map.get(key);
				if (existing === undefined) return map;
				map.set(key, updater(existing));
				return new Map(map);
			});
		},

		/** Remove an entry and trigger reactivity. */
		removeEntry(key: K) {
			update(map => { map.delete(key); return new Map(map); });
		},

		/** Clear the entire map. */
		clear() {
			set(new Map());
		}
	};
}
