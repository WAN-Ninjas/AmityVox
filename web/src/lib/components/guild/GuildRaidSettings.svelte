<script lang="ts">
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { getErrorMessage } from '$lib/utils/apiError';
	import { createAsyncOp } from '$lib/utils/asyncOp';
	import type { RaidConfig } from '$lib/types';

	interface Props {
		guildId: string;
	}

	let { guildId }: Props = $props();

	let raidConfig = $state<RaidConfig | null>(null);
	let loadOp = $state(createAsyncOp());
	let saveOp = $state(createAsyncOp());
	let loadedGuildId = $state<string | null>(null);

	$effect(() => {
		if (guildId && loadedGuildId !== guildId && !loadOp.loading) {
			loadRaid();
		}
	});

	async function loadRaid() {
		const result = await loadOp.run(
			() => api.getRaidConfig(guildId),
			msg => addToast(msg, 'error'),
			'Failed to load raid configuration'
		);
		if (result) {
			raidConfig = result;
			loadedGuildId = guildId;
		}
	}

	async function handleSaveRaid() {
		if (!raidConfig) return;
		const config = raidConfig;
		const result = await saveOp.run(
			() => api.updateRaidConfig(guildId, {
				enabled: config.enabled,
				join_rate_limit: config.join_rate_limit,
				join_rate_window: config.join_rate_window,
				min_account_age: config.min_account_age,
				lockdown_active: config.lockdown_active
			}),
			msg => addToast(msg, 'error'),
			'Failed to save raid config'
		);
		if (result) {
			raidConfig = result;
			addToast('Raid protection settings saved', 'success');
		}
	}

	function formatDate(iso: string): string {
		return new Date(iso).toLocaleString();
	}
</script>

<h1 class="mb-6 text-xl font-bold text-text-primary">Raid Protection</h1>

{#if loadOp.loading}
	<p class="text-sm text-text-muted">Loading raid configuration...</p>
{:else if raidConfig}
	<div class="space-y-6">
		<label class="flex items-center gap-3">
			<input type="checkbox" bind:checked={raidConfig.enabled} class="rounded" />
			<div>
				<span class="text-sm font-medium text-text-primary">Enable Raid Protection</span>
				<p class="text-xs text-text-muted">Automatically detect and respond to join raids</p>
			</div>
		</label>

		{#if raidConfig.enabled}
			<div class="grid grid-cols-2 gap-4">
				<div>
					<label for="raidJoinRateLimit" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Max Joins Per Window</label>
					<input id="raidJoinRateLimit" type="number" class="input w-full" bind:value={raidConfig.join_rate_limit} min="1" max="100" />
				</div>
				<div>
					<label for="raidJoinRateWindow" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Window (seconds)</label>
					<input id="raidJoinRateWindow" type="number" class="input w-full" bind:value={raidConfig.join_rate_window} min="5" max="300" />
				</div>
				<div>
					<label for="raidMinAccountAge" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Min Account Age (seconds)</label>
					<input id="raidMinAccountAge" type="number" class="input w-full" bind:value={raidConfig.min_account_age} min="0" max="604800" />
					<p class="mt-1 text-xs text-text-muted">0 = no requirement. 300 = 5 minutes.</p>
				</div>
			</div>
		{/if}

		<div class="border-t border-bg-modifier pt-4">
			<label class="flex items-center gap-3">
				<input type="checkbox" bind:checked={raidConfig.lockdown_active} class="rounded" />
				<div>
					<span class="text-sm font-medium text-text-primary {raidConfig.lockdown_active ? 'text-red-400' : ''}">
						{raidConfig.lockdown_active ? 'Lockdown Active' : 'Manual Lockdown'}
					</span>
					<p class="text-xs text-text-muted">When enabled, new joins are blocked and invites are paused</p>
					{#if raidConfig.lockdown_active && raidConfig.lockdown_started_at}
						<p class="text-xs text-red-400">Started: {formatDate(raidConfig.lockdown_started_at)}</p>
					{/if}
				</div>
			</label>
		</div>

		<button class="btn-primary" onclick={handleSaveRaid} disabled={saveOp.loading}>
			{saveOp.loading ? 'Saving...' : 'Save Raid Settings'}
		</button>
	</div>
{/if}
