<script lang="ts">
	import { api } from '$lib/api/client';
	import { onMount } from 'svelte';
	import { currentGuildId } from '$lib/stores/guilds';
	import { textChannels, voiceChannels, currentChannelId } from '$lib/stores/channels';
	import { unreadCounts, mentionCounts } from '$lib/stores/unreads';
	import { goto } from '$app/navigation';
	import { addToast } from '$lib/stores/toast';

	interface ChannelGroup {
		id: string;
		user_id: string;
		name: string;
		position: number;
		color: string;
		channels: string[];
		created_at: string;
	}

	let groups = $state<ChannelGroup[]>([]);
	let loading = $state(true);
	let showCreateModal = $state(false);
	let newGroupName = $state('');
	let newGroupColor = $state('#5c6bc0');
	let creating = $state(false);

	// Edit state
	let editingGroupId = $state<string | null>(null);
	let editGroupName = $state('');
	let editGroupColor = $state('');

	// Collapsed state
	let collapsedGroups = $state<Set<string>>(new Set());

	function authHeaders(includeContentType = false): Record<string, string> {
		const token = api.getToken();
		const headers: Record<string, string> = {};
		if (token) headers['Authorization'] = `Bearer ${token}`;
		if (includeContentType) headers['Content-Type'] = 'application/json';
		return headers;
	}

	onMount(async () => {
		await loadGroups();
		// Restore collapsed state from localStorage.
		try {
			const stored = localStorage.getItem('amityvox_collapsed_channel_groups');
			if (stored) {
				const parsed = JSON.parse(stored);
				if (Array.isArray(parsed)) collapsedGroups = new Set(parsed);
			}
		} catch {
			// Ignore.
		}
	});

	async function loadGroups() {
		loading = true;
		try {
			const res = await fetch('/api/v1/users/@me/channel-groups', {
				headers: authHeaders()
			});
			if (!res.ok) {
				groups = [];
				return;
			}
			const data = await res.json();
			groups = data.data ?? [];
		} catch {
			groups = [];
		} finally {
			loading = false;
		}
	}

	function toggleGroup(groupId: string) {
		const next = new Set(collapsedGroups);
		if (next.has(groupId)) {
			next.delete(groupId);
		} else {
			next.add(groupId);
		}
		collapsedGroups = next;
		localStorage.setItem('amityvox_collapsed_channel_groups', JSON.stringify([...next]));
	}

	async function createGroup() {
		if (!newGroupName.trim()) return;
		creating = true;
		try {
			const res = await fetch('/api/v1/users/@me/channel-groups', {
				method: 'POST',
				headers: authHeaders(true),
				body: JSON.stringify({
					name: newGroupName.trim(),
					color: newGroupColor
				})
			});
			if (!res.ok) {
				const body = await res.json();
				throw new Error(body?.error?.message || 'Failed to create group');
			}
			const data = await res.json();
			const group = data.data ?? data;
			groups = [...groups, group];
			showCreateModal = false;
			newGroupName = '';
			newGroupColor = '#5c6bc0';
			addToast('Channel group created', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to create group', 'error');
		} finally {
			creating = false;
		}
	}

	async function updateGroup(groupId: string) {
		if (!editGroupName.trim()) return;
		try {
			const res = await fetch(`/api/v1/users/@me/channel-groups/${groupId}`, {
				method: 'PATCH',
				headers: authHeaders(true),
				body: JSON.stringify({
					name: editGroupName.trim(),
					color: editGroupColor
				})
			});
			if (!res.ok) {
				const body = await res.json();
				throw new Error(body?.error?.message || 'Failed to update group');
			}
			const data = await res.json();
			const updated = data.data ?? data;
			groups = groups.map(g => g.id === groupId ? updated : g);
			editingGroupId = null;
		} catch (err: any) {
			addToast(err.message || 'Failed to update group', 'error');
		}
	}

	async function deleteGroup(groupId: string) {
		if (!confirm('Delete this channel group?')) return;
		try {
			const res = await fetch(`/api/v1/users/@me/channel-groups/${groupId}`, {
				method: 'DELETE',
				headers: authHeaders()
			});
			if (!res.ok && res.status !== 204) {
				throw new Error('Failed to delete group');
			}
			groups = groups.filter(g => g.id !== groupId);
			addToast('Channel group deleted', 'info');
		} catch (err: any) {
			addToast(err.message || 'Failed to delete group', 'error');
		}
	}

	async function removeChannel(groupId: string, channelId: string) {
		try {
			const res = await fetch(`/api/v1/users/@me/channel-groups/${groupId}/channels/${channelId}`, {
				method: 'DELETE',
				headers: authHeaders()
			});
			if (!res.ok && res.status !== 204) {
				throw new Error('Failed to remove channel');
			}
			groups = groups.map(g => {
				if (g.id === groupId) {
					return { ...g, channels: g.channels.filter(c => c !== channelId) };
				}
				return g;
			});
		} catch (err: any) {
			addToast(err.message || 'Failed to remove channel', 'error');
		}
	}

	function handleChannelClick(channelId: string) {
		const guildId = $currentGuildId;
		if (guildId) {
			goto(`/app/guilds/${guildId}/channels/${channelId}`);
		}
	}

	function getChannelName(channelId: string): string {
		const allChannels = [...$textChannels, ...$voiceChannels];
		const channel = allChannels.find(c => c.id === channelId);
		return channel?.name ?? 'Unknown Channel';
	}

	function getChannelType(channelId: string): string {
		const allChannels = [...$textChannels, ...$voiceChannels];
		const channel = allChannels.find(c => c.id === channelId);
		return channel?.channel_type ?? 'text';
	}

	function startEdit(group: ChannelGroup) {
		editingGroupId = group.id;
		editGroupName = group.name;
		editGroupColor = group.color;
	}

	function cancelEdit() {
		editingGroupId = null;
	}

	// Drag-and-drop support: allow channels to be dropped onto groups.
	function handleDragOver(e: DragEvent) {
		e.preventDefault();
		if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
	}

	async function handleDrop(e: DragEvent, groupId: string) {
		e.preventDefault();
		const channelId = e.dataTransfer?.getData('text/plain');
		if (!channelId) return;

		// Check if channel already in this group.
		const group = groups.find(g => g.id === groupId);
		if (group?.channels.includes(channelId)) return;

		try {
			const res = await fetch(`/api/v1/users/@me/channel-groups/${groupId}/channels`, {
				method: 'PUT',
				headers: authHeaders(true),
				body: JSON.stringify({ channel_id: channelId })
			});
			if (!res.ok && res.status !== 204) {
				throw new Error('Failed to add channel');
			}
			groups = groups.map(g => {
				if (g.id === groupId) {
					return { ...g, channels: [...g.channels, channelId] };
				}
				return g;
			});
			addToast('Channel added to group', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to add channel', 'error');
		}
	}
</script>

{#if !loading && groups.length > 0}
	{#each groups as group (group.id)}
		<div
			class="mb-1"
			ondragover={handleDragOver}
			ondrop={(e) => handleDrop(e, group.id)}
			role="group"
		>
			<!-- Group header -->
			<div class="flex items-center justify-between px-1 pt-3">
				{#if editingGroupId === group.id}
					<div class="flex flex-1 items-center gap-1">
						<input
							type="text"
							class="flex-1 rounded bg-bg-primary px-1 py-0.5 text-2xs font-bold uppercase tracking-wide text-text-primary"
							bind:value={editGroupName}
							onkeydown={(e) => {
								if (e.key === 'Enter') updateGroup(group.id);
								if (e.key === 'Escape') cancelEdit();
							}}
						/>
						<input
							type="color"
							class="h-4 w-4 cursor-pointer border-0 bg-transparent p-0"
							bind:value={editGroupColor}
						/>
						<button
							class="text-text-muted hover:text-green-400"
							onclick={() => updateGroup(group.id)}
							title="Save"
						>
							<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
								<path d="M5 13l4 4L19 7" />
							</svg>
						</button>
						<button
							class="text-text-muted hover:text-red-400"
							onclick={cancelEdit}
							title="Cancel"
						>
							<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
								<path d="M6 18L18 6M6 6l12 12" />
							</svg>
						</button>
					</div>
				{:else}
					<button
						class="flex items-center gap-1 text-2xs font-bold uppercase tracking-wide hover:text-text-secondary"
						style="color: {group.color || 'var(--text-muted)'};"
						onclick={() => toggleGroup(group.id)}
						title={collapsedGroups.has(group.id) ? 'Expand' : 'Collapse'}
					>
						<svg
							class="h-3 w-3 shrink-0 transition-transform duration-200 {collapsedGroups.has(group.id) ? '-rotate-90' : ''}"
							fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"
						>
							<path d="M19 9l-7 7-7-7" />
						</svg>
						{group.name}
						<span class="ml-1 text-text-muted">({group.channels.length})</span>
					</button>
					<div class="flex items-center gap-0.5">
						<button
							class="text-text-muted hover:text-text-primary"
							onclick={() => startEdit(group)}
							title="Edit group"
						>
							<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
								<path d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
							</svg>
						</button>
						<button
							class="text-text-muted hover:text-red-400"
							onclick={() => deleteGroup(group.id)}
							title="Delete group"
						>
							<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
								<path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
							</svg>
						</button>
					</div>
				{/if}
			</div>

			<!-- Group channels -->
			{#if !collapsedGroups.has(group.id)}
				{#if group.channels.length === 0}
					<p class="px-3 py-1 text-2xs text-text-muted italic">Drag channels here</p>
				{:else}
					{#each group.channels as channelId (channelId)}
						{@const unread = $unreadCounts.get(channelId) ?? 0}
						{@const mentions = $mentionCounts.get(channelId) ?? 0}
						{@const isVoice = getChannelType(channelId) === 'voice' || getChannelType(channelId) === 'stage'}
						<div class="group/item flex items-center">
							<button
								class="mb-0.5 flex flex-1 items-center gap-1.5 rounded px-2 py-1.5 text-left text-sm transition-colors {$currentChannelId === channelId ? 'bg-bg-modifier text-text-primary' : unread > 0 ? 'text-text-primary font-semibold hover:bg-bg-modifier' : 'text-text-muted hover:bg-bg-modifier hover:text-text-secondary'}"
								onclick={() => handleChannelClick(channelId)}
								draggable="true"
								ondragstart={(e) => e.dataTransfer?.setData('text/plain', channelId)}
							>
								{#if isVoice}
									<svg class="h-4 w-4 shrink-0" fill="currentColor" viewBox="0 0 24 24">
										<path d="M12 3a1 1 0 0 0-1 1v8a3 3 0 1 0 6 0V4a1 1 0 1 0-2 0v8a1 1 0 1 1-2 0V4a1 1 0 0 0-1-1zM7 12a5 5 0 0 0 10 0h2a7 7 0 0 1-6 6.92V21h-2v-2.08A7 7 0 0 1 5 12h2z" />
									</svg>
								{:else}
									<span class="text-lg leading-none">#</span>
								{/if}
								<span class="flex-1 truncate">{getChannelName(channelId)}</span>
								{#if mentions > 0 && $currentChannelId !== channelId}
									<span class="ml-auto flex h-4 min-w-4 items-center justify-center rounded-full bg-red-500 px-1 text-2xs font-bold text-white">
										@{mentions > 99 ? '99+' : mentions}
									</span>
								{:else if unread > 0 && $currentChannelId !== channelId}
									<span class="ml-auto flex h-4 min-w-4 items-center justify-center rounded-full bg-text-muted px-1 text-2xs font-bold text-white">
										{unread > 99 ? '99+' : unread}
									</span>
								{/if}
							</button>
							<button
								class="mr-1 hidden shrink-0 rounded p-0.5 text-text-muted hover:text-red-400 group-hover/item:block"
								onclick={() => removeChannel(group.id, channelId)}
								title="Remove from group"
							>
								<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
									<path d="M6 18L18 6M6 6l12 12" />
								</svg>
							</button>
						</div>
					{/each}
				{/if}
			{/if}
		</div>
	{/each}
{/if}

<!-- Create group button (shown when there are existing groups or always at bottom) -->
{#if !loading}
	<button
		class="mt-2 flex w-full items-center gap-1.5 rounded px-2 py-1 text-left text-xs text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-secondary"
		onclick={() => (showCreateModal = true)}
	>
		<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path d="M12 5v14m-7-7h14" />
		</svg>
		New Channel Group
	</button>
{/if}

<!-- Create Group Modal -->
{#if showCreateModal}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
		onclick={() => (showCreateModal = false)}
	>
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="w-full max-w-sm rounded-lg bg-bg-floating p-5 shadow-xl"
			onclick={(e) => e.stopPropagation()}
		>
			<h3 class="mb-4 text-base font-semibold text-text-primary">Create Channel Group</h3>

			<div class="mb-3">
				<label for="groupName" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
					Group Name
				</label>
				<input
					id="groupName"
					type="text"
					class="input w-full"
					bind:value={newGroupName}
					placeholder="My Favorites"
					maxlength="64"
					onkeydown={(e) => e.key === 'Enter' && createGroup()}
				/>
			</div>

			<div class="mb-4">
				<label for="groupColor" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
					Color
				</label>
				<div class="flex items-center gap-2">
					<input
						id="groupColor"
						type="color"
						class="h-8 w-8 cursor-pointer rounded border-0 bg-transparent p-0"
						bind:value={newGroupColor}
					/>
					<span class="text-xs text-text-muted">{newGroupColor}</span>
				</div>
			</div>

			<div class="flex justify-end gap-2">
				<button class="btn-secondary" onclick={() => (showCreateModal = false)}>Cancel</button>
				<button
					class="btn-primary"
					onclick={createGroup}
					disabled={creating || !newGroupName.trim()}
				>
					{creating ? 'Creating...' : 'Create'}
				</button>
			</div>
		</div>
	</div>
{/if}
