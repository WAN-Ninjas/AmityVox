import { writable } from 'svelte/store';
import { api, type ClientConfig } from '$lib/api/client';
import { setLocalInstanceId } from '$lib/utils/avatar';

export const clientConfig = writable<ClientConfig | null>(null);

export async function loadClientConfig(): Promise<ClientConfig | null> {
	try {
		const config = await api.getClientConfig();
		clientConfig.set(config);
		setLocalInstanceId(config.local_instance_id);
		return config;
	} catch {
		return null;
	}
}

export function isExperimentalEnabled(config: ClientConfig | null, feature: string): boolean {
	return Boolean(config?.experimental_features?.[feature]);
}
