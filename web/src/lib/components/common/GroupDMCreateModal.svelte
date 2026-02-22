<script lang="ts">
	import type { User } from '$lib/types';
	import { api } from '$lib/api/client';
	import { addDMChannel } from '$lib/stores/dms';
	import { relationships } from '$lib/stores/relationships';
	import { addToast } from '$lib/stores/toast';
	import { goto } from '$app/navigation';
	import { avatarUrl } from '$lib/utils/avatar';
	import Avatar from './Avatar.svelte';
	import Modal from './Modal.svelte';

	interface Props {
		open: boolean;
		onclose: () => void;
	}

	let { open = $bindable(), onclose }: Props = $props();

	let search = $state('');
	let selectedIds = $state<Set<string>>(new Set());
	let groupName = $state('');
	let creating = $state(false);

	// Build friend list from relationships store.
	const friends = $derived.by(() => {
		const list: Array<{ id: string; username: string; displayName: string | null; avatarId: string | null; instanceId: string | null }> = [];
		for (const [targetId, rel] of $relationships) {
			if (rel.type !== 'friend') continue;
			list.push({
				id: targetId,
				username: rel.user?.username ?? targetId,
				displayName: rel.user?.display_name ?? null,
				avatarId: rel.user?.avatar_id ?? null,
				instanceId: rel.user?.instance_id ?? null
			});
		}
		return list.sort((a, b) => (a.displayName ?? a.username).localeCompare(b.displayName ?? b.username));
	});

	const filteredFriends = $derived(
		search.trim()
			? friends.filter((f) => {
					const q = search.toLowerCase();
					return (f.displayName?.toLowerCase().includes(q) || f.username.toLowerCase().includes(q));
				})
			: friends
	);

	function toggleUser(userId: string) {
		const next = new Set(selectedIds);
		if (next.has(userId)) {
			next.delete(userId);
		} else if (next.size < 9) {
			next.add(userId);
		} else {
			addToast('Group DMs support up to 9 members', 'warning');
		}
		selectedIds = next;
	}

	async function createGroup() {
		if (selectedIds.size < 2 || creating) return;
		creating = true;
		try {
			const channel = await api.createGroupDM(Array.from(selectedIds), groupName.trim() || undefined);
			addDMChannel(channel);
			onclose();
			selectedIds = new Set();
			groupName = '';
			search = '';
			goto(`/app/dms/${channel.id}`);
		} catch (err: any) {
			addToast(err.message || 'Failed to create group DM', 'error');
		} finally {
			creating = false;
		}
	}
</script>

<Modal bind:open title="Create Group DM" {onclose}>
	<div class="space-y-4">
		<!-- Group name (optional) -->
		<div>
			<label class="mb-1 block text-xs font-medium text-text-muted" for="group-name">Group Name (optional)</label>
			<input
				id="group-name"
				type="text"
				class="input w-full text-sm"
				placeholder="My Group Chat"
				bind:value={groupName}
				maxlength="100"
			/>
		</div>

		<!-- Search friends -->
		<div>
			<label class="mb-1 block text-xs font-medium text-text-muted" for="friend-search">
				Add Friends ({selectedIds.size}/9 selected)
			</label>
			<input
				id="friend-search"
				type="text"
				class="input w-full text-sm"
				placeholder="Search friends..."
				bind:value={search}
			/>
		</div>

		<!-- Selected pills -->
		{#if selectedIds.size > 0}
			<div class="flex flex-wrap gap-1">
				{#each friends.filter((f) => selectedIds.has(f.id)) as friend (friend.id)}
					<button
						class="flex items-center gap-1 rounded-full bg-brand-500/20 px-2 py-0.5 text-xs text-brand-400 transition-colors hover:bg-brand-500/30"
						onclick={() => toggleUser(friend.id)}
						title="Remove {friend.displayName ?? friend.username}"
					>
						{friend.displayName ?? friend.username}
						<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M6 18L18 6M6 6l12 12" />
						</svg>
					</button>
				{/each}
			</div>
		{/if}

		<!-- Friend list -->
		<div class="max-h-48 overflow-y-auto rounded-md bg-bg-primary">
			{#if filteredFriends.length === 0}
				<p class="p-3 text-center text-sm text-text-muted">
					{search ? 'No friends match your search.' : 'No friends to add.'}
				</p>
			{:else}
				{#each filteredFriends as friend (friend.id)}
					<button
						class="flex w-full items-center gap-2.5 px-3 py-2 text-left transition-colors hover:bg-bg-modifier"
						onclick={() => toggleUser(friend.id)}
					>
						<Avatar
							name={friend.displayName ?? friend.username}
							src={avatarUrl(friend.avatarId, friend.instanceId || undefined)}
							size="sm"
						/>
						<span class="flex-1 truncate text-sm text-text-secondary">
							{friend.displayName ?? friend.username}
						</span>
						<div class="flex h-5 w-5 items-center justify-center rounded border {selectedIds.has(friend.id) ? 'border-brand-500 bg-brand-500' : 'border-bg-modifier'}">
							{#if selectedIds.has(friend.id)}
								<svg class="h-3 w-3 text-white" fill="none" stroke="currentColor" stroke-width="3" viewBox="0 0 24 24">
									<path d="M5 13l4 4L19 7" />
								</svg>
							{/if}
						</div>
					</button>
				{/each}
			{/if}
		</div>

		<!-- Create button -->
		<div class="flex justify-end gap-2">
			<button class="btn-secondary text-sm" onclick={onclose}>Cancel</button>
			<button
				class="btn-primary text-sm"
				onclick={createGroup}
				disabled={selectedIds.size < 2 || creating}
			>
				{creating ? 'Creating...' : `Create Group (${selectedIds.size})`}
			</button>
		</div>
	</div>
</Modal>
