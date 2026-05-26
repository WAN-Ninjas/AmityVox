<script lang="ts">
	import { api, type FeatureFlagState } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { createAsyncOp } from '$lib/utils/asyncOp';

	let features = $state<FeatureFlagState[]>([]);
	let loadOp = $state(createAsyncOp());
	let saveOps = $state<Record<string, ReturnType<typeof createAsyncOp>>>({});

	$effect(() => {
		if (features.length === 0 && !loadOp.loading) {
			loadFeatures();
		}
	});

	async function loadFeatures() {
		const result = await loadOp.run(
			() => api.getAdminFeatureFlags(),
			msg => addToast(msg, 'error'),
			'Failed to load feature flags'
		);
		if (result) {
			features = Object.values(result.features).sort((a, b) => a.name.localeCompare(b.name));
		}
	}

	function opFor(featureKey: string) {
		saveOps[featureKey] ??= createAsyncOp();
		return saveOps[featureKey];
	}

	function opLoading(featureKey: string) {
		return saveOps[featureKey]?.loading ?? false;
	}

	async function updateFeature(feature: FeatureFlagState, patch: { enabled?: boolean; hard_disabled?: boolean }) {
		const op = opFor(feature.key);
		const result = await op.run(
			() => api.updateAdminFeatureFlag(feature.key, patch),
			msg => addToast(msg, 'error'),
			'Failed to update feature flag'
		);
		if (result) {
			features = features.map(item => item.key === feature.key ? result : item)
				.sort((a, b) => a.name.localeCompare(b.name));
			addToast('Feature flag updated', 'success');
		}
	}
</script>

<div class="mb-6">
	<h1 class="text-2xl font-bold text-text-primary">Feature Flags</h1>
	<p class="mt-1 text-sm text-text-muted">Control instance-wide feature availability. Hard-disabled features cannot be enabled by servers.</p>
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
					<th class="px-4 py-3 font-semibold">Instance</th>
					<th class="px-4 py-3 font-semibold">Hard Disable</th>
					<th class="px-4 py-3 font-semibold">State</th>
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
									checked={feature.instance_enabled}
									disabled={opLoading(feature.key)}
									onchange={(event) => updateFeature(feature, { enabled: event.currentTarget.checked })}
								/>
								<span class="text-text-secondary">{feature.instance_enabled ? 'Enabled' : 'Disabled'}</span>
							</label>
						</td>
						<td class="px-4 py-3">
							<label class="inline-flex items-center gap-2">
								<input
									type="checkbox"
									class="rounded accent-red-500"
									checked={feature.instance_hard_disabled}
									disabled={opLoading(feature.key)}
									onchange={(event) => updateFeature(feature, { hard_disabled: event.currentTarget.checked })}
								/>
								<span class="text-text-secondary">{feature.instance_hard_disabled ? 'Hard off' : 'Guilds may configure'}</span>
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
