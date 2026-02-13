<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import type { User, Relationship, Channel } from '$lib/types';
	import { presenceMap } from '$lib/stores/presence';
	import { addDMChannel } from '$lib/stores/dms';
	import { addToast } from '$lib/stores/toast';
	import { goto } from '$app/navigation';
	import Avatar from '$lib/components/common/Avatar.svelte';

	type Tab = 'all' | 'online' | 'pending' | 'blocked' | 'add';
	let currentTab = $state<Tab>('all');

	let relationships = $state<Relationship[]>([]);
	let blockedUsers = $state<{ id: string; user_id: string; target_id: string; reason: string | null; created_at: string; user?: User }[]>([]);
	let loading = $state(true);
	let dmChannels = $state<Channel[]>([]);
	let loadingDMs = $state(true);

	// Add friend state
	let handleInput = $state('');
	let resolving = $state(false);
	let addError = $state('');
	let addSuccess = $state('');

	const friends = $derived(relationships.filter((r) => r.type === 'friend'));
	const pendingIncoming = $derived(relationships.filter((r) => r.type === 'pending_incoming'));
	const pendingOutgoing = $derived(relationships.filter((r) => r.type === 'pending_outgoing'));
	const onlineFriends = $derived(
		friends.filter((r) => {
			const status = $presenceMap.get(r.target_id);
			return status && status !== 'offline' && status !== 'invisible';
		})
	);

	onMount(() => {
		loadRelationships();
		loadDMs();
	});

	async function loadRelationships() {
		loading = true;
		try {
			const [relsRes, blockedRes] = await Promise.allSettled([
				api.getFriends(),
				api.getBlockedUsers()
			]);
			relationships = relsRes.status === 'fulfilled' ? relsRes.value : [];
			blockedUsers = blockedRes.status === 'fulfilled' ? blockedRes.value : [];
		} finally {
			loading = false;
		}
	}

	function loadDMs() {
		api.getMyDMs()
			.then((dms) => (dmChannels = dms))
			.catch((e) => console.error('Failed to load DMs:', e))
			.finally(() => (loadingDMs = false));
	}

	async function sendFriendRequest() {
		if (resolving) return;
		const handle = handleInput.trim();
		if (!handle) return;

		resolving = true;
		addError = '';
		addSuccess = '';

		try {
			const user = await api.resolveHandle(handle);
			await api.addFriend(user.id);
			addSuccess = `Friend request sent to ${user.display_name ?? user.username}!`;
			handleInput = '';
			await loadRelationships();
		} catch (err: any) {
			const code = err?.code || '';
			if (code === 'already_friends') {
				addError = 'You are already friends with this user.';
			} else if (code === 'already_pending') {
				addError = 'Friend request already sent.';
			} else if (code === 'user_not_found') {
				addError = 'No user found with that handle.';
			} else if (code === 'remote_lookup_failed') {
				addError = 'Could not reach the remote instance. Try again later.';
			} else if (code === 'blocked') {
				addError = 'Cannot send a friend request to this user.';
			} else {
				addError = 'Failed to send friend request. Check the handle and try again.';
			}
		} finally {
			resolving = false;
		}
	}

	async function acceptFriend(userId: string) {
		try {
			await api.addFriend(userId);
			addToast('Friend request accepted!', 'success');
			await loadRelationships();
		} catch {
			addToast('Failed to accept friend request', 'error');
		}
	}

	async function declineFriend(userId: string) {
		try {
			await api.removeFriend(userId);
			addToast('Friend request declined', 'info');
			await loadRelationships();
		} catch {
			addToast('Failed to decline friend request', 'error');
		}
	}

	async function cancelRequest(userId: string) {
		try {
			await api.removeFriend(userId);
			addToast('Friend request cancelled', 'info');
			await loadRelationships();
		} catch {
			addToast('Failed to cancel request', 'error');
		}
	}

	async function removeFriend(userId: string) {
		try {
			await api.removeFriend(userId);
			addToast('Friend removed', 'info');
			await loadRelationships();
		} catch {
			addToast('Failed to remove friend', 'error');
		}
	}

	async function unblockUser(userId: string) {
		try {
			await api.unblockUser(userId);
			addToast('User unblocked', 'info');
			await loadRelationships();
		} catch {
			addToast('Failed to unblock user', 'error');
		}
	}

	async function messageFriend(userId: string) {
		try {
			const channel = await api.createDM(userId);
			addDMChannel(channel);
			goto(`/app/dms/${channel.id}`);
		} catch {
			addToast('Failed to create DM', 'error');
		}
	}

	function openDM(channel: Channel) {
		goto(`/app/dms/${channel.id}`);
	}

	function displayName(user?: User): string {
		if (!user) return 'Unknown';
		return user.display_name ?? user.username;
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') sendFriendRequest();
	}

	const statusDotColor: Record<string, string> = {
		online: 'bg-status-online',
		idle: 'bg-status-idle',
		dnd: 'bg-status-dnd',
		focus: 'bg-status-dnd',
		busy: 'bg-status-dnd',
		offline: 'bg-status-offline',
		invisible: 'bg-status-offline'
	};
</script>

<svelte:head>
	<title>Friends — AmityVox</title>
</svelte:head>

<div class="flex h-full flex-col bg-bg-tertiary">
	<header class="flex h-12 items-center border-b border-bg-floating px-4">
		<h1 class="font-semibold text-text-primary">Friends</h1>
		<div class="ml-4 flex gap-2">
			{#each [
				{ id: 'all', label: 'All' },
				{ id: 'online', label: 'Online' },
				{ id: 'pending', label: 'Pending' },
				{ id: 'blocked', label: 'Blocked' },
				{ id: 'add', label: 'Add Friend' }
			] as tab (tab.id)}
				<button
					class="rounded px-3 py-1 text-xs transition-colors"
					class:bg-brand-500={currentTab === tab.id && tab.id === 'add'}
					class:text-white={currentTab === tab.id && tab.id === 'add'}
					class:bg-bg-modifier={currentTab === tab.id && tab.id !== 'add'}
					class:text-text-primary={currentTab === tab.id && tab.id !== 'add'}
					class:text-text-muted={currentTab !== tab.id}
					class:hover:bg-bg-modifier={currentTab !== tab.id}
					class:hover:text-text-secondary={currentTab !== tab.id}
					onclick={() => (currentTab = tab.id as Tab)}
				>
					{tab.label}
				</button>
			{/each}
		</div>
	</header>

	<div class="flex flex-1 overflow-hidden">
		<!-- Friends list (left) -->
		<div class="flex flex-1 flex-col overflow-y-auto">
			{#if loading}
				<div class="flex flex-1 items-center justify-center">
					<div class="h-6 w-6 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
				</div>
			{:else if currentTab === 'add'}
				<div class="p-6">
					<h2 class="mb-1 text-lg font-bold text-text-primary">Add Friend</h2>
					<p class="mb-4 text-sm text-text-muted">
						Enter a username handle to send a friend request. Use <code class="rounded bg-bg-primary px-1 py-0.5 text-xs">@username</code> for local users or <code class="rounded bg-bg-primary px-1 py-0.5 text-xs">@username@domain</code> for federated users.
					</p>

					<div class="flex gap-2">
						<input
							type="text"
							class="flex-1 rounded-lg bg-bg-primary px-4 py-2.5 text-sm text-text-primary outline-none placeholder:text-text-muted focus:ring-2 focus:ring-brand-500"
							placeholder="@username or @username@domain"
							bind:value={handleInput}
							onkeydown={handleKeydown}
							disabled={resolving}
						/>
						<button
							class="rounded-lg bg-brand-500 px-6 py-2.5 text-sm font-medium text-white transition-colors hover:bg-brand-600 disabled:cursor-not-allowed disabled:opacity-50"
							onclick={sendFriendRequest}
							disabled={resolving || !handleInput.trim()}
						>
							{resolving ? 'Sending...' : 'Send Request'}
						</button>
					</div>

					{#if addError}
						<p class="mt-3 text-sm text-red-400">{addError}</p>
					{/if}
					{#if addSuccess}
						<p class="mt-3 text-sm text-green-400">{addSuccess}</p>
					{/if}
				</div>

			{:else if currentTab === 'all'}
				{#if friends.length === 0}
					<div class="flex flex-1 flex-col items-center justify-center gap-2">
						<p class="text-sm text-text-muted">No friends yet.</p>
						<button
							class="text-sm text-brand-400 hover:text-brand-300"
							onclick={() => (currentTab = 'add')}
						>
							Add a friend
						</button>
					</div>
				{:else}
					<div class="px-4 py-3">
						<h3 class="mb-2 text-2xs font-bold uppercase tracking-wide text-text-muted">
							All Friends — {friends.length}
						</h3>
					</div>
					{#each friends as rel (rel.target_id)}
						{@const user = rel.user}
						{@const status = $presenceMap.get(rel.target_id) ?? 'offline'}
						<div class="mx-2 flex items-center gap-3 rounded-lg px-3 py-2 hover:bg-bg-modifier">
							<Avatar
								name={displayName(user)}
								src={user?.avatar_id ? `/api/v1/files/${user.avatar_id}` : null}
								size="md"
								{status}
							/>
							<div class="min-w-0 flex-1">
								<p class="truncate text-sm font-medium text-text-primary">{displayName(user)}</p>
								<p class="truncate text-xs text-text-muted">{user?.handle ?? '@' + (user?.username ?? '')}</p>
							</div>
							<div class="flex items-center gap-1.5">
								<span class="inline-block h-2 w-2 rounded-full {statusDotColor[status] ?? statusDotColor.offline}"></span>
								<span class="text-xs capitalize text-text-muted">{status}</span>
							</div>
							<div class="flex gap-1">
								<button
									class="rounded bg-bg-primary px-3 py-1 text-xs text-text-secondary hover:bg-bg-modifier"
									onclick={() => messageFriend(rel.target_id)}
								>
									Message
								</button>
								<button
									class="rounded bg-bg-primary px-3 py-1 text-xs text-red-400 hover:bg-bg-modifier"
									onclick={() => removeFriend(rel.target_id)}
								>
									Remove
								</button>
							</div>
						</div>
					{/each}
				{/if}

			{:else if currentTab === 'online'}
				{#if onlineFriends.length === 0}
					<div class="flex flex-1 items-center justify-center">
						<p class="text-sm text-text-muted">No friends online right now.</p>
					</div>
				{:else}
					<div class="px-4 py-3">
						<h3 class="mb-2 text-2xs font-bold uppercase tracking-wide text-text-muted">
							Online — {onlineFriends.length}
						</h3>
					</div>
					{#each onlineFriends as rel (rel.target_id)}
						{@const user = rel.user}
						{@const status = $presenceMap.get(rel.target_id) ?? 'offline'}
						<div class="mx-2 flex items-center gap-3 rounded-lg px-3 py-2 hover:bg-bg-modifier">
							<Avatar
								name={displayName(user)}
								src={user?.avatar_id ? `/api/v1/files/${user.avatar_id}` : null}
								size="md"
								{status}
							/>
							<div class="min-w-0 flex-1">
								<p class="truncate text-sm font-medium text-text-primary">{displayName(user)}</p>
								<p class="truncate text-xs text-text-muted">{user?.handle ?? '@' + (user?.username ?? '')}</p>
							</div>
							<div class="flex items-center gap-1.5">
								<span class="inline-block h-2 w-2 rounded-full {statusDotColor[status] ?? statusDotColor.offline}"></span>
								<span class="text-xs capitalize text-text-muted">{status}</span>
							</div>
							<button
								class="rounded bg-bg-primary px-3 py-1 text-xs text-text-secondary hover:bg-bg-modifier"
								onclick={() => messageFriend(rel.target_id)}
							>
								Message
							</button>
						</div>
					{/each}
				{/if}

			{:else if currentTab === 'pending'}
				{#if pendingIncoming.length === 0 && pendingOutgoing.length === 0}
					<div class="flex flex-1 items-center justify-center">
						<p class="text-sm text-text-muted">No pending friend requests.</p>
					</div>
				{:else}
					{#if pendingIncoming.length > 0}
						<div class="px-4 py-3">
							<h3 class="mb-2 text-2xs font-bold uppercase tracking-wide text-text-muted">
								Incoming — {pendingIncoming.length}
							</h3>
						</div>
						{#each pendingIncoming as rel (rel.target_id)}
							{@const user = rel.user}
							<div class="mx-2 flex items-center gap-3 rounded-lg px-3 py-2 hover:bg-bg-modifier">
								<Avatar
									name={displayName(user)}
									src={user?.avatar_id ? `/api/v1/files/${user.avatar_id}` : null}
									size="md"
								/>
								<div class="min-w-0 flex-1">
									<p class="truncate text-sm font-medium text-text-primary">{displayName(user)}</p>
									<p class="truncate text-xs text-text-muted">{user?.handle ?? '@' + (user?.username ?? '')}</p>
								</div>
								<div class="flex gap-1">
									<button
										class="rounded bg-green-600 px-3 py-1 text-xs text-white hover:bg-green-700"
										onclick={() => acceptFriend(rel.target_id)}
									>
										Accept
									</button>
									<button
										class="rounded bg-bg-primary px-3 py-1 text-xs text-red-400 hover:bg-bg-modifier"
										onclick={() => declineFriend(rel.target_id)}
									>
										Decline
									</button>
								</div>
							</div>
						{/each}
					{/if}

					{#if pendingOutgoing.length > 0}
						<div class="px-4 py-3">
							<h3 class="mb-2 text-2xs font-bold uppercase tracking-wide text-text-muted">
								Outgoing — {pendingOutgoing.length}
							</h3>
						</div>
						{#each pendingOutgoing as rel (rel.target_id)}
							{@const user = rel.user}
							<div class="mx-2 flex items-center gap-3 rounded-lg px-3 py-2 hover:bg-bg-modifier">
								<Avatar
									name={displayName(user)}
									src={user?.avatar_id ? `/api/v1/files/${user.avatar_id}` : null}
									size="md"
								/>
								<div class="min-w-0 flex-1">
									<p class="truncate text-sm font-medium text-text-primary">{displayName(user)}</p>
									<p class="truncate text-xs text-text-muted">{user?.handle ?? '@' + (user?.username ?? '')}</p>
								</div>
								<button
									class="rounded bg-bg-primary px-3 py-1 text-xs text-red-400 hover:bg-bg-modifier"
									onclick={() => cancelRequest(rel.target_id)}
								>
									Cancel
								</button>
							</div>
						{/each}
					{/if}
				{/if}

			{:else if currentTab === 'blocked'}
				{#if blockedUsers.length === 0}
					<div class="flex flex-1 items-center justify-center">
						<p class="text-sm text-text-muted">No blocked users.</p>
					</div>
				{:else}
					<div class="px-4 py-3">
						<h3 class="mb-2 text-2xs font-bold uppercase tracking-wide text-text-muted">
							Blocked — {blockedUsers.length}
						</h3>
					</div>
					{#each blockedUsers as block (block.id)}
						{@const user = block.user}
						<div class="mx-2 flex items-center gap-3 rounded-lg px-3 py-2 hover:bg-bg-modifier">
							<Avatar
								name={user?.display_name ?? user?.username ?? 'Unknown'}
								src={user?.avatar_id ? `/api/v1/files/${user.avatar_id}` : null}
								size="md"
							/>
							<div class="min-w-0 flex-1">
								<p class="truncate text-sm font-medium text-text-primary">
									{user?.display_name ?? user?.username ?? 'Unknown User'}
								</p>
								<p class="truncate text-xs text-text-muted">{user?.handle ?? '@' + (user?.username ?? '')}</p>
							</div>
							<button
								class="rounded bg-bg-primary px-3 py-1 text-xs text-text-secondary hover:bg-bg-modifier"
								onclick={() => unblockUser(block.target_id)}
							>
								Unblock
							</button>
						</div>
					{/each}
				{/if}
			{/if}
		</div>

		<!-- Active DMs (right sidebar) -->
		<div class="w-60 shrink-0 border-l border-bg-floating bg-bg-secondary">
			<div class="p-3">
				<h3 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Direct Messages</h3>
				{#if loadingDMs}
					<p class="text-xs text-text-muted">Loading...</p>
				{:else if dmChannels.length === 0}
					<p class="text-xs text-text-muted">No active DMs.</p>
				{:else}
					{#each dmChannels as dm (dm.id)}
						<button
							class="mb-0.5 flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-muted hover:bg-bg-modifier hover:text-text-secondary"
							onclick={() => openDM(dm)}
						>
							<span class="truncate">{dm.name ?? 'Direct Message'}</span>
						</button>
					{/each}
				{/if}
			</div>
		</div>
	</div>
</div>
