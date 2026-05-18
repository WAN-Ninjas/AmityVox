<script lang="ts">
	import { api } from '$lib/api/client';
	import { removeBlockedUser, updateBlockedUserLevel, type BlockLevel } from '$lib/stores/blocked';
	import { avatarUrl } from '$lib/utils/avatar';
	import { createAsyncOp } from '$lib/utils/asyncOp';
	import { getErrorMessage } from '$lib/utils/apiError';
	import Avatar from '$components/common/Avatar.svelte';
	import type { User } from '$lib/types';

	type DmPrivacy = 'everyone' | 'friends' | 'nobody';
	type FriendRequestPrivacy = 'everyone' | 'mutual_guilds' | 'nobody';
	type NsfwContentFilter = 'blur_all' | 'blur_suspicious' | 'show_all';

	let dmPrivacy = $state<DmPrivacy>('everyone');
	let friendRequestPrivacy = $state<FriendRequestPrivacy>('everyone');
	let nsfwContentFilter = $state<NsfwContentFilter>('blur_all');
	let privacySuccess = $state('');
	let privacyError = $state('');
	let blockedList = $state<Array<{ target_id: string; level: string; created_at: string; user?: User }>>([]);
	let updatingBlock = $state<string | null>(null);
	let privacyOp = $state(createAsyncOp());
	let blockedOp = $state(createAsyncOp());
	let saveOp = $state(createAsyncOp());
	let loaded = false;

	$effect(() => {
		if (!loaded) {
			loaded = true;
			loadPrivacy();
			loadBlockedList();
		}
	});

	async function loadPrivacy() {
		await privacyOp.run(async () => {
			const settings = await api.getUserSettings();
			dmPrivacy = settings.dm_privacy ?? 'everyone';
			friendRequestPrivacy = settings.friend_request_privacy ?? 'everyone';
			nsfwContentFilter = settings.nsfw_content_filter ?? 'blur_all';
		});
	}

	async function loadBlockedList() {
		const result = await blockedOp.run(() => api.getBlockedUsers());
		if (result) {
			blockedList = result;
		} else {
			blockedList = [];
		}
	}

	async function handleChangeBlockLevel(targetId: string, level: BlockLevel) {
		updatingBlock = targetId;
		privacyError = '';
		try {
			await api.updateBlockLevel(targetId, level);
			updateBlockedUserLevel(targetId, level);
			blockedList = blockedList.map((blocked) => blocked.target_id === targetId ? { ...blocked, level } : blocked);
		} catch (err: unknown) {
			privacyError = getErrorMessage(err, 'Failed to update blocked user');
		} finally {
			updatingBlock = null;
		}
	}

	async function handleUnblockUser(targetId: string) {
		updatingBlock = targetId;
		privacyError = '';
		try {
			await api.unblockUser(targetId);
			removeBlockedUser(targetId);
			blockedList = blockedList.filter((blocked) => blocked.target_id !== targetId);
		} catch (err: unknown) {
			privacyError = getErrorMessage(err, 'Failed to unblock user');
		} finally {
			updatingBlock = null;
		}
	}

	async function savePrivacy() {
		privacySuccess = '';
		privacyError = '';
		await saveOp.run(async () => {
			await api.updateUserSettings({
				dm_privacy: dmPrivacy,
				friend_request_privacy: friendRequestPrivacy,
				nsfw_content_filter: nsfwContentFilter
			});
			localStorage.setItem('av-nsfw-filter', nsfwContentFilter);
			privacySuccess = 'Privacy settings saved!';
			setTimeout(() => (privacySuccess = ''), 3000);
		}, msg => {
			privacyError = msg;
		}, 'Failed to save privacy settings');
	}
</script>

<h1 class="mb-6 text-xl font-bold text-text-primary">Privacy</h1>

{#if privacySuccess}
	<div class="mb-4 rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{privacySuccess}</div>
{/if}
{#if privacyError}
	<div class="mb-4 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{privacyError}</div>
{/if}

<div class="space-y-6">
	<div class="rounded-lg bg-bg-secondary p-4">
		<h3 class="mb-1 text-sm font-semibold text-text-primary">Direct Messages</h3>
		<p class="mb-3 text-xs text-text-muted">Control who can send you direct messages.</p>
		<div class="space-y-2">
			<label class="flex items-center gap-2">
				<input type="radio" name="dmPrivacy" value="everyone" bind:group={dmPrivacy} class="accent-brand-500" />
				<span class="text-sm text-text-secondary">Everyone</span>
			</label>
			<label class="flex items-center gap-2">
				<input type="radio" name="dmPrivacy" value="friends" bind:group={dmPrivacy} class="accent-brand-500" />
				<span class="text-sm text-text-secondary">Friends only</span>
			</label>
			<label class="flex items-center gap-2">
				<input type="radio" name="dmPrivacy" value="nobody" bind:group={dmPrivacy} class="accent-brand-500" />
				<span class="text-sm text-text-secondary">Nobody</span>
			</label>
		</div>
	</div>

	<div class="rounded-lg bg-bg-secondary p-4">
		<h3 class="mb-1 text-sm font-semibold text-text-primary">Friend Requests</h3>
		<p class="mb-3 text-xs text-text-muted">Control who can send you friend requests.</p>
		<div class="space-y-2">
			<label class="flex items-center gap-2">
				<input type="radio" name="friendPrivacy" value="everyone" bind:group={friendRequestPrivacy} class="accent-brand-500" />
				<span class="text-sm text-text-secondary">Everyone</span>
			</label>
			<label class="flex items-center gap-2">
				<input type="radio" name="friendPrivacy" value="mutual_guilds" bind:group={friendRequestPrivacy} class="accent-brand-500" />
				<span class="text-sm text-text-secondary">People in mutual servers</span>
			</label>
			<label class="flex items-center gap-2">
				<input type="radio" name="friendPrivacy" value="nobody" bind:group={friendRequestPrivacy} class="accent-brand-500" />
				<span class="text-sm text-text-secondary">Nobody</span>
			</label>
		</div>
	</div>

	<div class="rounded-lg bg-bg-secondary p-4">
		<h3 class="mb-1 text-sm font-semibold text-text-primary">NSFW Content Filter</h3>
		<p class="mb-3 text-xs text-text-muted">Control how images are displayed in NSFW-marked channels.</p>
		<div class="space-y-2">
			<label class="flex items-center gap-2">
				<input type="radio" name="nsfwFilter" value="blur_all" bind:group={nsfwContentFilter} class="accent-brand-500" />
				<span class="text-sm text-text-secondary">Blur all media in NSFW channels</span>
			</label>
			<label class="flex items-center gap-2">
				<input type="radio" name="nsfwFilter" value="show_all" bind:group={nsfwContentFilter} class="accent-brand-500" />
				<span class="text-sm text-text-secondary">Show all media</span>
			</label>
		</div>
	</div>

	<button class="btn-primary" onclick={savePrivacy} disabled={saveOp.loading}>
		{saveOp.loading ? 'Saving...' : 'Save Privacy Settings'}
	</button>

	<div class="mt-2 rounded-lg bg-bg-secondary p-4">
		<h3 class="mb-1 text-sm font-semibold text-text-primary">Blocked Users</h3>
		<p class="mb-3 text-xs text-text-muted">Manage users you've blocked or ignored.</p>

		{#if blockedOp.loading}
			<div class="flex items-center justify-center py-4">
				<div class="h-5 w-5 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
			</div>
		{:else if blockedList.length === 0}
			<p class="py-2 text-sm text-text-muted">No blocked users.</p>
		{:else}
			<div class="space-y-2">
				{#each blockedList as blocked (blocked.target_id)}
					<div class="flex items-center gap-3 rounded-md bg-bg-primary px-3 py-2">
						<Avatar
							name={blocked.user?.display_name ?? blocked.user?.username ?? 'Unknown'}
							src={avatarUrl(blocked.user?.avatar_id, blocked.user?.instance_id || undefined)}
							size="sm"
						/>
						<div class="min-w-0 flex-1">
							<p class="truncate text-sm font-medium text-text-primary">{blocked.user?.display_name ?? blocked.user?.username ?? 'Unknown'}</p>
							<p class="text-xs text-text-muted">@{blocked.user?.username ?? 'unknown'}</p>
						</div>
						<span class="rounded px-1.5 py-0.5 text-2xs font-bold uppercase {blocked.level === 'block' ? 'bg-red-500/20 text-red-400' : 'bg-yellow-500/20 text-yellow-400'}">
							{blocked.level}
						</span>
						<select
							class="rounded bg-bg-modifier px-2 py-1 text-xs text-text-secondary outline-none"
							value={blocked.level}
							onchange={(e) => handleChangeBlockLevel(blocked.target_id, (e.target as HTMLSelectElement).value as BlockLevel)}
							disabled={updatingBlock === blocked.target_id}
						>
							<option value="ignore">Ignore</option>
							<option value="block">Block</option>
						</select>
						<button
							class="rounded px-2 py-1 text-xs font-medium text-text-secondary transition-colors hover:bg-bg-modifier hover:text-text-primary"
							onclick={() => handleUnblockUser(blocked.target_id)}
							disabled={updatingBlock === blocked.target_id}
						>
							Unblock
						</button>
					</div>
				{/each}
			</div>
		{/if}
	</div>
</div>
