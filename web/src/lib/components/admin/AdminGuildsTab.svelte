<script lang="ts">
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { confirmAction } from '$lib/stores/confirm';
	import { createAsyncOp } from '$lib/utils/asyncOp';
	import { getErrorMessage } from '$lib/utils/apiError';
	import Avatar from '$components/common/Avatar.svelte';
	import Modal from '$components/common/Modal.svelte';

	interface AdminGuild {
		id: string;
		name: string;
		icon_url: string | null;
		owner_id: string;
		owner_name: string;
		member_count: number;
		channel_count: number;
		role_count: number;
		created_at: string;
	}

	interface AdminGuildDetail extends AdminGuild {
		description: string | null;
		emoji_count: number;
		invite_count: number;
		message_count: number;
		messages_today: number;
		ban_count: number;
	}

	let adminGuilds = $state<AdminGuild[]>([]);
	let guildSearch = $state('');
	let guildSort = $state('newest');
	let guildSearchTimeout: ReturnType<typeof setTimeout> | null = null;
	let selectedGuildDetail = $state<AdminGuildDetail | null>(null);
	let guildDetailModalOpen = $state(false);
	let loadOp = $state(createAsyncOp());
	let detailOp = $state(createAsyncOp());

	$effect(() => {
		if (adminGuilds.length === 0 && !loadOp.loading) {
			loadGuilds();
		}
	});

	async function loadGuilds() {
		const result = await loadOp.run(() => api.getAdminGuilds({ query: guildSearch, sort: guildSort, limit: 100 }), msg => addToast(msg, 'error'), 'Failed to load servers');
		if (result) adminGuilds = result;
	}

	function handleGuildSearch() {
		if (guildSearchTimeout) clearTimeout(guildSearchTimeout);
		guildSearchTimeout = setTimeout(() => loadGuilds(), 300);
	}

	async function viewGuildDetail(guildId: string) {
		guildDetailModalOpen = true;
		const result = await detailOp.run(() => api.getAdminGuildDetails(guildId), msg => addToast(msg, 'error'), 'Failed to load server details');
		if (result) {
			selectedGuildDetail = result;
		} else {
			guildDetailModalOpen = false;
		}
	}

	async function handleDeleteGuild(guildId: string, guildName: string) {
		if (!(await confirmAction({ title: 'Delete Server', message: `Are you sure you want to delete "${guildName}"? This action is irreversible.`, confirmLabel: 'Delete Server' }))) return;
		try {
			await api.adminDeleteGuild(guildId);
			adminGuilds = adminGuilds.filter((guild) => guild.id !== guildId);
			guildDetailModalOpen = false;
			addToast(`Server "${guildName}" deleted.`, 'success');
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to delete server'), 'error');
		}
	}
</script>

<div class="mb-6 flex items-center justify-between">
	<h1 class="text-2xl font-bold text-text-primary">Server Management</h1>
	<button class="btn-secondary text-sm" onclick={loadGuilds} disabled={loadOp.loading}>
		{loadOp.loading ? 'Loading...' : 'Refresh'}
	</button>
</div>
<div class="mb-4 flex gap-3">
	<input
		type="search"
		class="input flex-1"
		aria-label="Search servers"
		placeholder="Search servers..."
		bind:value={guildSearch}
		oninput={handleGuildSearch}
	/>
	<select class="input w-40" bind:value={guildSort} aria-label="Sort servers" onchange={loadGuilds}>
		<option value="newest">Newest</option>
		<option value="oldest">Oldest</option>
		<option value="name">Name A-Z</option>
		<option value="members">Most Members</option>
	</select>
</div>

{#if loadOp.loading}
	<p class="text-sm text-text-muted">Loading servers...</p>
{:else if adminGuilds.length === 0}
	<p class="text-sm text-text-muted">No servers found.</p>
{:else}
	<div class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
		{#each adminGuilds as guild (guild.id)}
			<div class="rounded-lg bg-bg-secondary p-4">
				<div class="mb-3 flex items-center gap-3">
					<Avatar src={guild.icon_url} name={guild.name} size="md" />
					<div class="min-w-0">
						<h3 class="truncate font-semibold text-text-primary">{guild.name}</h3>
						<p class="text-xs text-text-muted">Owner: @{guild.owner_name}</p>
					</div>
				</div>
				<div class="mb-3 grid grid-cols-3 gap-2 text-center text-xs">
					<div class="rounded bg-bg-primary p-2"><div class="font-bold text-text-primary">{guild.member_count}</div><div class="text-text-muted">Members</div></div>
					<div class="rounded bg-bg-primary p-2"><div class="font-bold text-text-primary">{guild.channel_count}</div><div class="text-text-muted">Channels</div></div>
					<div class="rounded bg-bg-primary p-2"><div class="font-bold text-text-primary">{guild.role_count}</div><div class="text-text-muted">Roles</div></div>
				</div>
				<div class="flex gap-2">
					<button class="btn-secondary flex-1 text-sm" onclick={() => viewGuildDetail(guild.id)}>Details</button>
					<button class="rounded bg-red-500/10 px-3 py-1.5 text-sm text-red-400 hover:bg-red-500/20" onclick={() => handleDeleteGuild(guild.id, guild.name)}>Delete</button>
				</div>
			</div>
		{/each}
	</div>
{/if}

<Modal open={guildDetailModalOpen} title="Server Details" onclose={() => (guildDetailModalOpen = false)}>
	{#if detailOp.loading}
		<p class="text-sm text-text-muted">Loading server details...</p>
	{:else if selectedGuildDetail}
		<div class="space-y-4">
			<div class="flex items-center gap-3">
				{#if selectedGuildDetail.icon_url}
					<img src={selectedGuildDetail.icon_url} alt="" class="h-12 w-12 rounded-full object-cover" />
				{:else}
					<div class="flex h-12 w-12 items-center justify-center rounded-full bg-brand-500/20 text-lg font-bold text-brand-400">
						{selectedGuildDetail.name.charAt(0).toUpperCase()}
					</div>
				{/if}
				<div>
					<h3 class="text-lg font-bold text-text-primary">{selectedGuildDetail.name}</h3>
					{#if selectedGuildDetail.description}
						<p class="text-sm text-text-muted">{selectedGuildDetail.description}</p>
					{/if}
				</div>
			</div>

			<div class="grid grid-cols-2 gap-3">
				<div class="rounded-lg bg-bg-modifier/30 p-3">
					<p class="text-xs text-text-muted">Members</p>
					<p class="text-lg font-bold text-text-primary">{selectedGuildDetail.member_count.toLocaleString()}</p>
				</div>
				<div class="rounded-lg bg-bg-modifier/30 p-3">
					<p class="text-xs text-text-muted">Channels</p>
					<p class="text-lg font-bold text-text-primary">{selectedGuildDetail.channel_count.toLocaleString()}</p>
				</div>
				<div class="rounded-lg bg-bg-modifier/30 p-3">
					<p class="text-xs text-text-muted">Roles</p>
					<p class="text-lg font-bold text-text-primary">{selectedGuildDetail.role_count.toLocaleString()}</p>
				</div>
				<div class="rounded-lg bg-bg-modifier/30 p-3">
					<p class="text-xs text-text-muted">Emojis</p>
					<p class="text-lg font-bold text-text-primary">{selectedGuildDetail.emoji_count.toLocaleString()}</p>
				</div>
				<div class="rounded-lg bg-bg-modifier/30 p-3">
					<p class="text-xs text-text-muted">Total Messages</p>
					<p class="text-lg font-bold text-text-primary">{selectedGuildDetail.message_count.toLocaleString()}</p>
				</div>
				<div class="rounded-lg bg-bg-modifier/30 p-3">
					<p class="text-xs text-text-muted">Messages Today</p>
					<p class="text-lg font-bold text-text-primary">{selectedGuildDetail.messages_today.toLocaleString()}</p>
				</div>
				<div class="rounded-lg bg-bg-modifier/30 p-3">
					<p class="text-xs text-text-muted">Active Invites</p>
					<p class="text-lg font-bold text-text-primary">{selectedGuildDetail.invite_count.toLocaleString()}</p>
				</div>
				<div class="rounded-lg bg-bg-modifier/30 p-3">
					<p class="text-xs text-text-muted">Bans</p>
					<p class="text-lg font-bold text-text-primary">{selectedGuildDetail.ban_count.toLocaleString()}</p>
				</div>
			</div>

			<div class="space-y-1 text-xs text-text-muted">
				<p>Owner: <span class="text-text-primary">{selectedGuildDetail.owner_name}</span></p>
				<p>ID: <span class="font-mono text-text-primary">{selectedGuildDetail.id}</span></p>
				<p>Created: <span class="text-text-primary">{new Date(selectedGuildDetail.created_at).toLocaleString()}</span></p>
			</div>

			<div class="flex justify-end gap-2 pt-2">
				<button class="btn-secondary text-sm" onclick={() => (guildDetailModalOpen = false)}>Close</button>
				<button
					class="rounded bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-500"
					onclick={() => handleDeleteGuild(selectedGuildDetail!.id, selectedGuildDetail!.name)}
				>
					Delete Server
				</button>
			</div>
		</div>
	{/if}
</Modal>
