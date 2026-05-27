import { writable } from 'svelte/store';

export interface DesktopInstance {
	url: string;
	name: string;
}

interface DesktopInstanceState {
	enabled: boolean;
	activeUrl: string | null;
	instances: DesktopInstance[];
}

const INSTANCES_KEY = 'amityvox.desktop.instances';
const ACTIVE_KEY = 'amityvox.desktop.activeInstance';
const DEFAULT_INSTANCE = 'https://app.amityvox.chat';

export const desktopInstances = writable<DesktopInstanceState>({
	enabled: false,
	activeUrl: null,
	instances: []
});

export function isTauriRuntime(): boolean {
	if (typeof window === 'undefined') return false;
	const w = window as Window & {
		__TAURI_INTERNALS__?: unknown;
		__TAURI__?: unknown;
	};
	return Boolean(w.__TAURI_INTERNALS__ || w.__TAURI__);
}

export function normalizeInstanceUrl(value: string): string {
	const trimmed = value.trim().replace(/\/+$/, '');
	if (!trimmed) throw new Error('Instance URL is required');

	const withProtocol = /^[a-z][a-z0-9+.-]*:\/\//i.test(trimmed)
		? trimmed
		: /^(localhost|127\.0\.0\.1|\[::1\])(?::\d+)?$/i.test(trimmed)
			? `http://${trimmed}`
			: `https://${trimmed}`;

	const url = new URL(withProtocol);
	if (url.protocol !== 'http:' && url.protocol !== 'https:') {
		throw new Error('Instance URL must use http or https');
	}
	url.pathname = '';
	url.search = '';
	url.hash = '';
	return url.toString().replace(/\/+$/, '');
}

export function initDesktopInstances() {
	if (!isTauriRuntime()) return;

	const instances = readInstances();
	const activeUrl = localStorage.getItem(ACTIVE_KEY) || instances[0]?.url || DEFAULT_INSTANCE;
	const next = ensureInstance(instances, activeUrl);
	localStorage.setItem(INSTANCES_KEY, JSON.stringify(next));
	localStorage.setItem(ACTIVE_KEY, activeUrl);

	desktopInstances.set({ enabled: true, activeUrl, instances: next });
}

export function getActiveInstanceUrl(): string | null {
	if (!isTauriRuntime()) return null;
	const saved = localStorage.getItem(ACTIVE_KEY);
	if (!saved) return DEFAULT_INSTANCE;
	try {
		return normalizeInstanceUrl(saved);
	} catch {
		return DEFAULT_INSTANCE;
	}
}

export function setActiveInstanceUrl(value: string) {
	const url = normalizeInstanceUrl(value);
	const instances = ensureInstance(readInstances(), url);
	localStorage.setItem(INSTANCES_KEY, JSON.stringify(instances));
	localStorage.setItem(ACTIVE_KEY, url);
	desktopInstances.set({ enabled: true, activeUrl: url, instances });
}

export function removeInstanceUrl(value: string) {
	const url = normalizeInstanceUrl(value);
	const remaining = readInstances().filter((instance) => instance.url !== url);
	const activeUrl = localStorage.getItem(ACTIVE_KEY);
	const nextActive = activeUrl === url ? remaining[0]?.url || DEFAULT_INSTANCE : activeUrl || DEFAULT_INSTANCE;
	const next = ensureInstance(remaining, nextActive);
	localStorage.setItem(INSTANCES_KEY, JSON.stringify(next));
	localStorage.setItem(ACTIVE_KEY, nextActive);
	desktopInstances.set({ enabled: true, activeUrl: nextActive, instances: next });
}

export function getScopedStorageKey(key: string): string {
	const instanceUrl = getActiveInstanceUrl();
	if (!instanceUrl) return key;
	return `${key}:${instanceUrl}`;
}

export function getApiBase(): string {
	const instanceUrl = getActiveInstanceUrl();
	return instanceUrl ? `${instanceUrl}/api/v1` : '/api/v1';
}

export function getPublicOrigin(): string {
	return getActiveInstanceUrl() || (typeof location === 'undefined' ? '' : location.origin);
}

export function getWebSocketUrl(): string {
	const instanceUrl = getActiveInstanceUrl();
	if (instanceUrl) {
		const url = new URL(instanceUrl);
		url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
		url.pathname = '/ws';
		return url.toString();
	}

	if (typeof location === 'undefined') return 'ws://localhost/ws';
	return `${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/ws`;
}

function readInstances(): DesktopInstance[] {
	try {
		const parsed = JSON.parse(localStorage.getItem(INSTANCES_KEY) || '[]') as DesktopInstance[];
		return parsed
			.map((instance) => ({ url: normalizeInstanceUrl(instance.url), name: instance.name || labelForUrl(instance.url) }))
			.filter((instance, index, all) => all.findIndex((item) => item.url === instance.url) === index);
	} catch {
		return [];
	}
}

function ensureInstance(instances: DesktopInstance[], url: string): DesktopInstance[] {
	const normalized = normalizeInstanceUrl(url);
	if (instances.some((instance) => instance.url === normalized)) return instances;
	return [...instances, { url: normalized, name: labelForUrl(normalized) }];
}

function labelForUrl(value: string): string {
	try {
		return new URL(normalizeInstanceUrl(value)).host;
	} catch {
		return value;
	}
}
