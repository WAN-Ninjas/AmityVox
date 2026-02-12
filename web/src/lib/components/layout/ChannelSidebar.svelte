<script lang="ts">
	import { currentGuild, currentGuildId } from '$lib/stores/guilds';
	import { textChannels, voiceChannels, currentChannelId, setChannel } from '$lib/stores/channels';
	import { currentUser } from '$lib/stores/auth';
	import Avatar from '$components/common/Avatar.svelte';
	import Modal from '$components/common/Modal.svelte';
	import { presenceMap } from '$lib/stores/presence';
	import { dmList } from '$lib/stores/dms';
	import { unreadCounts } from '$lib/stores/unreads';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import InviteModal from '$components/guild/InviteModal.svelte';

	// Create channel modal
	let showCreateChannel = $state(false);
	let newChannelName = $state('');
	let newChannelType = $state<'text' | 'voice'>('text');
	let creatingChannel = $state(false);
	let channelError = $state('');

	// Edit channel modal
	let showEditChannel = $state(false);
	let editChannelId = $state('');
	let editChannelName = $state('');
	let editChannelTopic = $state('');
	let editingChannel = $state(false);

	// Invite modal
	let showInvite = $state(false);

	// Context menu
	let contextMenu = $state<{ x: number; y: number; channelId: string; channelName: string } | null>(null);

	function handleChannelClick(channelId: string) {
		const guildId = $currentGuildId;
		if (guildId) {
			goto(`/app/guilds/${guildId}/channels/${channelId}`);
		}
	}

	async function handleCreateChannel() {
		const guildId = $currentGuildId;
		if (!guildId || !newChannelName.trim()) return;
		creatingChannel = true;
		channelError = '';
		try {
			await api.createChannel(guildId, newChannelName.trim(), newChannelType);
			showCreateChannel = false;
			newChannelName = '';
			newChannelType = 'text';
		} catch (err: any) {
			channelError = err.message || 'Failed to create channel';
		} finally {
			creatingChannel = false;
		}
	}

	async function handleEditChannel() {
		if (!editChannelId || !editChannelName.trim()) return;
		editingChannel = true;
		channelError = '';
		try {
			await api.updateChannel(editChannelId, {
				name: editChannelName.trim(),
				topic: editChannelTopic || undefined
			} as any);
			showEditChannel = false;
		} catch (err: any) {
			channelError = err.message || 'Failed to update channel';
		} finally {
			editingChannel = false;
		}
	}

	async function handleDeleteChannel(channelId: string) {
		if (!confirm('Are you sure you want to delete this channel?')) return;
		try {
			await api.deleteChannel(channelId);
		} catch (err: any) {
			alert(err.message || 'Failed to delete channel');
		}
	}

	function openContextMenu(e: MouseEvent, channelId: string, channelName: string) {
		e.preventDefault();
		contextMenu = { x: e.clientX, y: e.clientY, channelId, channelName };
	}

	function closeContextMenu() {
		contextMenu = null;
	}

	function openEditModal(channelId: string, channelName: string) {
		editChannelId = channelId;
		editChannelName = channelName;
		editChannelTopic = '';
		channelError = '';
		showEditChannel = true;
		closeContextMenu();
	}

	function channelTypeButtonClass(type: 'text' | 'voice'): string {
		const base = 'rounded-lg border-2 px-4 py-2 text-sm transition-colors';
		if (newChannelType === type) return `${base} border-brand-500 bg-brand-500/10 text-text-primary`;
		return `${base} border-bg-modifier text-text-muted`;
	}
</script>

<svelte:window onclick={closeContextMenu} />

<aside class="flex h-full w-60 shrink-0 flex-col bg-bg-secondary">
	<!-- Guild header -->
	{#if $currentGuild}
		<div class="flex h-12 items-center justify-between border-b border-bg-floating px-4">
			<h2 class="truncate text-sm font-semibold text-text-primary">{$currentGuild.name}</h2>
			<div class="flex items-center gap-1">
				<button
					class="text-text-muted hover:text-text-primary"
					onclick={() => (showInvite = true)}
					title="Create Invite"
				>
					<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M18 9v3m0 0v3m0-3h3m-3 0h-3m-2-5a4 4 0 11-8 0 4 4 0 018 0zM3 20a6 6 0 0112 0v1H3v-1z" />
					</svg>
				</button>
				<button
					class="text-text-muted hover:text-text-primary"
					onclick={() => goto(`/app/guilds/${$currentGuild?.id}/settings`)}
					title="Guild Settings"
				>
					<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M12 6V4m0 2a2 2 0 100 4m0-4a2 2 0 110 4m-6 8a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4m6 6v10m6-2a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4" />
					</svg>
				</button>
			</div>
		</div>
	{:else}
		<div class="flex h-12 items-center border-b border-bg-floating px-4">
			<h2 class="text-sm font-semibold text-text-primary">Direct Messages</h2>
		</div>
	{/if}

	<!-- Channel list -->
	<div class="flex-1 overflow-y-auto px-2 py-2">
		{#if $currentGuild}
			<!-- Text Channels -->
			{#if $textChannels.length > 0 || $currentGuild}
				<div class="mb-1 flex items-center justify-between px-1 pt-4 first:pt-0">
					<h3 class="text-2xs font-bold uppercase tracking-wide text-text-muted">Text Channels</h3>
					<button
						class="text-text-muted hover:text-text-primary"
						onclick={() => { newChannelType = 'text'; showCreateChannel = true; channelError = ''; }}
						title="Create Text Channel"
					>
						<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M12 5v14m-7-7h14" />
						</svg>
					</button>
				</div>
				{#each $textChannels as channel (channel.id)}
					{@const unread = $unreadCounts.get(channel.id) ?? 0}
					<button
						class="mb-0.5 flex w-full items-center gap-1.5 rounded px-2 py-1.5 text-left text-sm transition-colors {$currentChannelId === channel.id ? 'bg-bg-modifier text-text-primary' : unread > 0 ? 'text-text-primary font-semibold hover:bg-bg-modifier' : 'text-text-muted hover:bg-bg-modifier hover:text-text-secondary'}"
						onclick={() => handleChannelClick(channel.id)}
						oncontextmenu={(e) => openContextMenu(e, channel.id, channel.name ?? '')}
					>
						<span class="text-lg leading-none">#</span>
						<span class="flex-1 truncate">{channel.name}</span>
						{#if unread > 0 && $currentChannelId !== channel.id}
							<span class="ml-auto flex h-4 min-w-4 items-center justify-center rounded-full bg-red-500 px-1 text-2xs font-bold text-white">
								{unread > 99 ? '99+' : unread}
							</span>
						{/if}
					</button>
				{/each}
			{/if}

			<!-- Voice Channels -->
			{#if $voiceChannels.length > 0 || $currentGuild}
				<div class="mb-1 flex items-center justify-between px-1 pt-4">
					<h3 class="text-2xs font-bold uppercase tracking-wide text-text-muted">Voice Channels</h3>
					<button
						class="text-text-muted hover:text-text-primary"
						onclick={() => { newChannelType = 'voice'; showCreateChannel = true; channelError = ''; }}
						title="Create Voice Channel"
					>
						<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M12 5v14m-7-7h14" />
						</svg>
					</button>
				</div>
				{#each $voiceChannels as channel (channel.id)}
					<button
						class="mb-0.5 flex w-full items-center gap-1.5 rounded px-2 py-1.5 text-left text-sm text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-secondary"
						onclick={() => handleChannelClick(channel.id)}
						oncontextmenu={(e) => openContextMenu(e, channel.id, channel.name ?? '')}
					>
						<svg class="h-4 w-4 shrink-0" fill="currentColor" viewBox="0 0 24 24">
							<path d="M12 3a1 1 0 0 0-1 1v8a3 3 0 1 0 6 0V4a1 1 0 1 0-2 0v8a1 1 0 1 1-2 0V4a1 1 0 0 0-1-1zM7 12a5 5 0 0 0 10 0h2a7 7 0 0 1-6 6.92V21h-2v-2.08A7 7 0 0 1 5 12h2z" />
						</svg>
						<span class="truncate">{channel.name}</span>
					</button>
				{/each}
			{/if}
		{:else}
			<!-- DM List (when no guild is selected) -->
			<div class="mb-1 flex items-center justify-between px-1">
				<button
					class="mb-0.5 flex w-full items-center gap-1.5 rounded px-2 py-1.5 text-left text-sm text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-secondary"
					onclick={() => goto('/app/friends')}
				>
					<svg class="h-4 w-4 shrink-0" fill="currentColor" viewBox="0 0 24 24">
						<path d="M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z" />
					</svg>
					<span>Friends</span>
				</button>
			</div>

			<div class="mb-1 flex items-center justify-between px-1 pt-2">
				<h3 class="text-2xs font-bold uppercase tracking-wide text-text-muted">Direct Messages</h3>
			</div>

			{#if $dmList.length === 0}
				<p class="px-2 py-2 text-xs text-text-muted">No conversations yet.</p>
			{:else}
				{#each $dmList as dm (dm.id)}
					{@const dmUnread = $unreadCounts.get(dm.id) ?? 0}
					{@const dmName = dm.name ?? 'Direct Message'}
					<button
						class="mb-0.5 flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm transition-colors {$currentChannelId === dm.id ? 'bg-bg-modifier text-text-primary' : dmUnread > 0 ? 'text-text-primary font-semibold hover:bg-bg-modifier' : 'text-text-muted hover:bg-bg-modifier hover:text-text-secondary'}"
						onclick={() => goto(`/app/dms/${dm.id}`)}
					>
						<Avatar name={dmName} size="sm" status={dm.owner_id ? ($presenceMap.get(dm.owner_id) ?? undefined) : undefined} />
						<span class="flex-1 truncate">{dmName}</span>
						{#if dmUnread > 0}
							<span class="ml-auto flex h-4 min-w-4 items-center justify-center rounded-full bg-red-500 px-1 text-2xs font-bold text-white">
								{dmUnread > 99 ? '99+' : dmUnread}
							</span>
						{/if}
					</button>
				{/each}
			{/if}
		{/if}
	</div>

	<!-- User panel (bottom) -->
	{#if $currentUser}
		{@const myStatus = $presenceMap.get($currentUser.id) ?? $currentUser.status_presence ?? 'online'}
		<div class="flex items-center gap-2 border-t border-bg-floating bg-bg-primary/50 p-2">
			<Avatar name={$currentUser.display_name ?? $currentUser.username} size="sm" status={myStatus} />
			<div class="min-w-0 flex-1">
				<p class="truncate text-sm font-medium text-text-primary">
					{$currentUser.display_name ?? $currentUser.username}
				</p>
				<p class="truncate text-xs text-text-muted">
					{$currentUser.status_text ?? myStatus}
				</p>
			</div>
			<button
				class="text-text-muted hover:text-text-primary"
				onclick={() => goto('/app/settings')}
				title="User Settings"
			>
				<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
					<circle cx="12" cy="12" r="3" />
				</svg>
			</button>
		</div>
	{/if}
</aside>

<!-- Context menu -->
{#if contextMenu}
	<div
		class="fixed z-50 min-w-[160px] rounded-md bg-bg-floating p-1 shadow-lg"
		style="left: {contextMenu.x}px; top: {contextMenu.y}px;"
	>
		<button
			class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
			onclick={() => openEditModal(contextMenu!.channelId, contextMenu!.channelName)}
		>
			<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
			</svg>
			Edit Channel
		</button>
		<button
			class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-red-400 hover:bg-red-500 hover:text-white"
			onclick={() => { handleDeleteChannel(contextMenu!.channelId); closeContextMenu(); }}
		>
			<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
			</svg>
			Delete Channel
		</button>
	</div>
{/if}

<!-- Invite Modal -->
<InviteModal bind:open={showInvite} onclose={() => (showInvite = false)} />

<!-- Create Channel Modal -->
<Modal open={showCreateChannel} title="Create Channel" onclose={() => (showCreateChannel = false)}>
	{#if channelError}
		<div class="mb-4 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{channelError}</div>
	{/if}

	<div class="mb-4">
		<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Channel Type</label>
		<div class="flex gap-2">
			<button
				class={channelTypeButtonClass('text')}
				onclick={() => (newChannelType = 'text')}
			>
				# Text
			</button>
			<button
				class={channelTypeButtonClass('voice')}
				onclick={() => (newChannelType = 'voice')}
			>
				Voice
			</button>
		</div>
	</div>

	<div class="mb-4">
		<label for="channelName" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
			Channel Name
		</label>
		<input
			id="channelName"
			type="text"
			class="input w-full"
			bind:value={newChannelName}
			placeholder={newChannelType === 'text' ? 'new-channel' : 'General'}
			maxlength="100"
			onkeydown={(e) => e.key === 'Enter' && handleCreateChannel()}
		/>
	</div>

	<div class="flex justify-end gap-2">
		<button class="btn-secondary" onclick={() => (showCreateChannel = false)}>Cancel</button>
		<button class="btn-primary" onclick={handleCreateChannel} disabled={creatingChannel || !newChannelName.trim()}>
			{creatingChannel ? 'Creating...' : 'Create'}
		</button>
	</div>
</Modal>

<!-- Edit Channel Modal -->
<Modal open={showEditChannel} title="Edit Channel" onclose={() => (showEditChannel = false)}>
	{#if channelError}
		<div class="mb-4 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{channelError}</div>
	{/if}

	<div class="mb-4">
		<label for="editName" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
			Channel Name
		</label>
		<input
			id="editName"
			type="text"
			class="input w-full"
			bind:value={editChannelName}
			maxlength="100"
		/>
	</div>

	<div class="mb-4">
		<label for="editTopic" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
			Topic
		</label>
		<input
			id="editTopic"
			type="text"
			class="input w-full"
			bind:value={editChannelTopic}
			placeholder="Set a channel topic"
			maxlength="1024"
		/>
	</div>

	<div class="flex justify-end gap-2">
		<button class="btn-secondary" onclick={() => (showEditChannel = false)}>Cancel</button>
		<button class="btn-primary" onclick={handleEditChannel} disabled={editingChannel || !editChannelName.trim()}>
			{editingChannel ? 'Saving...' : 'Save'}
		</button>
	</div>
</Modal>
