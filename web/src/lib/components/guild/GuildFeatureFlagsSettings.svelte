<script lang="ts">
	import { api, type FeatureFlagState } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { createAsyncOp } from '$lib/utils/asyncOp';

	interface Props {
		guildId: string;
	}

	let { guildId }: Props = $props();

	let features = $state<FeatureFlagState[]>([]);
	let loadedGuildId = $state<string | null>(null);
	let loadOp = $state(createAsyncOp());
	let saveOps = $state<Record<string, ReturnType<typeof createAsyncOp>>>({});

	$effect(() => {
		if (guildId && loadedGuildId !== guildId && !loadOp.loading) {
			loadFeatures();
		}
	});

	async function loadFeatures() {
		const result = await loadOp.run(
			() => api.getGuildFeatureFlags(guildId),
			msg => addToast(msg, 'error'),
			'Failed to load server feature flags'
		);
		if (result) {
			features = Object.values(result.features).sort((a, b) => a.name.localeCompare(b.name));
			loadedGuildId = guildId;
		}
	}

	function opFor(featureKey: string) {
		saveOps[featureKey] ??= createAsyncOp();
		return saveOps[featureKey];
	}

	function opLoading(featureKey: string) {
		return saveOps[featureKey]?.loading ?? false;
	}

	async function updateFeature(feature: FeatureFlagState, enabled: boolean) {
		const op = opFor(feature.key);
		const result = await op.run(
			() => api.updateGuildFeatureFlag(guildId, feature.key, { enabled }),
			msg => addToast(msg, 'error'),
			'Failed to update server feature flag'
		);
		if (result) {
			features = features.map((item) => item.key === feature.key ? result : item)
				.sort((a, b) => a.name.localeCompare(b.name));
			addToast('Server feature flag updated', 'success');
		}
	}
</script>

<div class="mb-6">
	<h1 class="text-xl font-bold text-text-primary">Feature Flags</h1>
	<p class="mt-1 text-sm text-text-muted">Control feature availability for this server. Instance hard-disabled features cannot be enabled here.</p>
</div>

{#if loadOp.loading}
	<p class="text-sm text-text-muted">Loading feature flags...</p>
{:else if loadOp.error}
	<div class="rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{loadOp.error}</div>
{:else}
	<div class="overflow-hidden rounded-lg border border-bg-floating bg-bg-secondary">
		<table class="w-full text-left text-sm">
			<thead class="bg-bg-primary text-xs uppercase text-text-muted">
				<tr>
					<th class="px-4 py-3 font-semibold">Feature</th>
					<th class="px-4 py-3 font-semibold">Server Override</th>
					<th class="px-4 py-3 font-semibold">Effective State</th>
				</tr>
			</thead>
			<tbody>
				{#each features as feature (feature.key)}
					<tr class="border-t border-bg-floating">
						<td class="px-4 py-3">
							<div class="font-medium text-text-primary">{feature.name}</div>
							<div class="mt-0.5 text-xs text-text-muted">{feature.description}</div>
							<code class="mt-1 block text-2xs text-text-muted">{feature.key}</code>
						</td>
						<td class="px-4 py-3">
							<label class="inline-flex items-center gap-2">
								<input
									type="checkbox"
									class="rounded accent-brand-500"
									checked={feature.guild_enabled ?? feature.instance_enabled}
									disabled={opLoading(feature.key) || feature.instance_hard_disabled}
									onchange={(event) => updateFeature(feature, event.currentTarget.checked)}
								/>
								<span class="text-text-secondary">
									{feature.instance_hard_disabled ? 'Locked by instance' : (feature.guild_enabled ?? feature.instance_enabled) ? 'Enabled' : 'Disabled'}
								</span>
							</label>
						</td>
						<td class="px-4 py-3">
							<span class="rounded px-2 py-1 text-xs font-medium {feature.enabled ? 'bg-green-500/10 text-green-400' : 'bg-red-500/10 text-red-400'}">
								{feature.enabled ? 'Available' : `Disabled by ${feature.disabled_by ?? 'policy'}`}
							</span>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}
