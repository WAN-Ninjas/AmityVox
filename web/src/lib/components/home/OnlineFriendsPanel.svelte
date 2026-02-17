<script lang="ts">
	import { relationships } from '$lib/stores/relationships';
	import { presenceMap } from '$lib/stores/presence';
	import { api } from '$lib/api/client';
	import { addDMChannel } from '$lib/stores/dms';
	import { goto } from '$app/navigation';
	import Avatar from '$lib/components/common/Avatar.svelte';

	// Get friends that are currently online/idle/dnd.
	const onlineFriends = $derived.by(() => {
		const result: Array<{ id: string; username: string; displayName: string | null; avatarId: string | null; status: string }> = [];
		for (const [targetId, rel] of $relationships) {
			if (rel.type !== 'friend') continue;
			const status = $presenceMap.get(targetId);
			if (status && status !== 'offline') {
				result.push({
					id: targetId,
					username: rel.user?.username ?? targetId,
					displayName: rel.user?.display_name ?? null,
					avatarId: rel.user?.avatar_id ?? null,
					status
				});
			}
		}
		return result.sort((a, b) => (a.displayName ?? a.username).localeCompare(b.displayName ?? b.username));
	});

	const statusDotColor: Record<string, string> = {
		online: 'bg-status-online',
		idle: 'bg-status-idle',
		dnd: 'bg-status-dnd'
	};

	async function openDM(userId: string) {
		try {
			const channel = await api.createDM(userId);
			addDMChannel(channel);
			goto(`/app/dms/${channel.id}`);
		} catch {
			// Ignore
		}
	}
</script>

<div class="rounded-lg bg-bg-secondary p-4">
	<div class="mb-3 flex items-center gap-2">
		<svg class="h-5 w-5 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path stroke-linecap="round" stroke-linejoin="round" d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0z" />
		</svg>
		<h3 class="text-sm font-semibold text-text-primary">Online Friends</h3>
		{#if onlineFriends.length > 0}
			<span class="rounded-full bg-brand-500/20 px-1.5 py-0.5 text-2xs font-medium text-brand-400">{onlineFriends.length}</span>
		{/if}
	</div>

	{#if onlineFriends.length === 0}
		<p class="py-2 text-sm text-text-muted">No friends online right now.</p>
	{:else}
		<div class="space-y-1">
			{#each onlineFriends.slice(0, 10) as friend (friend.id)}
				<button
					class="flex w-full items-center gap-2.5 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-bg-modifier"
					onclick={() => openDM(friend.id)}
					title="Send message to {friend.displayName ?? friend.username}"
				>
					<Avatar
						name={friend.displayName ?? friend.username}
						src={friend.avatarId ? `/api/v1/files/${friend.avatarId}` : null}
						size="sm"
						status={friend.status}
					/>
					<span class="flex-1 truncate text-sm text-text-secondary">{friend.displayName ?? friend.username}</span>
					<span class="inline-block h-2 w-2 rounded-full {statusDotColor[friend.status] ?? 'bg-status-offline'}"></span>
				</button>
			{/each}
			{#if onlineFriends.length > 10}
				<p class="pt-1 text-center text-xs text-text-muted">+{onlineFriends.length - 10} more online</p>
			{/if}
		</div>
	{/if}
</div>
