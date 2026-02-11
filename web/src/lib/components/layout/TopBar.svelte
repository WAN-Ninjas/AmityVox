<script lang="ts">
	import { currentChannel } from '$lib/stores/channels';
	import { currentGuild } from '$lib/stores/guilds';

	interface Props {
		onToggleMembers?: () => void;
	}

	let { onToggleMembers }: Props = $props();
</script>

<header class="flex h-12 items-center border-b border-bg-floating bg-bg-tertiary px-4">
	{#if $currentChannel}
		<div class="flex items-center gap-2">
			{#if $currentChannel.channel_type === 'text' || $currentChannel.channel_type === 'announcement'}
				<span class="text-lg text-text-muted">#</span>
			{/if}
			<h1 class="font-semibold text-text-primary">{$currentChannel.name ?? 'Unknown Channel'}</h1>
			{#if $currentChannel.topic}
				<span class="mx-2 text-text-muted">|</span>
				<span class="truncate text-sm text-text-muted">{$currentChannel.topic}</span>
			{/if}
		</div>
	{:else if $currentGuild}
		<h1 class="font-semibold text-text-primary">Select a channel</h1>
	{:else}
		<h1 class="font-semibold text-text-primary">Home</h1>
	{/if}

	<div class="ml-auto flex items-center gap-3">
		<!-- Member toggle -->
		{#if $currentGuild}
			<button
				class="text-text-muted transition-colors hover:text-text-primary"
				onclick={onToggleMembers}
				title="Toggle Member List"
			>
				<svg class="h-5 w-5" fill="currentColor" viewBox="0 0 24 24">
					<path d="M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z" />
				</svg>
			</button>
		{/if}

		<!-- Search -->
		<button class="text-text-muted transition-colors hover:text-text-primary" title="Search">
			<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<circle cx="11" cy="11" r="8" />
				<path d="m21 21-4.35-4.35" />
			</svg>
		</button>
	</div>
</header>
