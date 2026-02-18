import { describe, it, expect, vi } from 'vitest';
import { get } from 'svelte/store';
import { createMapStore } from '$lib/stores/mapHelpers';

describe('createMapStore', () => {
	it('initializes with an empty map', () => {
		const store = createMapStore<string, number>();
		expect(get(store).size).toBe(0);
	});

	it('setEntry adds a new entry', () => {
		const store = createMapStore<string, number>();
		store.setEntry('a', 1);
		expect(get(store).get('a')).toBe(1);
	});

	it('setEntry overwrites an existing entry', () => {
		const store = createMapStore<string, number>();
		store.setEntry('a', 1);
		store.setEntry('a', 2);
		expect(get(store).get('a')).toBe(2);
	});

	it('setEntry triggers reactivity (new reference)', () => {
		const store = createMapStore<string, number>();
		const refs: Map<string, number>[] = [];
		const unsub = store.subscribe(v => refs.push(v));
		store.setEntry('a', 1);
		store.setEntry('b', 2);
		expect(refs.length).toBe(3); // initial + 2 updates
		expect(refs[0]).not.toBe(refs[1]);
		expect(refs[1]).not.toBe(refs[2]);
		unsub();
	});

	it('removeEntry removes an entry', () => {
		const store = createMapStore<string, number>();
		store.setEntry('a', 1);
		store.setEntry('b', 2);
		store.removeEntry('a');
		expect(get(store).has('a')).toBe(false);
		expect(get(store).has('b')).toBe(true);
	});

	it('removeEntry triggers reactivity', () => {
		const store = createMapStore<string, number>();
		store.setEntry('a', 1);
		const refs: Map<string, number>[] = [];
		const unsub = store.subscribe(v => refs.push(v));
		store.removeEntry('a');
		expect(refs.length).toBe(2);
		unsub();
	});

	it('updateEntry updates existing entry via updater', () => {
		const store = createMapStore<string, { name: string; count: number }>();
		store.setEntry('a', { name: 'foo', count: 1 });
		store.updateEntry('a', v => ({ ...v, count: v.count + 1 }));
		expect(get(store).get('a')).toEqual({ name: 'foo', count: 2 });
	});

	it('updateEntry does nothing for non-existent key', () => {
		const store = createMapStore<string, number>();
		const fn = vi.fn((v: number) => v + 1);
		store.updateEntry('missing', fn);
		expect(fn).not.toHaveBeenCalled();
		expect(get(store).size).toBe(0);
	});

	it('setAll replaces entire map from array', () => {
		const store = createMapStore<string, number>();
		store.setEntry('old', 99);
		store.setAll([['a', 1], ['b', 2]]);
		expect(get(store).size).toBe(2);
		expect(get(store).get('a')).toBe(1);
		expect(get(store).has('old')).toBe(false);
	});

	it('setAll replaces entire map from Map', () => {
		const store = createMapStore<string, number>();
		store.setEntry('old', 99);
		store.setAll(new Map([['x', 10]]));
		expect(get(store).size).toBe(1);
		expect(get(store).get('x')).toBe(10);
	});

	it('clear empties the map', () => {
		const store = createMapStore<string, number>();
		store.setEntry('a', 1);
		store.setEntry('b', 2);
		store.clear();
		expect(get(store).size).toBe(0);
	});

	it('is compatible with svelte derived stores', async () => {
		const { derived } = await import('svelte/store');
		const store = createMapStore<string, number>();
		const count = derived(store, ($map) => $map.size);
		store.setEntry('a', 1);
		store.setEntry('b', 2);
		expect(get(count)).toBe(2);
	});

	it('set method works directly (writable compatibility)', () => {
		const store = createMapStore<string, number>();
		store.set(new Map([['a', 1], ['b', 2]]));
		expect(get(store).size).toBe(2);
	});

	it('update method works directly (writable compatibility)', () => {
		const store = createMapStore<string, number>();
		store.setEntry('a', 1);
		store.update(map => {
			map.set('b', 2);
			return new Map(map);
		});
		expect(get(store).size).toBe(2);
	});
});
