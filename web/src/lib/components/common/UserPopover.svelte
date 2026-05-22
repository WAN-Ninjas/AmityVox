<script lang="ts">
	import type { User, MutualGuild } from '$lib/types';
	import { api } from '$lib/api/client';
	import { addDMChannel } from '$lib/stores/dms';
	import { presenceMap } from '$lib/stores/presence';
	import { currentUser } from '$lib/stores/auth';
	import { addToast } from '$lib/stores/toast';
	import { relationships, addOrUpdateRelationship } from '$lib/stores/relationships';
	import { goto } from '$app/navigation';
	import Avatar from './Avatar.svelte';
	import ProfileModal from './ProfileModal.svelte';
	import FederationBadge from './FederationBadge.svelte';
	import { guildMembers, guildRolesMap } from '$lib/stores/members';
	import { getMemberRoleColor } from '$lib/utils/roleColor';
	import { clientNicknames, setClientNickname } from '$lib/stores/nicknames';
	import { blockedUsers, addBlockedUser, removeBlockedUser, type BlockLevel } from '$lib/stores/blocked';
	import { avatarUrl, fileUrl } from '$lib/utils/avatar';
	import { getErrorMessage } from '$lib/utils/apiError';
	import { createAsyncOp } from '$lib/utils/asyncOp';

	interface Props {
		userId: string;
		x: number;
		y: number;
		onclose: () => void;
		onviewprofile?: (userId: string) => void;
	}

	let { userId, x, y, onclose, onviewprofile }: Props = $props();

	let user = $state<User | null>(null);
	let loadOp = $state(createAsyncOp(true));
	let note = $state('');
	let noteLoaded = $state(false);
	let noteOp = $state(createAsyncOp());
	let nickname = $state('');
	const currentNickname = $derived($clientNicknames.get(userId) ?? '');

	// Mutual data
	let mutualFriends = $state<User[]>([]);
	let mutualGuilds = $state<MutualGuild[]>([]);
	let mutualsLoaded = $state(false);

	const isSelf = $derived($currentUser?.id === userId);
	const status = $derived($presenceMap.get(userId) ?? 'offline');
	const relationship = $derived($relationships.get(userId));
	const userRoleColor = $derived.by(() => {
		const member = $guildMembers.get(userId);
		if (!member?.roles) return null;
		return getMemberRoleColor(member.roles, $guildRolesMap);
	});

	let friendOp = $state(createAsyncOp());
	let showFullProfile = $state(false);
	let blockOp = $state(createAsyncOp());
	const currentBlockLevel = $derived($blockedUsers.get(userId) ?? null);

	async function handleBlock(level: BlockLevel) {
		if (blockOp.loading) return;
		await blockOp.run(
			async () => {
			await api.blockUser(userId, level);
			addBlockedUser(userId, level);
			addToast(level === 'ignore' ? 'User ignored' : 'User blocked', 'success');
			},
			(message) => addToast(message, 'error'),
			'Failed to block user'
		);
	}

	async function handleUnblock() {
		if (blockOp.loading) return;
		await blockOp.run(
			async () => {
			await api.unblockUser(userId);
			removeBlockedUser(userId);
			addToast('User unblocked', 'success');
			},
			(message) => addToast(message, 'error'),
			'Failed to unblock user'
		);
	}

	async function handleAddFriend() {
		if (friendOp.loading) return;
		await friendOp.run(
			async () => {
			const rel = await api.addFriend(userId);
			addOrUpdateRelationship(rel);
			addToast(rel.type === 'friend' ? 'Friend request accepted!' : 'Friend request sent!', 'success');
			},
			(message) => addToast(message, 'error'),
			'Failed to send friend request'
		);
	}

	function saveNickname() {
		setClientNickname(userId, nickname);
	}

	$effect(() => {
		nickname = $clientNicknames.get(userId) ?? '';
	});

	$effect(() => {
		loadOp.run(
			async () => {
				user = await api.getUser(userId);
			},
			undefined,
			'Failed to load user'
		);

		if (!isSelf) {
			api.getUserNote(userId)
				.then((data) => { note = data.note ?? ''; noteLoaded = true; })
				.catch(() => { noteLoaded = true; });

			// Load mutual friends + guilds in parallel.
			Promise.all([
				api.getMutualFriends(userId).catch(() => []),
				api.getMutualGuilds(userId).catch(() => [])
			]).then(([friends, guilds]) => {
				mutualFriends = friends;
				mutualGuilds = guilds;
				mutualsLoaded = true;
			});
		}
	});

	// Position the popover so it doesn't overflow the viewport.
	const popoverStyle = $derived.by(() => {
		const maxW = 320;
		const maxH = 520;
		let left = x;
		let top = y;

		if (typeof window !== 'undefined') {
			if (left + maxW > window.innerWidth) left = window.innerWidth - maxW - 8;
			if (top + maxH > window.innerHeight) top = window.innerHeight - maxH - 8;
			if (left < 8) left = 8;
			if (top < 8) top = 8;
		}

		return `left: ${left}px; top: ${top}px;`;
	});

	async function handleMessage() {
		if (!user) return;
		try {
			const channel = await api.createDM(user.id);
			addDMChannel(channel);
			onclose();
			goto(`/app/dms/${channel.id}`);
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to create DM'), 'error');
		}
	}

	async function saveNote() {
		if (noteOp.loading) return;
		await noteOp.run(
			async () => {
			await api.setUserNote(userId, note);
			},
			(message) => addToast(message, 'error'),
			'Failed to save note'
		);
	}

	// Guard: ignore the click event that opened us (same event still bubbling).
	let ready = false;
	$effect(() => {
		requestAnimationFrame(() => { ready = true; });
		return () => { ready = false; };
	});

	function handleClickOutside(e: MouseEvent) {
		if (!ready) return;
		const target = e.target as HTMLElement;
		if (!target.closest('.user-popover')) {
			onclose();
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') onclose();
	}

	const statusText: Record<string, string> = {
		online: 'Online',
		idle: 'Idle',
		dnd: 'Do Not Disturb',
		offline: 'Offline'
	};

	const statusDotColor: Record<string, string> = {
		online: 'bg-status-online',
		idle: 'bg-status-idle',
		dnd: 'bg-status-dnd',
		offline: 'bg-status-offline'
	};

	// User flag constants.
	const UserFlagBot = 1 << 3;
	const UserFlagVerified = 1 << 4;
</script>

<svelte:document onclick={handleClickOutside} onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
	class="user-popover fixed z-50 w-80 overflow-hidden rounded-lg bg-bg-floating shadow-xl"
	style={popoverStyle}
>
	{#if loadOp.loading}
		<div class="flex items-center justify-center p-8">
			<div class="h-5 w-5 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
		</div>
	{:else if loadOp.error}
		<div class="p-4 text-sm text-red-400">{loadOp.error}</div>
	{:else if user}
		<!-- Banner area — use accent color if set, banner image if available -->
		{#if user.banner_id}
			<img class="w-full object-cover" style="aspect-ratio: 3/1;" src={fileUrl(user.banner_id, user.instance_id || undefined)} alt="" />
		{:else}
			<div class="h-16" style="background: {user.accent_color ?? 'var(--brand-600)'}"></div>
		{/if}

		<!-- Avatar overlapping banner -->
		<div class="relative px-4">
			<div class="-mt-8">
				<Avatar
					name={user.display_name ?? user.username}
					src={avatarUrl(user.avatar_id, user.instance_id && $currentUser && user.instance_id !== $currentUser.instance_id ? user.instance_id : undefined)}
					size="lg"
					{status}
				/>
			</div>
		</div>

		<!-- User info -->
		<div class="max-h-96 overflow-y-auto px-4 pb-4 pt-2">
			<div class="flex items-center gap-1.5">
				<h3 class="text-lg font-bold text-text-primary" style={userRoleColor ? `color: ${userRoleColor}` : ''}>
					{#if currentNickname}
						<span class="italic">{currentNickname}</span>
					{:else}
						{user.display_name ?? user.username}
					{/if}
				</h3>
					{#if user.flags & UserFlagVerified}
						<svg class="h-4 w-4 text-brand-500" viewBox="0 0 24 24" fill="currentColor">
							<title>Verified</title>
							<path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41L9 16.17z"/>
						</svg>
				{/if}
				{#if user.flags & UserFlagBot}
					<span class="rounded bg-brand-500/20 px-1.5 py-0.5 text-2xs font-bold text-brand-400">BOT</span>
				{/if}
				{#if user.instance_domain || (user.instance_id && $currentUser && user.instance_id !== $currentUser.instance_id)}
					<FederationBadge domain={user.instance_domain ?? user.instance_id} />
				{/if}
			</div>
			{#if currentNickname}
				<p class="text-xs text-text-muted">{user.display_name ?? user.username}</p>
			{/if}
			<p class="text-sm text-text-muted">{user.handle ?? '@' + user.username}</p>
			{#if user.pronouns}
				<p class="text-xs text-text-muted">{user.pronouns}</p>
			{/if}

			<!-- Status -->
			<div class="mt-2 flex items-center gap-1.5">
				<span class="inline-block h-2.5 w-2.5 rounded-full {statusDotColor[status] ?? statusDotColor.offline}"></span>
				<span class="text-xs text-text-secondary">
					{#if user.status_emoji}{user.status_emoji} {/if}{user.status_text ?? statusText[status] ?? 'Offline'}
				</span>
			</div>

			<!-- Bio -->
			{#if user.bio}
				<div class="mt-3 border-t border-bg-modifier pt-3">
					<h4 class="mb-1 text-2xs font-bold uppercase tracking-wide text-text-muted">About Me</h4>
					<p class="whitespace-pre-wrap text-sm text-text-secondary">{user.bio}</p>
				</div>
			{/if}

			<!-- Mutual Guilds -->
			{#if !isSelf && mutualsLoaded && mutualGuilds.length > 0}
				<div class="mt-3 border-t border-bg-modifier pt-3">
					<h4 class="mb-1.5 text-2xs font-bold uppercase tracking-wide text-text-muted">
						Mutual Servers - {mutualGuilds.length}
					</h4>
					<div class="flex flex-wrap gap-1.5">
						{#each mutualGuilds.slice(0, 6) as guild (guild.id)}
							<button
								class="flex items-center gap-1.5 rounded-md bg-bg-primary px-2 py-1 text-xs text-text-secondary transition-colors hover:bg-bg-modifier hover:text-text-primary"
								onclick={() => { onclose(); goto(`/app/guilds/${guild.id}`); }}
								title={guild.name}
							>
								{#if guild.icon_id}
									<img class="h-4 w-4 rounded object-cover" src={fileUrl(guild.icon_id, guild.instance_id || undefined)} alt="" />
								{:else}
									<span class="flex h-4 w-4 items-center justify-center rounded bg-brand-600 text-2xs font-bold text-white">
										{guild.name[0]?.toUpperCase() ?? '?'}
									</span>
								{/if}
								<span class="max-w-20 truncate">{guild.name}</span>
							</button>
						{/each}
						{#if mutualGuilds.length > 6}
							<span class="flex items-center px-1 text-xs text-text-muted">+{mutualGuilds.length - 6} more</span>
						{/if}
					</div>
				</div>
			{/if}

			<!-- Mutual Friends -->
			{#if !isSelf && mutualsLoaded && mutualFriends.length > 0}
				<div class="mt-3 border-t border-bg-modifier pt-3">
					<h4 class="mb-1.5 text-2xs font-bold uppercase tracking-wide text-text-muted">
						Mutual Friends - {mutualFriends.length}
					</h4>
					<div class="flex flex-wrap gap-1.5">
						{#each mutualFriends.slice(0, 6) as friend (friend.id)}
							<div class="flex items-center gap-1.5 rounded-md bg-bg-primary px-2 py-1 text-xs text-text-secondary" title={friend.display_name ?? friend.username}>
								<Avatar
									name={friend.display_name ?? friend.username}
									src={avatarUrl(friend.avatar_id, friend.instance_id || undefined)}
									size="sm"
								/>
								<span class="max-w-16 truncate">{friend.display_name ?? friend.username}</span>
							</div>
						{/each}
						{#if mutualFriends.length > 6}
							<span class="flex items-center px-1 text-xs text-text-muted">+{mutualFriends.length - 6} more</span>
						{/if}
					</div>
				</div>
			{/if}

			<!-- Member since & Last online -->
			<div class="mt-3 border-t border-bg-modifier pt-3">
				<h4 class="mb-1 text-2xs font-bold uppercase tracking-wide text-text-muted">Member Since</h4>
				<p class="text-xs text-text-secondary">
					{new Date(user.created_at).toLocaleDateString(undefined, { month: 'long', day: 'numeric', year: 'numeric' })}
				</p>
				{#if status === 'offline' && user.last_online}
					<h4 class="mb-1 mt-2 text-2xs font-bold uppercase tracking-wide text-text-muted">Last Online</h4>
					<p class="text-xs text-text-secondary">
						{(() => {
							const diff = Date.now() - new Date(user.last_online).getTime();
							const mins = Math.floor(diff / 60000);
							if (mins < 1) return 'Just now';
							if (mins < 60) return `${mins} minute${mins !== 1 ? 's' : ''} ago`;
							const hours = Math.floor(mins / 60);
							if (hours < 24) return `${hours} hour${hours !== 1 ? 's' : ''} ago`;
							const days = Math.floor(hours / 24);
							if (days < 7) return `${days} day${days !== 1 ? 's' : ''} ago`;
							return new Date(user.last_online).toLocaleDateString(undefined, { month: 'long', day: 'numeric', year: 'numeric' });
						})()}
					</p>
				{/if}
			</div>

			<!-- Nickname (client-side only) -->
			{#if !isSelf}
				<div class="mt-3 border-t border-bg-modifier pt-3">
					<h4 class="mb-1 text-2xs font-bold uppercase tracking-wide text-text-muted">Nickname</h4>
					<input
						type="text"
						class="w-full rounded bg-bg-primary px-2 py-1.5 text-xs text-text-secondary outline-none placeholder:text-text-muted focus:ring-1 focus:ring-brand-500"
						placeholder="Set a nickname (only visible to you)"
						maxlength="64"
						bind:value={nickname}
						onblur={saveNickname}
						onkeydown={(e) => { if (e.key === 'Enter') { e.currentTarget.blur(); } }}
					/>
				</div>
			{/if}

			<!-- Note -->
			{#if !isSelf && noteLoaded}
				<div class="mt-3 border-t border-bg-modifier pt-3">
					<h4 class="mb-1 text-2xs font-bold uppercase tracking-wide text-text-muted">Note</h4>
					<textarea
						class="w-full resize-none rounded bg-bg-primary px-2 py-1.5 text-xs text-text-secondary outline-none placeholder:text-text-muted focus:ring-1 focus:ring-brand-500"
						placeholder="Click to add a note"
						rows="2"
						maxlength="256"
						bind:value={note}
						onblur={saveNote}
					></textarea>
				</div>
			{/if}

			<!-- View Full Profile -->
			<div class="mt-3 border-t border-bg-modifier pt-3">
				<button
					class="w-full rounded px-3 py-1.5 text-xs font-medium text-text-secondary transition-colors hover:bg-bg-modifier hover:text-text-primary"
					onclick={() => { if (onviewprofile) { onviewprofile(userId); } else { showFullProfile = true; } }}
				>
					View Full Profile
				</button>
			</div>

			<!-- Actions -->
			{#if !isSelf}
				<div class="mt-3 flex gap-2">
					<button class="btn-primary flex-1 text-sm" onclick={handleMessage}>
						Message
					</button>
					{#if !relationship || relationship.type === 'pending_incoming'}
						<button
							class="flex-1 rounded px-3 py-1.5 text-sm font-medium transition-colors {relationship?.type === 'pending_incoming' ? 'bg-green-600 text-white hover:bg-green-700' : 'bg-brand-500 text-white hover:bg-brand-600'}"
							onclick={handleAddFriend}
							disabled={friendOp.loading}
						>
							{friendOp.loading ? '...' : relationship?.type === 'pending_incoming' ? 'Accept Request' : 'Add Friend'}
						</button>
					{:else if relationship.type === 'pending_outgoing'}
						<button
							class="flex-1 cursor-default rounded bg-bg-modifier px-3 py-1.5 text-sm font-medium text-text-muted"
							disabled
						>
							Request Sent
						</button>
					{/if}
				</div>

				<!-- Block / Unblock -->
				<div class="mt-2 flex gap-2">
					{#if currentBlockLevel}
						<button
							class="flex-1 rounded border border-bg-modifier px-3 py-1.5 text-xs font-medium text-text-secondary transition-colors hover:bg-bg-modifier"
							onclick={handleUnblock}
							disabled={blockOp.loading}
						>
							{blockOp.loading ? '...' : 'Unblock'}
						</button>
					{:else}
						<button
							class="flex-1 rounded px-3 py-1.5 text-xs font-medium text-yellow-500 transition-colors hover:bg-yellow-500/10"
							onclick={() => handleBlock('ignore')}
							disabled={blockOp.loading}
						>
							Ignore
						</button>
						<button
							class="flex-1 rounded px-3 py-1.5 text-xs font-medium text-red-500 transition-colors hover:bg-red-500/10"
							onclick={() => handleBlock('block')}
							disabled={blockOp.loading}
						>
							Block
						</button>
					{/if}
				</div>
			{/if}
		</div>
	{/if}
</div>

<ProfileModal {userId} bind:open={showFullProfile} onclose={() => (showFullProfile = false)} />
