<script lang="ts">
	import { currentUser } from '$lib/stores/auth';
	import { avatarUrl } from '$lib/utils/avatar';
	import Avatar from '$lib/components/common/Avatar.svelte';
	import MyIssuesPanel from './MyIssuesPanel.svelte';
	import OnlineFriendsPanel from './OnlineFriendsPanel.svelte';
	import ActiveVoicePanel from './ActiveVoicePanel.svelte';
	import CreateGuildModal from '$components/guild/CreateGuildModal.svelte';

	let showCreateModal = $state(false);
	let createModalMode = $state<'create' | 'join'>('create');

	const greeting = $derived.by(() => {
		const hour = new Date().getHours();
		if (hour < 12) return 'Good morning';
		if (hour < 18) return 'Good afternoon';
		return 'Good evening';
	});
</script>

<div class="flex h-full flex-col overflow-y-auto bg-bg-tertiary">
	<!-- Header -->
	<div class="border-b border-bg-modifier px-6 py-5">
		<div class="flex items-center gap-4">
			{#if $currentUser}
				<Avatar
					name={$currentUser.display_name ?? $currentUser.username}
					src={avatarUrl($currentUser.avatar_id)}
					size="lg"
					status={$currentUser.status_presence}
				/>
			{/if}
			<div>
				<h1 class="text-xl font-bold text-text-primary">
					{greeting}{$currentUser ? `, ${$currentUser.display_name ?? $currentUser.username}` : ''}!
				</h1>
				<p class="mt-1 text-sm text-text-muted">Here's what's happening across your servers.</p>
			</div>
		</div>
	</div>

	<!-- Quick Actions -->
	<div class="flex gap-3 px-6 pt-4">
		<button class="btn-primary text-sm" onclick={() => { createModalMode = 'create'; showCreateModal = true; }}>Create a Server</button>
		<button class="btn-secondary text-sm" onclick={() => { createModalMode = 'join'; showCreateModal = true; }}>Join a Server</button>
	</div>

	<!-- Dashboard Panels -->
	<div class="grid gap-4 p-6 lg:grid-cols-2 xl:grid-cols-3">
		<div class="xl:col-span-1">
			<OnlineFriendsPanel />
		</div>
		<div class="xl:col-span-1">
			<ActiveVoicePanel />
		</div>
		<div class="xl:col-span-1">
			<MyIssuesPanel />
		</div>
	</div>
</div>

<CreateGuildModal bind:open={showCreateModal} initialMode={createModalMode} onclose={() => (showCreateModal = false)} />
