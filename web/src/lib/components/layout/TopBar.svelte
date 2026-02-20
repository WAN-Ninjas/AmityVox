<script lang="ts">
	import { currentChannel, editChannelSignal } from '$lib/stores/channels';
	import { currentGuild } from '$lib/stores/guilds';
	import { canGoBack, canGoForward, goBack, goForward } from '$lib/stores/navigation';
	import { canManageChannels } from '$lib/stores/permissions';
	import SearchModal from '$components/chat/SearchModal.svelte';

	interface Props {
		onToggleMembers?: () => void;
		onTogglePins?: () => void;
		onToggleFollowers?: () => void;
		onToggleGallery?: () => void;
		showPins?: boolean;
		showFollowers?: boolean;
		showGallery?: boolean;
	}

	let { onToggleMembers, onTogglePins, onToggleFollowers, onToggleGallery, showPins = false, showFollowers = false, showGallery = false }: Props = $props();
	let showSearch = $state(false);
	let topicExpanded = $state(false);

	function handleKeydown(e: KeyboardEvent) {
		if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
			e.preventDefault();
			showSearch = !showSearch;
		}
	}
</script>

<svelte:document onkeydown={handleKeydown} />

<header class="flex h-12 items-center border-b border-bg-floating bg-bg-tertiary px-4">
	<!-- Back/Forward navigation buttons -->
	<div class="mr-2 flex items-center gap-0.5">
		<button
			class="rounded p-1 transition-colors {$canGoBack ? 'text-text-muted hover:text-text-primary' : 'cursor-default text-text-muted/30'}"
			onclick={() => goBack()}
			disabled={!$canGoBack}
			title="Go back"
		>
			<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2.5" viewBox="0 0 24 24">
				<path d="M15 19l-7-7 7-7" />
			</svg>
		</button>
		<button
			class="rounded p-1 transition-colors {$canGoForward ? 'text-text-muted hover:text-text-primary' : 'cursor-default text-text-muted/30'}"
			onclick={() => goForward()}
			disabled={!$canGoForward}
			title="Go forward"
		>
			<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2.5" viewBox="0 0 24 24">
				<path d="M9 5l7 7-7 7" />
			</svg>
		</button>
	</div>

	{#if $currentChannel}
		<div class="flex min-w-0 flex-1 items-center gap-2">
			{#if $currentChannel.channel_type === 'text' || $currentChannel.channel_type === 'announcement'}
				<span class="text-lg text-text-muted">#</span>
			{/if}
			<h1 class="shrink-0 font-semibold text-text-primary">{$currentChannel.name ?? 'Unknown Channel'}</h1>
			{#if $currentChannel.encrypted}
				<span class="flex items-center gap-1 rounded-full bg-status-online/10 px-2 py-0.5 text-2xs font-medium text-status-online" title="End-to-end encrypted">
					<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
					</svg>
					Encrypted
				</span>
			{/if}
			{#if $currentChannel.topic}
				<span class="mx-1 text-text-muted">|</span>
				<!-- svelte-ignore a11y_no_static_element_interactions -->
				<span
					class="min-w-0 cursor-pointer text-sm text-text-muted {topicExpanded ? '' : 'truncate'}"
					onclick={() => (topicExpanded = !topicExpanded)}
					title={topicExpanded ? 'Click to collapse' : $currentChannel.topic}
				>
					{$currentChannel.topic}
				</span>
			{/if}
		</div>
	{:else if $currentGuild}
		<h1 class="font-semibold text-text-primary">Select a channel</h1>
	{:else}
		<h1 class="font-semibold text-text-primary">Home</h1>
	{/if}

	<div class="ml-auto flex items-center gap-1">
		<!-- Channel settings gear (ManageChannels permission required) -->
		{#if $currentChannel && $canManageChannels}
			<button
				class="rounded p-1.5 text-text-muted transition-colors hover:text-text-primary"
				onclick={() => editChannelSignal.set($currentChannel!.id)}
				title="Channel Settings"
			>
				<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
					<circle cx="12" cy="12" r="3" />
				</svg>
			</button>
		{/if}

		<!-- Pinned messages toggle -->
		{#if $currentChannel}
			<button
				class="rounded p-1.5 transition-colors {showPins ? 'bg-bg-modifier text-text-primary' : 'text-text-muted hover:text-text-primary'}"
				onclick={onTogglePins}
				title="Pinned Messages"
			>
				<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z" />
				</svg>
			</button>
		{/if}

		<!-- Followers toggle (announcement channels only) -->
		{#if $currentChannel?.channel_type === 'announcement'}
			<button
				class="rounded p-1.5 transition-colors {showFollowers ? 'bg-bg-modifier text-text-primary' : 'text-text-muted hover:text-text-primary'}"
				onclick={onToggleFollowers}
				title="Channel Followers"
			>
				<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9" />
				</svg>
			</button>
		{/if}

		<!-- Gallery toggle -->
		{#if $currentChannel}
			<button
				class="rounded p-1.5 transition-colors {showGallery ? 'bg-bg-modifier text-text-primary' : 'text-text-muted hover:text-text-primary'}"
				onclick={onToggleGallery}
				title="Gallery"
			>
				<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
				</svg>
			</button>
		{/if}

		<!-- Member toggle -->
		{#if $currentGuild}
			<button
				class="rounded p-1.5 text-text-muted transition-colors hover:text-text-primary"
				onclick={onToggleMembers}
				title="Toggle Member List"
			>
				<svg class="h-5 w-5" fill="currentColor" viewBox="0 0 24 24">
					<path d="M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z" />
				</svg>
			</button>
		{/if}

		<!-- Search -->
		<button
			class="rounded p-1.5 text-text-muted transition-colors hover:text-text-primary"
			title="Search (Ctrl+K)"
			onclick={() => (showSearch = true)}
		>
			<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<circle cx="11" cy="11" r="8" />
				<path d="m21 21-4.35-4.35" />
			</svg>
		</button>
	</div>
</header>

<SearchModal bind:open={showSearch} onclose={() => (showSearch = false)} />
