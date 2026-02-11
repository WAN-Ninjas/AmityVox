<script lang="ts">
	import { currentGuildId } from '$lib/stores/guilds';
	import { currentUser } from '$lib/stores/auth';
</script>

<svelte:head>
	<title>AmityVox</title>
</svelte:head>

<div class="flex h-full flex-col items-center justify-center bg-bg-tertiary p-8">
	{#if !$currentGuildId}
		<div class="max-w-md text-center">
			<h1 class="mb-2 text-2xl font-bold text-text-primary">
				Welcome{$currentUser ? `, ${$currentUser.display_name ?? $currentUser.username}` : ''}!
			</h1>
			<p class="mb-6 text-text-secondary">
				Select a guild from the sidebar to get started, or create a new one.
			</p>
			<div class="flex justify-center gap-4">
				<button class="btn-primary">Create a Guild</button>
				<button class="btn-secondary">Join a Guild</button>
			</div>
		</div>
	{:else}
		<p class="text-text-muted">Select a channel from the sidebar.</p>
	{/if}
</div>
