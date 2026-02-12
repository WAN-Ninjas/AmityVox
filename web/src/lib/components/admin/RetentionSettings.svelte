<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';

	interface RetentionPolicy {
		id: string;
		channel_id: string | null;
		guild_id: string | null;
		max_age_days: number;
		delete_attachments: boolean;
		delete_pins: boolean;
		enabled: boolean;
		last_run_at: string | null;
		next_run_at: string | null;
		messages_deleted: number;
		created_by: string;
		created_at: string;
		updated_at: string;
		channel_name: string | null;
		guild_name: string | null;
		creator_name: string;
	}

	let loading = $state(true);
	let policies = $state<RetentionPolicy[]>([]);
	let creating = $state(false);
	let showCreateForm = $state(false);

	// Create form state
	let newScope = $state<'guild' | 'channel'>('guild');
	let newGuildId = $state('');
	let newChannelId = $state('');
	let newMaxAgeDays = $state(90);
	let newDeleteAttachments = $state(true);
	let newDeletePins = $state(false);
	let newEnabled = $state(true);

	// Running state
	let runningPolicyId = $state('');

	async function loadPolicies() {
		loading = true;
		try {
			const res = await fetch('/api/v1/admin/retention', {
				headers: { 'Authorization': `Bearer ${api.getToken()}` }
			});
			const json = await res.json();
			if (res.ok) {
				policies = json.data || [];
			}
		} catch {
			addToast('Failed to load retention policies', 'error');
		}
		loading = false;
	}

	async function createPolicy() {
		creating = true;
		try {
			const body: Record<string, unknown> = {
				max_age_days: newMaxAgeDays,
				delete_attachments: newDeleteAttachments,
				delete_pins: newDeletePins,
				enabled: newEnabled
			};
			if (newScope === 'guild' && newGuildId) {
				body.guild_id = newGuildId;
			} else if (newScope === 'channel' && newChannelId) {
				body.channel_id = newChannelId;
			}

			const res = await fetch('/api/v1/admin/retention', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
					'Authorization': `Bearer ${api.getToken()}`
				},
				body: JSON.stringify(body)
			});
			const json = await res.json();
			if (res.ok) {
				addToast('Retention policy created', 'success');
				showCreateForm = false;
				resetForm();
				await loadPolicies();
			} else {
				addToast(json.error?.message || 'Failed to create policy', 'error');
			}
		} catch {
			addToast('Failed to create retention policy', 'error');
		}
		creating = false;
	}

	async function togglePolicy(policy: RetentionPolicy) {
		try {
			const res = await fetch(`/api/v1/admin/retention/${policy.id}`, {
				method: 'PATCH',
				headers: {
					'Content-Type': 'application/json',
					'Authorization': `Bearer ${api.getToken()}`
				},
				body: JSON.stringify({ enabled: !policy.enabled })
			});
			if (res.ok) {
				policy.enabled = !policy.enabled;
				policies = [...policies];
				addToast(`Policy ${policy.enabled ? 'enabled' : 'disabled'}`, 'success');
			}
		} catch {
			addToast('Failed to update policy', 'error');
		}
	}

	async function deletePolicy(policyId: string) {
		if (!confirm('Delete this retention policy? This action cannot be undone.')) return;
		try {
			const res = await fetch(`/api/v1/admin/retention/${policyId}`, {
				method: 'DELETE',
				headers: { 'Authorization': `Bearer ${api.getToken()}` }
			});
			if (res.ok) {
				policies = policies.filter(p => p.id !== policyId);
				addToast('Retention policy deleted', 'success');
			}
		} catch {
			addToast('Failed to delete policy', 'error');
		}
	}

	async function runPolicy(policyId: string) {
		if (!confirm('Run this retention policy now? Messages older than the retention period will be permanently deleted.')) return;
		runningPolicyId = policyId;
		try {
			const res = await fetch(`/api/v1/admin/retention/${policyId}/run`, {
				method: 'POST',
				headers: { 'Authorization': `Bearer ${api.getToken()}` }
			});
			const json = await res.json();
			if (res.ok) {
				addToast(`Deleted ${json.data?.messages_deleted || 0} messages`, 'success');
				await loadPolicies();
			} else {
				addToast(json.error?.message || 'Failed to run policy', 'error');
			}
		} catch {
			addToast('Failed to run retention policy', 'error');
		}
		runningPolicyId = '';
	}

	function resetForm() {
		newScope = 'guild';
		newGuildId = '';
		newChannelId = '';
		newMaxAgeDays = 90;
		newDeleteAttachments = true;
		newDeletePins = false;
		newEnabled = true;
	}

	function formatDate(date: string | null): string {
		if (!date) return 'Never';
		return new Date(date).toLocaleDateString(undefined, {
			year: 'numeric', month: 'short', day: 'numeric',
			hour: '2-digit', minute: '2-digit'
		});
	}

	function scopeLabel(policy: RetentionPolicy): string {
		if (policy.channel_name) return `Channel: ${policy.channel_name}`;
		if (policy.guild_name) return `Guild: ${policy.guild_name}`;
		return 'Instance-wide';
	}

	onMount(() => {
		loadPolicies();
	});
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<div>
			<h2 class="text-xl font-bold text-text-primary">Data Retention</h2>
			<p class="text-text-muted text-sm">Auto-delete messages older than a configurable threshold</p>
		</div>
		<button
			class="btn-primary text-sm px-4 py-2"
			onclick={() => showCreateForm = !showCreateForm}
		>
			{showCreateForm ? 'Cancel' : 'Create Policy'}
		</button>
	</div>

	<!-- Warning -->
	<div class="bg-status-idle/10 border border-status-idle/30 rounded-lg p-4">
		<div class="flex gap-3">
			<svg class="w-5 h-5 text-status-idle flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L4.082 16.5c-.77.833.192 2.5 1.732 2.5z" />
			</svg>
			<p class="text-sm text-text-secondary">
				Retention policies permanently delete messages. Deleted messages cannot be recovered.
				Pinned messages are preserved by default unless explicitly included.
			</p>
		</div>
	</div>

	<!-- Create Form -->
	{#if showCreateForm}
		<div class="bg-bg-tertiary rounded-lg p-5 border border-bg-modifier">
			<h3 class="text-sm font-semibold text-text-secondary mb-4">New Retention Policy</h3>
			<div class="space-y-4">
				<div>
					<label class="block text-sm font-medium text-text-secondary mb-2">Scope</label>
					<div class="flex gap-4">
						<label class="flex items-center gap-2 text-sm text-text-primary">
							<input type="radio" bind:group={newScope} value="guild" />
							Guild
						</label>
						<label class="flex items-center gap-2 text-sm text-text-primary">
							<input type="radio" bind:group={newScope} value="channel" />
							Channel
						</label>
					</div>
				</div>

				{#if newScope === 'guild'}
					<div>
						<label for="ret-guild-id" class="block text-sm font-medium text-text-secondary mb-1">Guild ID</label>
						<input id="ret-guild-id" type="text" class="input w-full" placeholder="Paste guild ULID" bind:value={newGuildId} />
					</div>
				{:else}
					<div>
						<label for="ret-channel-id" class="block text-sm font-medium text-text-secondary mb-1">Channel ID</label>
						<input id="ret-channel-id" type="text" class="input w-full" placeholder="Paste channel ULID" bind:value={newChannelId} />
					</div>
				{/if}

				<div>
					<label for="ret-max-age" class="block text-sm font-medium text-text-secondary mb-1">
						Delete messages older than (days)
					</label>
					<input id="ret-max-age" type="number" class="input w-32" min="1" max="36500" bind:value={newMaxAgeDays} />
					<p class="text-text-muted text-xs mt-1">
						{#if newMaxAgeDays <= 30}
							Aggressive: messages older than {newMaxAgeDays} days will be deleted.
						{:else if newMaxAgeDays <= 365}
							Moderate: messages older than {newMaxAgeDays} days ({Math.round(newMaxAgeDays / 30)} months) will be deleted.
						{:else}
							Conservative: messages older than {newMaxAgeDays} days ({Math.round(newMaxAgeDays / 365)} years) will be deleted.
						{/if}
					</p>
				</div>

				<div class="flex gap-6">
					<label class="flex items-center gap-2 text-sm text-text-primary">
						<input type="checkbox" bind:checked={newDeleteAttachments} />
						Delete attachments
					</label>
					<label class="flex items-center gap-2 text-sm text-text-primary">
						<input type="checkbox" bind:checked={newDeletePins} />
						Include pinned messages
					</label>
					<label class="flex items-center gap-2 text-sm text-text-primary">
						<input type="checkbox" bind:checked={newEnabled} />
						Enable immediately
					</label>
				</div>

				<div class="flex justify-end gap-3">
					<button class="btn-secondary px-4 py-2 text-sm" onclick={() => { showCreateForm = false; resetForm(); }}>
						Cancel
					</button>
					<button class="btn-primary px-4 py-2 text-sm" onclick={createPolicy} disabled={creating}>
						{creating ? 'Creating...' : 'Create Policy'}
					</button>
				</div>
			</div>
		</div>
	{/if}

	<!-- Policies List -->
	{#if loading && policies.length === 0}
		<div class="flex justify-center py-12">
			<div class="animate-spin w-8 h-8 border-2 border-brand-500 border-t-transparent rounded-full"></div>
		</div>
	{:else if policies.length === 0}
		<div class="bg-bg-tertiary rounded-lg p-8 text-center">
			<p class="text-text-muted">No retention policies configured.</p>
			<p class="text-text-muted text-sm mt-1">Messages will be kept indefinitely until a policy is created.</p>
		</div>
	{:else}
		<div class="space-y-3">
			{#each policies as policy}
				<div class="bg-bg-tertiary rounded-lg p-4">
					<div class="flex items-start justify-between">
						<div class="flex-1">
							<div class="flex items-center gap-2 mb-1">
								<span class="text-text-primary font-medium text-sm">{scopeLabel(policy)}</span>
								<span class="text-xs px-2 py-0.5 rounded-full {policy.enabled ? 'bg-status-online/20 text-status-online' : 'bg-bg-modifier text-text-muted'}">
									{policy.enabled ? 'Active' : 'Disabled'}
								</span>
							</div>
							<div class="grid grid-cols-2 md:grid-cols-4 gap-x-4 gap-y-1 text-sm mt-2">
								<div>
									<span class="text-text-muted">Max Age:</span>
									<span class="text-text-secondary ml-1">{policy.max_age_days} days</span>
								</div>
								<div>
									<span class="text-text-muted">Deleted:</span>
									<span class="text-text-secondary ml-1">{policy.messages_deleted.toLocaleString()} msgs</span>
								</div>
								<div>
									<span class="text-text-muted">Last Run:</span>
									<span class="text-text-secondary ml-1">{formatDate(policy.last_run_at)}</span>
								</div>
								<div>
									<span class="text-text-muted">Next Run:</span>
									<span class="text-text-secondary ml-1">{formatDate(policy.next_run_at)}</span>
								</div>
							</div>
							<div class="flex gap-3 mt-1 text-xs text-text-muted">
								{#if policy.delete_attachments}<span>Deletes attachments</span>{/if}
								{#if policy.delete_pins}<span>Includes pinned</span>{/if}
								<span>Created by {policy.creator_name}</span>
							</div>
						</div>
						<div class="flex items-center gap-2 ml-4">
							<button
								class="btn-secondary text-xs px-3 py-1"
								onclick={() => runPolicy(policy.id)}
								disabled={runningPolicyId === policy.id}
								title="Run now"
							>
								{runningPolicyId === policy.id ? 'Running...' : 'Run Now'}
							</button>
							<button
								class="text-xs px-3 py-1 rounded {policy.enabled ? 'bg-status-idle/20 text-status-idle hover:bg-status-idle/30' : 'bg-status-online/20 text-status-online hover:bg-status-online/30'} transition-colors"
								onclick={() => togglePolicy(policy)}
							>
								{policy.enabled ? 'Disable' : 'Enable'}
							</button>
							<button
								class="text-xs px-3 py-1 rounded bg-status-dnd/20 text-status-dnd hover:bg-status-dnd/30 transition-colors"
								onclick={() => deletePolicy(policy.id)}
							>
								Delete
							</button>
						</div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
