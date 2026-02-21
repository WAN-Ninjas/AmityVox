<script lang="ts">
	import type { Message } from '$lib/types';
	import { currentChannelId } from '$lib/stores/channels';
	import { api } from '$lib/api/client';
	import Avatar from '$components/common/Avatar.svelte';
	import { avatarUrl } from '$lib/utils/avatar';

	interface Props {
		onclose: () => void;
		onscrollto?: (messageId: string) => void;
	}

	let { onclose, onscrollto }: Props = $props();

	let pins = $state<Message[]>([]);
	let loading = $state(true);
	let error = $state('');

	$effect(() => {
		const channelId = $currentChannelId;
		if (channelId) {
			loading = true;
			error = '';
			api.getPins(channelId)
				.then((p) => (pins = p))
				.catch((e) => (error = e.message || 'Failed to load pins'))
				.finally(() => (loading = false));
		}
	});

	function handleJump(messageId: string) {
		onclose();
		onscrollto?.(messageId);
	}

	function formatTime(iso: string): string {
		return new Date(iso).toLocaleString();
	}
</script>

<aside class="fixed inset-0 z-50 flex flex-col bg-bg-secondary md:relative md:inset-auto md:z-auto md:h-full md:w-80 md:shrink-0 md:border-l md:border-bg-floating">
	<div class="flex h-12 items-center justify-between border-b border-bg-floating px-4">
		<h2 class="text-sm font-semibold text-text-primary">Pinned Messages</h2>
		<button
			class="text-text-muted hover:text-text-primary"
			onclick={onclose}
			title="Close"
		>
			<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M6 18L18 6M6 6l12 12" />
			</svg>
		</button>
	</div>

	<div class="flex-1 overflow-y-auto">
		{#if loading}
			<div class="flex items-center justify-center p-8">
				<div class="h-5 w-5 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
			</div>
		{:else if error}
			<div class="p-4 text-sm text-red-400">{error}</div>
		{:else if pins.length === 0}
			<div class="flex flex-col items-center justify-center p-8 text-center">
				<svg class="mb-2 h-12 w-12 text-text-muted" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
					<path d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z" />
				</svg>
				<p class="text-sm text-text-muted">No pinned messages in this channel.</p>
			</div>
		{:else}
			{#each pins as pin (pin.id)}
				<button
					class="w-full border-b border-bg-modifier px-4 py-3 text-left transition-colors hover:bg-bg-modifier/50"
					onclick={() => handleJump(pin.id)}
				>
					<div class="flex items-center gap-2">
						<Avatar
							name={pin.author?.display_name ?? pin.author?.username ?? '?'}
							src={avatarUrl(pin.author?.avatar_id, pin.author?.instance_domain)}
							size="sm"
						/>
						<span class="text-sm font-medium text-text-primary">
							{pin.author?.display_name ?? pin.author?.username ?? 'Unknown'}
						</span>
						<span class="text-xs text-text-muted">{formatTime(pin.created_at)}</span>
					</div>
					{#if pin.content}
						<p class="mt-1 line-clamp-3 text-sm text-text-secondary">{pin.content}</p>
					{/if}
					{#if pin.attachments?.length > 0}
						<p class="mt-1 text-xs text-text-muted">
							{pin.attachments.length} attachment{pin.attachments.length > 1 ? 's' : ''}
						</p>
					{/if}
				</button>
			{/each}
		{/if}
	</div>
</aside>
