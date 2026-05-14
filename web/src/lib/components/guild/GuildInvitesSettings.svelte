<script lang="ts">
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import type { Invite } from '$lib/types';

	interface Props {
		guildId: string;
	}

	let { guildId }: Props = $props();

	let invites = $state<Invite[]>([]);
	let loadingInvites = $state(false);
	let creatingInvite = $state(false);
	let newInviteMaxUses = $state(0);
	let newInviteExpiry = $state(86400);
	let loadedGuildId = $state<string | null>(null);

	const expiryOptions = [
		{ label: '30 minutes', value: 1800 },
		{ label: '1 hour', value: 3600 },
		{ label: '6 hours', value: 21600 },
		{ label: '12 hours', value: 43200 },
		{ label: '1 day', value: 86400 },
		{ label: '7 days', value: 604800 },
		{ label: 'Never', value: 0 }
	];

	$effect(() => {
		if (guildId && loadedGuildId !== guildId && !loadingInvites) {
			loadInvites();
		}
	});

	async function loadInvites() {
		loadingInvites = true;
		try {
			invites = await api.getGuildInvites(guildId);
			loadedGuildId = guildId;
		} catch {
			invites = [];
		} finally {
			loadingInvites = false;
		}
	}

	async function handleCreateInvite() {
		creatingInvite = true;
		try {
			const opts: { max_uses?: number; max_age_seconds?: number } = {};
			if (newInviteMaxUses > 0) opts.max_uses = newInviteMaxUses;
			if (newInviteExpiry > 0) opts.max_age_seconds = newInviteExpiry;
			const invite = await api.createInvite(guildId, opts);
			invites = [invite, ...invites];
			addToast('Invite created', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to create invite', 'error');
		} finally {
			creatingInvite = false;
		}
	}

	async function handleRevokeInvite(code: string) {
		try {
			await api.deleteInvite(code);
			invites = invites.filter((invite) => invite.code !== code);
			addToast('Invite revoked', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to revoke invite', 'error');
		}
	}

	function copyInviteLink(code: string) {
		navigator.clipboard.writeText(`${window.location.origin}/invite/${code}`).then(
			() => addToast('Invite link copied', 'success'),
			() => addToast('Failed to copy invite link', 'error')
		);
	}

	function formatRelative(iso: string | null): string {
		if (!iso) return 'Never';
		const diff = new Date(iso).getTime() - Date.now();
		if (diff <= 0) return 'Expired';
		const hours = Math.floor(diff / 3600000);
		if (hours < 1) return `${Math.floor(diff / 60000)}m`;
		if (hours < 24) return `${hours}h`;
		return `${Math.floor(hours / 24)}d`;
	}
</script>

<h1 class="mb-6 text-xl font-bold text-text-primary">Invites</h1>

<div class="mb-6 rounded-lg bg-bg-secondary p-4">
	<h3 class="mb-3 text-sm font-semibold text-text-primary">Create Invite</h3>
	<div class="mb-3 flex gap-4">
		<div class="flex-1">
			<label for="newInviteExpiry" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Expire After</label>
			<select id="newInviteExpiry" bind:value={newInviteExpiry} class="input w-full">
				{#each expiryOptions as opt}
					<option value={opt.value}>{opt.label}</option>
				{/each}
			</select>
		</div>
		<div class="flex-1">
			<label for="newInviteMaxUses" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Max Uses (0 = unlimited)</label>
			<input id="newInviteMaxUses" type="number" min="0" max="100" bind:value={newInviteMaxUses} class="input w-full" />
		</div>
	</div>
	<button class="btn-primary" onclick={handleCreateInvite} disabled={creatingInvite}>
		{creatingInvite ? 'Creating...' : 'Create Invite'}
	</button>
</div>

{#if loadingInvites}
	<p class="text-sm text-text-muted">Loading invites...</p>
{:else if invites.length === 0}
	<p class="text-sm text-text-muted">No active invites.</p>
{:else}
	<div class="space-y-2">
		{#each invites as invite (invite.code)}
			<div class="flex items-center justify-between rounded-lg bg-bg-secondary p-3">
				<div>
					<code class="text-sm font-medium text-text-primary">{invite.code}</code>
					<p class="text-xs text-text-muted">
						Uses: {invite.uses}{invite.max_uses ? `/${invite.max_uses}` : ''} ·
						Expires: {formatRelative(invite.expires_at)}
					</p>
				</div>
				<div class="flex items-center gap-2">
					<button
						class="text-xs text-brand-400 hover:text-brand-300"
						onclick={() => copyInviteLink(invite.code)}
					>
						Copy Link
					</button>
					<button
						class="text-xs text-red-400 hover:text-red-300"
						onclick={() => handleRevokeInvite(invite.code)}
					>
						Revoke
					</button>
				</div>
			</div>
		{/each}
	</div>
{/if}
