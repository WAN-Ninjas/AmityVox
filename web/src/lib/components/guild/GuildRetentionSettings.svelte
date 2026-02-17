<script lang="ts">
	import type { RetentionPolicy, Channel } from '$lib/types';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { channels } from '$lib/stores/channels';

	let { guildId }: { guildId: string } = $props();

	let policies = $state<RetentionPolicy[]>([]);
	let loading = $state(true);
	let creating = $state(false);

	// Create form state.
	let newScope = $state<'guild' | 'channel'>('guild');
	let newChannelId = $state('');
	let newMaxAge = $state(90);
	let newDeleteAttachments = $state(true);
	let newDeletePins = $state(false);

	// Get text/announcement channels for the dropdown.
	let guildChannels = $derived(
		Array.from($channels.values())
			.filter((c) => c.guild_id === guildId && !c.parent_channel_id && (c.channel_type === 'text' || c.channel_type === 'announcement' || c.channel_type === 'forum'))
			.sort((a, b) => a.position - b.position)
	);

	$effect(() => {
		loadPolicies();
	});

	async function loadPolicies() {
		loading = true;
		try {
			policies = await api.getGuildRetentionPolicies(guildId);
		} catch (err: any) {
			addToast(err.message || 'Failed to load retention policies', 'error');
		} finally {
			loading = false;
		}
	}

	async function handleCreate() {
		if (newMaxAge < 1) {
			addToast('Retention period must be at least 1 day', 'warning');
			return;
		}
		creating = true;
		try {
			const policy = await api.createGuildRetentionPolicy(guildId, {
				channel_id: newScope === 'channel' ? newChannelId : undefined,
				max_age_days: newMaxAge,
				delete_attachments: newDeleteAttachments,
				delete_pins: newDeletePins
			});
			policies = [policy, ...policies];
			addToast('Retention policy created', 'success');
			resetForm();
		} catch (err: any) {
			addToast(err.message || 'Failed to create policy', 'error');
		} finally {
			creating = false;
		}
	}

	async function handleToggle(policy: RetentionPolicy) {
		try {
			const updated = await api.updateGuildRetentionPolicy(guildId, policy.id, {
				enabled: !policy.enabled
			});
			policies = policies.map((p) => (p.id === policy.id ? updated : p));
		} catch (err: any) {
			addToast(err.message || 'Failed to update policy', 'error');
		}
	}

	async function handleDelete(policyId: string) {
		try {
			await api.deleteGuildRetentionPolicy(guildId, policyId);
			policies = policies.filter((p) => p.id !== policyId);
			addToast('Retention policy deleted', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to delete policy', 'error');
		}
	}

	function resetForm() {
		newScope = 'guild';
		newChannelId = '';
		newMaxAge = 90;
		newDeleteAttachments = true;
		newDeletePins = false;
	}

	function getChannelName(channelId: string | null): string {
		if (!channelId) return 'Guild-wide';
		const ch = $channels.get(channelId);
		return ch?.name ? `#${ch.name}` : channelId;
	}

	function formatAge(days: number): string {
		if (days >= 365) return `${Math.floor(days / 365)} year${days >= 730 ? 's' : ''}`;
		if (days >= 30) return `${Math.floor(days / 30)} month${days >= 60 ? 's' : ''}`;
		return `${days} day${days !== 1 ? 's' : ''}`;
	}

	function formatDate(iso: string): string {
		return new Date(iso).toLocaleString();
	}
</script>

<h1 class="mb-2 text-xl font-bold text-text-primary">Message Retention</h1>
<p class="mb-6 text-sm text-text-muted">
	Automatically delete messages older than a specified period. You can set guild-wide or per-channel policies.
</p>

<!-- Create form -->
<div class="mb-6 rounded-lg bg-bg-secondary p-4">
	<h3 class="mb-3 text-sm font-semibold text-text-primary">Create Policy</h3>
	<div class="space-y-3">
		<div>
			<label class="mb-1 block text-xs text-text-muted">Scope</label>
			<select
				bind:value={newScope}
				class="w-full rounded-md border border-bg-modifier bg-bg-tertiary px-3 py-2 text-sm text-text-primary"
			>
				<option value="guild">Guild-wide</option>
				<option value="channel">Specific channel</option>
			</select>
		</div>

		{#if newScope === 'channel'}
			<div>
				<label class="mb-1 block text-xs text-text-muted">Channel</label>
				<select
					bind:value={newChannelId}
					class="w-full rounded-md border border-bg-modifier bg-bg-tertiary px-3 py-2 text-sm text-text-primary"
				>
					<option value="">Select a channel...</option>
					{#each guildChannels as ch (ch.id)}
						<option value={ch.id}>#{ch.name}</option>
					{/each}
				</select>
			</div>
		{/if}

		<div>
			<label class="mb-1 block text-xs text-text-muted">Delete messages older than (days)</label>
			<input
				type="number"
				bind:value={newMaxAge}
				min="1"
				max="3650"
				class="w-full rounded-md border border-bg-modifier bg-bg-tertiary px-3 py-2 text-sm text-text-primary"
			/>
		</div>

		<div class="flex flex-wrap gap-4">
			<label class="flex items-center gap-2">
				<input type="checkbox" bind:checked={newDeleteAttachments} class="rounded" />
				<span class="text-sm text-text-primary">Delete attachments from storage</span>
			</label>
			<label class="flex items-center gap-2">
				<input type="checkbox" bind:checked={newDeletePins} class="rounded" />
				<span class="text-sm text-text-primary">Include pinned messages</span>
			</label>
		</div>

		<button
			class="rounded-md bg-brand-500 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-brand-600 disabled:opacity-50"
			onclick={handleCreate}
			disabled={creating || (newScope === 'channel' && !newChannelId)}
		>
			{creating ? 'Creating...' : 'Create Policy'}
		</button>
	</div>
</div>

<!-- Policy list -->
{#if loading}
	<div class="flex items-center justify-center py-8">
		<div class="h-6 w-6 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
	</div>
{:else if policies.length === 0}
	<div class="rounded-lg bg-bg-secondary p-6 text-center">
		<p class="text-sm text-text-muted">No retention policies configured.</p>
	</div>
{:else}
	<div class="space-y-2">
		{#each policies as policy (policy.id)}
			<div class="flex items-center justify-between rounded-lg bg-bg-secondary p-4">
				<div class="min-w-0 flex-1">
					<div class="flex items-center gap-2">
						<span class="text-sm font-medium text-text-primary">{getChannelName(policy.channel_id)}</span>
						<span class="rounded bg-bg-modifier px-1.5 py-0.5 text-xs text-text-muted">
							{formatAge(policy.max_age_days)}
						</span>
						{#if !policy.enabled}
							<span class="rounded bg-yellow-500/20 px-1.5 py-0.5 text-xs text-yellow-400">Disabled</span>
						{/if}
					</div>
					<div class="mt-1 flex flex-wrap gap-3 text-xs text-text-muted">
						<span>{policy.delete_attachments ? 'Deletes attachments' : 'Keeps attachments'}</span>
						<span>{policy.delete_pins ? 'Includes pinned' : 'Skips pinned'}</span>
						<span>{policy.messages_deleted.toLocaleString()} deleted</span>
						{#if policy.last_run_at}
							<span>Last run: {formatDate(policy.last_run_at)}</span>
						{/if}
					</div>
				</div>
				<div class="flex items-center gap-2">
					<button
						class="rounded px-2 py-1 text-xs transition-colors {policy.enabled ? 'bg-green-500/20 text-green-400 hover:bg-green-500/30' : 'bg-bg-modifier text-text-muted hover:bg-bg-modifier/80'}"
						onclick={() => handleToggle(policy)}
					>
						{policy.enabled ? 'Enabled' : 'Disabled'}
					</button>
					<button
						class="text-xs text-red-400 transition-colors hover:text-red-300"
						onclick={() => handleDelete(policy.id)}
					>
						Delete
					</button>
				</div>
			</div>
		{/each}
	</div>
{/if}
