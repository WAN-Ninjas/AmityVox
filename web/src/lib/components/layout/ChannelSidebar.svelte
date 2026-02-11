<script lang="ts">
	import { currentGuild } from '$lib/stores/guilds';
	import { textChannels, voiceChannels, currentChannelId, setChannel } from '$lib/stores/channels';
	import { currentUser } from '$lib/stores/auth';
	import Avatar from '$components/common/Avatar.svelte';
	import { getPresence } from '$lib/stores/presence';

	function handleChannelClick(channelId: string) {
		setChannel(channelId);
	}
</script>

<aside class="flex h-full w-60 shrink-0 flex-col bg-bg-secondary">
	<!-- Guild header -->
	{#if $currentGuild}
		<div class="flex h-12 items-center border-b border-bg-floating px-4">
			<h2 class="truncate text-sm font-semibold text-text-primary">{$currentGuild.name}</h2>
		</div>
	{:else}
		<div class="flex h-12 items-center border-b border-bg-floating px-4">
			<h2 class="text-sm font-semibold text-text-primary">Direct Messages</h2>
		</div>
	{/if}

	<!-- Channel list -->
	<div class="flex-1 overflow-y-auto px-2 py-2">
		{#if $currentGuild}
			<!-- Text Channels -->
			{#if $textChannels.length > 0}
				<div class="mb-1 px-1 pt-4 first:pt-0">
					<h3 class="text-2xs font-bold uppercase tracking-wide text-text-muted">Text Channels</h3>
				</div>
				{#each $textChannels as channel (channel.id)}
					<button
						class="mb-0.5 flex w-full items-center gap-1.5 rounded px-2 py-1.5 text-left text-sm transition-colors"
						class:bg-bg-modifier={$currentChannelId === channel.id}
						class:text-text-primary={$currentChannelId === channel.id}
						class:text-text-muted={$currentChannelId !== channel.id}
						class:hover:bg-bg-modifier={$currentChannelId !== channel.id}
						class:hover:text-text-secondary={$currentChannelId !== channel.id}
						onclick={() => handleChannelClick(channel.id)}
					>
						<span class="text-lg leading-none">#</span>
						<span class="truncate">{channel.name}</span>
					</button>
				{/each}
			{/if}

			<!-- Voice Channels -->
			{#if $voiceChannels.length > 0}
				<div class="mb-1 px-1 pt-4">
					<h3 class="text-2xs font-bold uppercase tracking-wide text-text-muted">Voice Channels</h3>
				</div>
				{#each $voiceChannels as channel (channel.id)}
					<button
						class="mb-0.5 flex w-full items-center gap-1.5 rounded px-2 py-1.5 text-left text-sm text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-secondary"
						onclick={() => handleChannelClick(channel.id)}
					>
						<svg class="h-4 w-4 shrink-0" fill="currentColor" viewBox="0 0 24 24">
							<path d="M12 3a1 1 0 0 0-1 1v8a3 3 0 1 0 6 0V4a1 1 0 1 0-2 0v8a1 1 0 1 1-2 0V4a1 1 0 0 0-1-1zM7 12a5 5 0 0 0 10 0h2a7 7 0 0 1-6 6.92V21h-2v-2.08A7 7 0 0 1 5 12h2z" />
						</svg>
						<span class="truncate">{channel.name}</span>
					</button>
				{/each}
			{/if}
		{/if}
	</div>

	<!-- User panel (bottom) -->
	{#if $currentUser}
		<div class="flex items-center gap-2 border-t border-bg-floating bg-bg-primary/50 p-2">
			<Avatar name={$currentUser.display_name ?? $currentUser.username} size="sm" status={$currentUser.status_presence} />
			<div class="min-w-0 flex-1">
				<p class="truncate text-sm font-medium text-text-primary">
					{$currentUser.display_name ?? $currentUser.username}
				</p>
				<p class="truncate text-xs text-text-muted">
					{$currentUser.status_text ?? $currentUser.status_presence}
				</p>
			</div>
		</div>
	{/if}
</aside>
