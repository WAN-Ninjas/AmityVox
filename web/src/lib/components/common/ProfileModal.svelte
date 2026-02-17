<script lang="ts">
	import type { User, UserLink, MutualGuild } from '$lib/types';
	import { api } from '$lib/api/client';
	import { presenceMap } from '$lib/stores/presence';
	import { currentUser } from '$lib/stores/auth';
	import { addToast } from '$lib/stores/toast';
	import { relationships, addOrUpdateRelationship } from '$lib/stores/relationships';
	import { addDMChannel } from '$lib/stores/dms';
	import { guildMembers, guildRolesMap } from '$lib/stores/members';
	import { getMemberRoleColor } from '$lib/utils/roleColor';
	import Avatar from './Avatar.svelte';
	import { goto } from '$app/navigation';

	interface Props {
		userId: string;
		open?: boolean;
		onclose: () => void;
	}

	let { userId, open = $bindable(false), onclose }: Props = $props();

	let user = $state<User | null>(null);
	let links = $state<UserLink[]>([]);
	let mutualFriends = $state<User[]>([]);
	let mutualGuilds = $state<MutualGuild[]>([]);
	let loading = $state(true);
	let note = $state('');
	let noteLoaded = $state(false);
	let noteSaving = $state(false);
	let addingFriend = $state(false);

	const isSelf = $derived($currentUser?.id === userId);
	const status = $derived($presenceMap.get(userId) ?? 'offline');
	const relationship = $derived($relationships.get(userId));
	const roleColor = $derived.by(() => {
		const member = $guildMembers.get(userId);
		if (!member?.roles) return null;
		return getMemberRoleColor(member.roles, $guildRolesMap);
	});

	const UserFlagBot = 1 << 3;
	const UserFlagVerified = 1 << 4;

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

	$effect(() => {
		if (!open) return;
		loading = true;
		const promises: Promise<void>[] = [
			api.getUser(userId).then((u) => { user = u; }),
			api.getUserLinks(userId).then((l) => { links = l; }).catch(() => { links = []; })
		];
		if (!isSelf) {
			promises.push(
				api.getMutualFriends(userId).then((f) => { mutualFriends = f; }).catch(() => { mutualFriends = []; }),
				api.getMutualGuilds(userId).then((g) => { mutualGuilds = g; }).catch(() => { mutualGuilds = []; }),
				api.getUserNote(userId).then((d) => { note = d.note ?? ''; noteLoaded = true; }).catch(() => { noteLoaded = true; })
			);
		}
		Promise.all(promises).finally(() => { loading = false; });
	});

	async function handleMessage() {
		if (!user) return;
		try {
			const channel = await api.createDM(user.id);
			addDMChannel(channel);
			onclose();
			goto(`/app/dms/${channel.id}`);
		} catch {
			addToast('Failed to create DM', 'error');
		}
	}

	async function handleAddFriend() {
		if (addingFriend) return;
		addingFriend = true;
		try {
			const rel = await api.addFriend(userId);
			addOrUpdateRelationship(rel);
			addToast(rel.type === 'friend' ? 'Friend request accepted!' : 'Friend request sent!', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to send friend request', 'error');
		} finally {
			addingFriend = false;
		}
	}

	async function saveNote() {
		if (noteSaving) return;
		noteSaving = true;
		try {
			await api.setUserNote(userId, note);
		} catch {
			addToast('Failed to save note', 'error');
		} finally {
			noteSaving = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') onclose();
	}

	function formatLastOnline(dateStr: string): string {
		const diff = Date.now() - new Date(dateStr).getTime();
		const mins = Math.floor(diff / 60000);
		if (mins < 1) return 'Just now';
		if (mins < 60) return `${mins}m ago`;
		const hours = Math.floor(mins / 60);
		if (hours < 24) return `${hours}h ago`;
		const days = Math.floor(hours / 24);
		if (days < 7) return `${days}d ago`;
		return new Date(dateStr).toLocaleDateString(undefined, { month: 'long', day: 'numeric', year: 'numeric' });
	}

	const platformIcons: Record<string, string> = {
		github: 'M12 2C6.477 2 2 6.477 2 12c0 4.42 2.87 8.17 6.84 9.5.5.08.66-.23.66-.5v-1.69c-2.77.6-3.36-1.34-3.36-1.34-.46-1.16-1.11-1.47-1.11-1.47-.91-.62.07-.6.07-.6 1 .07 1.53 1.03 1.53 1.03.87 1.52 2.34 1.07 2.91.83.09-.65.35-1.09.63-1.34-2.22-.25-4.55-1.11-4.55-4.92 0-1.11.38-2 1.03-2.71-.1-.25-.45-1.29.1-2.64 0 0 .84-.27 2.75 1.02.79-.22 1.65-.33 2.5-.33.85 0 1.71.11 2.5.33 1.91-1.29 2.75-1.02 2.75-1.02.55 1.35.2 2.39.1 2.64.65.71 1.03 1.6 1.03 2.71 0 3.82-2.34 4.66-4.57 4.91.36.31.69.92.69 1.85V21c0 .27.16.59.67.5C19.14 20.16 22 16.42 22 12A10 10 0 0012 2z',
		twitter: 'M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z',
		website: 'M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2v1.93zm6.9-2.54c-.26-.81-1-1.39-1.9-1.39h-1v-3c0-.55-.45-1-1-1H8v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z',
		youtube: 'M23.498 6.186a3.016 3.016 0 0 0-2.122-2.136C19.505 3.545 12 3.545 12 3.545s-7.505 0-9.377.505A3.017 3.017 0 0 0 .502 6.186C0 8.07 0 12 0 12s0 3.93.502 5.814a3.016 3.016 0 0 0 2.122 2.136c1.871.505 9.376.505 9.376.505s7.505 0 9.377-.505a3.015 3.015 0 0 0 2.122-2.136C24 15.93 24 12 24 12s0-3.93-.502-5.814zM9.545 15.568V8.432L15.818 12l-6.273 3.568z'
	};
</script>

{#if open}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-[100] flex items-center justify-center bg-black/60"
		onclick={() => onclose()}
		onkeydown={handleKeydown}
		role="dialog"
		aria-modal="true"
		tabindex="-1"
	>
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="w-full max-w-lg overflow-hidden rounded-xl bg-bg-floating shadow-2xl"
			onclick={(e) => e.stopPropagation()}
		>
			{#if loading}
				<div class="flex items-center justify-center p-16">
					<div class="h-6 w-6 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
				</div>
			{:else if user}
				<!-- Banner -->
				{#if user.banner_id}
					<img class="w-full object-cover" style="aspect-ratio: 3/1;" src="/api/v1/files/{user.banner_id}" alt="" />
				{:else}
					<div class="h-24" style="background: {user.accent_color ?? 'var(--brand-600)'}"></div>
				{/if}

				<!-- Avatar -->
				<div class="relative px-5">
					<div class="-mt-10">
						<Avatar
							name={user.display_name ?? user.username}
							src={user.avatar_id ? `/api/v1/files/${user.avatar_id}` : null}
							size="lg"
							{status}
						/>
					</div>
				</div>

				<!-- Content -->
				<div class="max-h-[60vh] overflow-y-auto px-5 pb-5 pt-3">
					<!-- Name + badges -->
					<div class="flex items-center gap-2">
						<h2
							class="text-xl font-bold text-text-primary"
							style={roleColor ? `color: ${roleColor}` : ''}
						>
							{user.display_name ?? user.username}
						</h2>
						{#if user.flags & UserFlagVerified}
							<svg class="h-5 w-5 text-brand-500" viewBox="0 0 24 24" fill="currentColor">
								<path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41L9 16.17z" />
							</svg>
						{/if}
						{#if user.flags & UserFlagBot}
							<span class="rounded bg-brand-500/20 px-1.5 py-0.5 text-xs font-bold text-brand-400">BOT</span>
						{/if}
					</div>
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

					<!-- Profile Links -->
					{#if links.length > 0}
						<div class="mt-3 border-t border-bg-modifier pt-3">
							<h4 class="mb-2 text-2xs font-bold uppercase tracking-wide text-text-muted">Links</h4>
							<div class="flex flex-wrap gap-2">
								{#each links as link (link.id)}
									<a
										href={/^https?:\/\//i.test(link.url) ? link.url : '#'}
										target="_blank"
										rel="noopener noreferrer"
										class="flex items-center gap-1.5 rounded-md bg-bg-primary px-2.5 py-1.5 text-xs text-text-secondary transition-colors hover:bg-bg-modifier hover:text-text-primary"
										title={link.url}
									>
										{#if platformIcons[link.platform]}
											<svg class="h-3.5 w-3.5 shrink-0" fill="currentColor" viewBox="0 0 24 24">
												<path d={platformIcons[link.platform]} />
											</svg>
										{:else}
											<svg class="h-3.5 w-3.5 shrink-0" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
												<path d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
											</svg>
										{/if}
										{link.label}
										{#if link.verified}
											<svg class="h-3 w-3 text-brand-500" viewBox="0 0 24 24" fill="currentColor">
												<path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41L9 16.17z" />
											</svg>
										{/if}
									</a>
								{/each}
							</div>
						</div>
					{/if}

					<!-- Mutual Guilds -->
					{#if !isSelf && mutualGuilds.length > 0}
						<div class="mt-3 border-t border-bg-modifier pt-3">
							<h4 class="mb-1.5 text-2xs font-bold uppercase tracking-wide text-text-muted">
								Mutual Servers - {mutualGuilds.length}
							</h4>
							<div class="flex flex-wrap gap-1.5">
								{#each mutualGuilds as guild (guild.id)}
									<button
										class="flex items-center gap-1.5 rounded-md bg-bg-primary px-2 py-1 text-xs text-text-secondary transition-colors hover:bg-bg-modifier hover:text-text-primary"
										onclick={() => { onclose(); goto(`/app/guilds/${guild.id}`); }}
										title={guild.name}
									>
										{#if guild.icon_id}
											<img class="h-4 w-4 rounded object-cover" src="/api/v1/files/{guild.icon_id}" alt="" />
										{:else}
											<span class="flex h-4 w-4 items-center justify-center rounded bg-brand-600 text-2xs font-bold text-white">
												{guild.name[0]?.toUpperCase() ?? '?'}
											</span>
										{/if}
										<span class="max-w-24 truncate">{guild.name}</span>
									</button>
								{/each}
							</div>
						</div>
					{/if}

					<!-- Mutual Friends -->
					{#if !isSelf && mutualFriends.length > 0}
						<div class="mt-3 border-t border-bg-modifier pt-3">
							<h4 class="mb-1.5 text-2xs font-bold uppercase tracking-wide text-text-muted">
								Mutual Friends - {mutualFriends.length}
							</h4>
							<div class="flex flex-wrap gap-1.5">
								{#each mutualFriends as friend (friend.id)}
									<div
										class="flex items-center gap-1.5 rounded-md bg-bg-primary px-2 py-1 text-xs text-text-secondary"
										title={friend.display_name ?? friend.username}
									>
										<Avatar
											name={friend.display_name ?? friend.username}
											src={friend.avatar_id ? `/api/v1/files/${friend.avatar_id}` : null}
											size="sm"
										/>
										<span class="max-w-20 truncate">{friend.display_name ?? friend.username}</span>
									</div>
								{/each}
							</div>
						</div>
					{/if}

					<!-- Member Since & Last Online -->
					<div class="mt-3 border-t border-bg-modifier pt-3">
						<h4 class="mb-1 text-2xs font-bold uppercase tracking-wide text-text-muted">Member Since</h4>
						<p class="text-xs text-text-secondary">
							{new Date(user.created_at).toLocaleDateString(undefined, { month: 'long', day: 'numeric', year: 'numeric' })}
						</p>
						{#if status === 'offline' && user.last_online}
							<h4 class="mb-1 mt-2 text-2xs font-bold uppercase tracking-wide text-text-muted">Last Online</h4>
							<p class="text-xs text-text-secondary">{formatLastOnline(user.last_online)}</p>
						{/if}
					</div>

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

					<!-- Actions -->
					{#if !isSelf}
						<div class="mt-4 flex gap-2">
							<button class="btn-primary flex-1 text-sm" onclick={handleMessage}>Message</button>
							{#if !relationship || relationship.type === 'pending_incoming'}
								<button
									class="flex-1 rounded px-3 py-1.5 text-sm font-medium transition-colors {relationship?.type === 'pending_incoming' ? 'bg-green-600 text-white hover:bg-green-700' : 'bg-brand-500 text-white hover:bg-brand-600'}"
									onclick={handleAddFriend}
									disabled={addingFriend}
								>
									{addingFriend ? '...' : relationship?.type === 'pending_incoming' ? 'Accept Request' : 'Add Friend'}
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
					{/if}
				</div>
			{/if}
		</div>
	</div>
{/if}
