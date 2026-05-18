<script lang="ts">
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { getErrorMessage } from '$lib/utils/apiError';
	import { createAsyncOp } from '$lib/utils/asyncOp';
	import Avatar from '$components/common/Avatar.svelte';
	import Modal from '$components/common/Modal.svelte';
	import type { User } from '$lib/types';

	interface AdminUserGuild {
		id: string;
		name: string;
		icon_url: string | null;
		is_owner: boolean;
		member_count: number;
		joined_at: string;
	}

	let users = $state<User[]>([]);
	let usersLoaded = $state(false);
	let loadUsersOp = $state(createAsyncOp());
	let userSearch = $state('');
	let searchTimeout: ReturnType<typeof setTimeout> | null = null;
	let expandedUserGuilds = $state<string | null>(null);
	let userGuildsList = $state<AdminUserGuild[]>([]);
	let loadGuildsOp = $state(createAsyncOp());
	let banModalOpen = $state(false);
	let banTargetUser = $state<User | null>(null);
	let banReason = $state('');
	let banOp = $state(createAsyncOp());

	$effect(() => {
		if (!usersLoaded && !loadUsersOp.loading) {
			loadUsers();
		}
	});

	async function loadUsers(query?: string) {
		const result = await loadUsersOp.run(() => api.getAdminUsers({ limit: 50, query }));
		if (result) {
			users = result;
			usersLoaded = true;
		} else {
			users = [];
		}
	}

	function handleUserSearch() {
		if (searchTimeout) clearTimeout(searchTimeout);
		searchTimeout = setTimeout(() => {
			loadUsers(userSearch || undefined);
		}, 300);
	}

	async function handleSuspend(userId: string) {
		try {
			await api.suspendUser(userId);
			users = users.map((user) => user.id === userId ? { ...user, flags: user.flags | 1 } : user);
			addToast('User suspended', 'success');
		} catch {
			addToast('Failed to suspend user', 'error');
		}
	}

	async function handleUnsuspend(userId: string) {
		try {
			await api.unsuspendUser(userId);
			users = users.map((user) => user.id === userId ? { ...user, flags: user.flags & ~1 } : user);
			addToast('User unsuspended', 'success');
		} catch {
			addToast('Failed to unsuspend user', 'error');
		}
	}

	async function handleToggleAdmin(userId: string, currentFlags: number) {
		const isAdmin = (currentFlags & 4) !== 0;
		try {
			await api.setAdmin(userId, !isAdmin);
			users = users.map((user) => user.id === userId ? { ...user, flags: isAdmin ? user.flags & ~4 : user.flags | 4 } : user);
			addToast(isAdmin ? 'Admin removed' : 'Admin granted', 'success');
		} catch {
			addToast('Failed to update admin status', 'error');
		}
	}

	async function handleToggleGlobalMod(userId: string, currentFlags: number) {
		const isMod = (currentFlags & 32) !== 0;
		try {
			await api.setGlobalMod(userId, !isMod);
			users = users.map((user) => user.id === userId ? { ...user, flags: isMod ? user.flags & ~32 : user.flags | 32 } : user);
			addToast(isMod ? 'Global Mod removed' : 'Global Mod granted', 'success');
		} catch {
			addToast('Failed to update global mod status', 'error');
		}
	}

	async function loadUserGuilds(userId: string) {
		if (expandedUserGuilds === userId) {
			expandedUserGuilds = null;
			return;
		}
		expandedUserGuilds = userId;
		const result = await loadGuildsOp.run(
			() => api.getAdminUserGuilds(userId),
			msg => addToast(msg, 'error'),
			'Failed to load user servers'
		);
		if (result) {
			userGuildsList = result;
		} else {
			userGuildsList = [];
		}
	}

	function openBanModal(user: User) {
		banTargetUser = user;
		banReason = '';
		banModalOpen = true;
	}

	async function handleInstanceBan() {
		if (!banTargetUser || !banReason.trim()) return;
		const result = await banOp.run(
			() => api.instanceBanUser(banTargetUser!.id, banReason.trim()),
			msg => addToast(msg, 'error'),
			'Failed to ban user'
		);
		if (result !== undefined) {
			addToast(`${banTargetUser.display_name ?? banTargetUser.username} has been instance-banned`, 'success');
			users = users.map((user) => user.id === banTargetUser?.id ? { ...user, flags: user.flags | 1 } : user);
			banModalOpen = false;
			banTargetUser = null;
			banReason = '';
		}
	}
</script>

<div class="mb-6 flex items-center justify-between">
	<h1 class="text-2xl font-bold text-text-primary">User Management</h1>
	<input
		type="search"
		class="input w-72"
		aria-label="Search users"
		placeholder="Search users..."
		bind:value={userSearch}
		oninput={handleUserSearch}
	/>
</div>

{#if loadUsersOp.loading}
	<p class="text-sm text-text-muted">Loading users...</p>
{:else if users.length === 0}
	<p class="text-sm text-text-muted">No users found.</p>
{:else}
	<div class="space-y-2">
		{#each users as user (user.id)}
			{@const isSuspended = (user.flags & 1) !== 0}
			{@const isAdmin = (user.flags & 4) !== 0}
			{@const isMod = (user.flags & 32) !== 0}
			<div class="rounded-lg bg-bg-secondary p-3">
				<div class="flex items-center justify-between gap-4">
					<div class="flex min-w-0 items-center gap-3">
						<Avatar name={user.display_name ?? user.username} size="sm" />
						<div class="min-w-0">
							<div class="flex flex-wrap items-center gap-2">
								<span class="font-medium text-text-primary">{user.display_name ?? user.username}</span>
								<span class="text-xs text-text-muted">@{user.username}</span>
								{#if isAdmin}<span class="rounded bg-brand-500/20 px-1.5 py-0.5 text-2xs font-bold text-brand-400">Admin</span>{/if}
								{#if isMod}<span class="rounded bg-orange-500/20 px-1.5 py-0.5 text-2xs font-bold text-orange-400">Global Mod</span>{/if}
								{#if isSuspended}<span class="rounded bg-red-500/20 px-1.5 py-0.5 text-2xs font-bold text-red-400">Suspended</span>{/if}
							</div>
							<p class="truncate text-xs text-text-muted">
								{user.email ?? 'No email'} · {user.id.slice(0, 8)}... · Joined {new Date(user.created_at).toLocaleDateString()}
							</p>
						</div>
					</div>
					<div class="flex shrink-0 flex-wrap justify-end gap-2">
						<button class="text-xs text-brand-400 hover:text-brand-300" onclick={() => loadUserGuilds(user.id)}>
							{expandedUserGuilds === user.id ? 'Hide Servers' : 'Servers'}
						</button>
						<button class="text-xs text-purple-400 hover:text-purple-300" onclick={() => handleToggleAdmin(user.id, user.flags)}>
							{isAdmin ? 'Remove Admin' : 'Make Admin'}
						</button>
						<button class="text-xs text-blue-400 hover:text-blue-300" onclick={() => handleToggleGlobalMod(user.id, user.flags)}>
							{isMod ? 'Remove Mod' : 'Make Mod'}
						</button>
						<button
							class="text-xs {isSuspended ? 'text-green-400 hover:text-green-300' : 'text-yellow-400 hover:text-yellow-300'}"
							onclick={() => isSuspended ? handleUnsuspend(user.id) : handleSuspend(user.id)}
						>
							{isSuspended ? 'Unsuspend' : 'Suspend'}
						</button>
						<button class="text-xs text-red-400 hover:text-red-300" onclick={() => openBanModal(user)}>Ban</button>
					</div>
				</div>
				{#if expandedUserGuilds === user.id}
					<div class="mt-3 rounded bg-bg-primary p-3">
						{#if loadGuildsOp.loading}
							<p class="text-xs text-text-muted">Loading servers...</p>
						{:else if userGuildsList.length === 0}
							<p class="text-xs text-text-muted">No servers found.</p>
						{:else}
							<div class="grid gap-2 md:grid-cols-2">
								{#each userGuildsList as guild (guild.id)}
									<div class="flex items-center justify-between rounded bg-bg-secondary px-2 py-1.5">
										<span class="truncate text-sm text-text-primary">{guild.name}</span>
										<span class="text-xs text-text-muted">{guild.is_owner ? 'Owner' : 'Member'} · {guild.member_count} members</span>
									</div>
								{/each}
							</div>
						{/if}
					</div>
				{/if}
			</div>
		{/each}
	</div>
{/if}

<Modal open={banModalOpen} title="Instance Ban User" onclose={() => (banModalOpen = false)}>
	{#if banTargetUser}
		<p class="mb-4 text-sm text-text-secondary">
			Ban <strong class="text-text-primary">{banTargetUser.display_name ?? banTargetUser.username}</strong> from this instance? They will not be able to log in or interact.
		</p>
		<div class="mb-4">
			<label for="admin-ban-reason" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Reason</label>
			<textarea
				id="admin-ban-reason"
				class="input w-full"
				rows="3"
				placeholder="Provide a reason for the ban..."
				bind:value={banReason}
			></textarea>
		</div>
		<div class="flex justify-end gap-2">
			<button class="btn-secondary text-sm" onclick={() => (banModalOpen = false)}>Cancel</button>
			<button
				class="rounded bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-500 disabled:opacity-50"
				onclick={handleInstanceBan}
				disabled={banOp.loading || !banReason.trim()}
			>
				{banOp.loading ? 'Banning...' : 'Ban User'}
			</button>
		</div>
	{/if}
</Modal>
