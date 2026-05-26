import { writable } from 'svelte/store';
import { api, type ClientConfig } from '$lib/api/client';
import { setLocalInstanceId } from '$lib/utils/avatar';

export const clientConfig = writable<ClientConfig | null>(null);

export async function loadClientConfig(guildId?: string): Promise<ClientConfig | null> {
	try {
		const config = await api.getClientConfig(guildId);
		clientConfig.set(config);
		setLocalInstanceId(config.local_instance_id);
		return config;
	} catch {
		return null;
	}
}

export function isFeatureEnabled(config: ClientConfig | null, feature: string): boolean {
	return Boolean(config?.feature_flags?.[feature]?.enabled ?? config?.experimental_features?.[feature]);
}

export function isExperimentalEnabled(config: ClientConfig | null, feature: string): boolean {
	return isFeatureEnabled(config, feature);
}
