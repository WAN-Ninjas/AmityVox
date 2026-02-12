<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import type { Channel } from '$lib/types';
	import { goto } from '$app/navigation';

	type Tab = 'all' | 'online' | 'pending' | 'blocked';
	let currentTab = $state<Tab>('all');

	let dmChannels = $state<Channel[]>([]);
	let loadingDMs = $state(true);

	onMount(() => {
		api.getMyDMs()
			.then((dms) => (dmChannels = dms))
			.catch(() => {})
			.finally(() => (loadingDMs = false));
	});

	async function openDM(channel: Channel) {
		goto(`/app/dms/${channel.id}`);
	}
</script>

<svelte:head>
	<title>Friends — AmityVox</title>
</svelte:head>

<div class="flex h-full flex-col bg-bg-tertiary">
	<header class="flex h-12 items-center border-b border-bg-floating px-4">
		<h1 class="font-semibold text-text-primary">Friends</h1>
		<div class="ml-4 flex gap-2">
			{#each [{ id: 'all', label: 'All' }, { id: 'online', label: 'Online' }, { id: 'pending', label: 'Pending' }, { id: 'blocked', label: 'Blocked' }] as tab (tab.id)}
				<button
					class="rounded px-3 py-1 text-xs transition-colors"
					class:bg-bg-modifier={currentTab === tab.id}
					class:text-text-primary={currentTab === tab.id}
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

	<div class="flex flex-1">
		<!-- Friends list (left) -->
		<div class="flex flex-1 flex-col overflow-y-auto">
			{#if currentTab === 'all'}
				<div class="p-4">
					<p class="mb-4 text-sm text-text-muted">
						Friend system is coming soon. For now, you can start direct messages with users from guild member lists.
					</p>
				</div>
			{:else if currentTab === 'online'}
				<div class="flex flex-1 items-center justify-center">
					<p class="text-sm text-text-muted">No online friends to show.</p>
				</div>
			{:else if currentTab === 'pending'}
				<div class="flex flex-1 items-center justify-center">
					<p class="text-sm text-text-muted">No pending friend requests.</p>
				</div>
			{:else if currentTab === 'blocked'}
				<div class="flex flex-1 items-center justify-center">
					<p class="text-sm text-text-muted">No blocked users.</p>
				</div>
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
