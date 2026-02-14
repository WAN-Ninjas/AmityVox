<script lang="ts">
	import type { User } from '$lib/types';
	import { api } from '$lib/api/client';
	import { addDMChannel } from '$lib/stores/dms';
	import { presenceMap } from '$lib/stores/presence';
	import { currentUser } from '$lib/stores/auth';
	import { addToast } from '$lib/stores/toast';
	import { relationships, addOrUpdateRelationship } from '$lib/stores/relationships';
	import { goto } from '$app/navigation';
	import Avatar from './Avatar.svelte';

	interface Props {
		userId: string;
		x: number;
		y: number;
		onclose: () => void;
	}

	let { userId, x, y, onclose }: Props = $props();

	let user = $state<User | null>(null);
	let loading = $state(true);
	let error = $state('');
	let note = $state('');
	let noteLoaded = $state(false);
	let noteSaving = $state(false);

	const isSelf = $derived($currentUser?.id === userId);
	const status = $derived($presenceMap.get(userId) ?? 'offline');
	const relationship = $derived($relationships.get(userId));

	let addingFriend = $state(false);

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

	$effect(() => {
		api.getUser(userId)
			.then((u) => (user = u))
			.catch((e) => (error = e.message || 'Failed to load user'))
			.finally(() => (loading = false));

		if (!isSelf) {
			api.getUserNote(userId)
				.then((data) => { note = data.note ?? ''; noteLoaded = true; })
				.catch(() => { noteLoaded = true; });
		}
	});

	// Position the popover so it doesn't overflow the viewport.
	const popoverStyle = $derived.by(() => {
		const maxW = 320;
		const maxH = 400;
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
		} catch (err: any) {
			addToast('Failed to create DM', 'error');
		}
	}

	async function saveNote() {
		if (noteSaving) return;
		noteSaving = true;
		try {
			await api.setUserNote(userId, note);
		} catch (err: any) {
			addToast('Failed to save note', 'error');
		} finally {
			noteSaving = false;
		}
	}

	function handleClickOutside(e: MouseEvent) {
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
</script>

<svelte:document onclick={handleClickOutside} onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
	class="user-popover fixed z-50 w-80 overflow-hidden rounded-lg bg-bg-floating shadow-xl"
	style={popoverStyle}
>
	{#if loading}
		<div class="flex items-center justify-center p-8">
			<div class="h-5 w-5 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
		</div>
	{:else if error}
		<div class="p-4 text-sm text-red-400">{error}</div>
	{:else if user}
		<!-- Banner area -->
		<div class="h-16 bg-brand-600"></div>

		<!-- Avatar overlapping banner -->
		<div class="relative px-4">
			<div class="-mt-8">
				<Avatar
					name={user.display_name ?? user.username}
					src={user.avatar_id ? `/api/v1/files/${user.avatar_id}` : null}
					size="lg"
					{status}
				/>
			</div>
		</div>

		<!-- User info -->
		<div class="px-4 pb-4 pt-2">
			<h3 class="text-lg font-bold text-text-primary">
				{user.display_name ?? user.username}
			</h3>
			<p class="text-sm text-text-muted">{user.handle ?? '@' + user.username}</p>

			<!-- Status -->
			<div class="mt-2 flex items-center gap-1.5">
				<span class="inline-block h-2.5 w-2.5 rounded-full {statusDotColor[status] ?? statusDotColor.offline}"></span>
				<span class="text-xs text-text-secondary">
					{user.status_text ?? statusText[status] ?? 'Offline'}
				</span>
			</div>

			<!-- Bio -->
			{#if user.bio}
				<div class="mt-3 border-t border-bg-modifier pt-3">
					<h4 class="mb-1 text-2xs font-bold uppercase tracking-wide text-text-muted">About Me</h4>
					<p class="text-sm text-text-secondary">{user.bio}</p>
				</div>
			{/if}

			<!-- Member since -->
			<div class="mt-3 border-t border-bg-modifier pt-3">
				<h4 class="mb-1 text-2xs font-bold uppercase tracking-wide text-text-muted">Member Since</h4>
				<p class="text-xs text-text-secondary">
					{new Date(user.created_at).toLocaleDateString(undefined, { month: 'long', day: 'numeric', year: 'numeric' })}
				</p>
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
				<div class="mt-3 flex gap-2">
					<button class="btn-primary flex-1 text-sm" onclick={handleMessage}>
						Message
					</button>
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
