<script lang="ts">
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import type { Channel, Role } from '$lib/types';

	let {
		channel,
		roles = [],
		onUpdate
	}: {
		channel: Channel;
		roles: Role[];
		onUpdate: (updated: Channel) => void;
	} = $props();

	// Read-only settings
	let readOnly = $state(false);
	let readOnlyRoleIds = $state<string[]>([]);

	// Auto-archive settings
	let autoArchiveDuration = $state(0);

	let saving = $state(false);

	// Initialize state from channel when it changes.
	$effect(() => {
		if (channel) {
			readOnly = (channel as any).read_only ?? false;
			readOnlyRoleIds = [...((channel as any).read_only_role_ids ?? [])];
			autoArchiveDuration = (channel as any).default_auto_archive_duration ?? 0;
		}
	});

	const archiveOptions = [
		{ label: 'Never', value: 0 },
		{ label: '1 Hour', value: 60 },
		{ label: '1 Day', value: 1440 },
		{ label: '3 Days', value: 4320 },
		{ label: '1 Week', value: 10080 }
	];

	function toggleRole(roleId: string) {
		if (readOnlyRoleIds.includes(roleId)) {
			readOnlyRoleIds = readOnlyRoleIds.filter(id => id !== roleId);
		} else {
			readOnlyRoleIds = [...readOnlyRoleIds, roleId];
		}
	}

	async function handleSave() {
		saving = true;
		try {
			const updated = await api.updateChannel(channel.id, {
				read_only: readOnly,
				read_only_role_ids: readOnlyRoleIds,
				default_auto_archive_duration: autoArchiveDuration
			} as any);
			onUpdate(updated);
			addToast('Channel settings updated', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to update channel settings', 'error');
		} finally {
			saving = false;
		}
	}
</script>

<div class="space-y-6">
	<!-- Read-Only Channel -->
	<div class="rounded-lg bg-bg-secondary p-4">
		<h3 class="mb-2 text-sm font-semibold text-text-primary">Read-Only Channel</h3>
		<p class="mb-3 text-xs text-text-muted">
			When enabled, only users with selected roles (or guild owners/admins) can send messages. Everyone else can only read.
		</p>
		<label class="mb-3 flex items-center gap-2">
			<input type="checkbox" bind:checked={readOnly} class="rounded" />
			<span class="text-sm text-text-primary">Enable read-only mode</span>
		</label>

		{#if readOnly}
			<div class="mt-3">
				<p class="mb-2 text-xs font-medium text-text-muted">Roles that can still post:</p>
				{#if roles.length === 0}
					<p class="text-xs text-text-muted">No roles available. Guild owner and admins can always post.</p>
				{:else}
					<div class="flex flex-wrap gap-2">
						{#each roles as role (role.id)}
							<button
								class="rounded px-2 py-1 text-xs transition-colors {readOnlyRoleIds.includes(role.id)
									? 'bg-brand-500/20 text-brand-400 ring-1 ring-brand-500/50'
									: 'bg-bg-modifier text-text-muted hover:bg-bg-modifier/80'}"
								onclick={() => toggleRole(role.id)}
							>
								{#if role.color}
									<span class="mr-1 inline-block h-2 w-2 rounded-full" style="background-color: {role.color}"></span>
								{/if}
								{role.name}
							</button>
						{/each}
					</div>
				{/if}
			</div>
		{/if}
	</div>

	<!-- Thread Auto-Archive Duration -->
	{#if channel.channel_type === 'text' || channel.channel_type === 'forum'}
		<div class="rounded-lg bg-bg-secondary p-4">
			<h3 class="mb-2 text-sm font-semibold text-text-primary">Thread Auto-Archive</h3>
			<p class="mb-3 text-xs text-text-muted">
				Threads will be automatically archived after this duration of inactivity (no new messages).
			</p>
			<select class="input w-full" bind:value={autoArchiveDuration}>
				{#each archiveOptions as opt (opt.value)}
					<option value={opt.value}>{opt.label}</option>
				{/each}
			</select>
		</div>
	{/if}

	<!-- Save Button -->
	<button
		class="btn-primary text-sm"
		onclick={handleSave}
		disabled={saving}
	>
		{saving ? 'Saving...' : 'Save Channel Settings'}
	</button>
</div>
