<script lang="ts">
	import type { Message } from '$lib/types';
	import { api } from '$lib/api/client';
	import { messagesByChannel, loadMessages, appendMessage } from '$lib/stores/messages';
	import Avatar from '$components/common/Avatar.svelte';
	import { tick } from 'svelte';

	interface Props {
		threadChannel: { id: string; name: string | null; guild_id: string | null };
		parentMessage?: Message | null;
		onclose: () => void;
	}

	let { threadChannel, parentMessage = null, onclose }: Props = $props();

	let content = $state('');
	let messagesContainer: HTMLDivElement;
	let loading = $state(true);

	const threadMessages = $derived.by(() => {
		return $messagesByChannel.get(threadChannel.id) ?? [];
	});

	$effect(() => {
		loading = true;
		loadMessages(threadChannel.id).finally(() => {
			loading = false;
			tick().then(() => {
				if (messagesContainer) {
					messagesContainer.scrollTop = messagesContainer.scrollHeight;
				}
			});
		});
	});

	// Auto-scroll on new messages.
	$effect(() => {
		if (threadMessages.length > 0) {
			tick().then(() => {
				if (messagesContainer) {
					messagesContainer.scrollTop = messagesContainer.scrollHeight;
				}
			});
		}
	});

	async function sendThreadMessage() {
		if (!content.trim()) return;
		const msg = content.trim();
		content = '';

		try {
			const sent = await api.sendMessage(threadChannel.id, msg);
			appendMessage(sent);
		} catch (err) {
			content = msg;
			console.error('Failed to send thread message:', err);
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			sendThreadMessage();
		}
		if (e.key === 'Escape') onclose();
	}

	function formatTime(ts: string): string {
		return new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
	}
</script>

<div class="flex h-full w-80 shrink-0 flex-col border-l border-bg-floating bg-bg-secondary">
	<!-- Header -->
	<div class="flex h-12 items-center justify-between border-b border-bg-floating px-4">
		<div class="min-w-0">
			<h3 class="truncate text-sm font-semibold text-text-primary">Thread</h3>
			<p class="truncate text-2xs text-text-muted">{threadChannel.name ?? 'Thread'}</p>
		</div>
		<button class="rounded p-1 text-text-muted hover:text-text-primary" onclick={onclose} title="Close Thread">
			<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M6 18L18 6M6 6l12 12" />
			</svg>
		</button>
	</div>

	<!-- Parent message -->
	{#if parentMessage}
		<div class="border-b border-bg-floating bg-bg-primary/50 p-3">
			<div class="flex items-center gap-2 text-xs text-text-muted">
				<span class="font-medium text-text-primary">
					{parentMessage.author?.display_name ?? parentMessage.author?.username ?? 'Unknown'}
				</span>
				<time>{formatTime(parentMessage.created_at)}</time>
			</div>
			<p class="mt-1 text-sm text-text-secondary line-clamp-3">{parentMessage.content}</p>
		</div>
	{/if}

	<!-- Thread messages -->
	<div bind:this={messagesContainer} class="flex-1 overflow-y-auto p-2">
		{#if loading}
			<div class="flex items-center justify-center py-8">
				<div class="h-5 w-5 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
			</div>
		{:else if threadMessages.length === 0}
			<p class="py-8 text-center text-sm text-text-muted">No replies yet</p>
		{:else}
			{#each threadMessages as msg (msg.id)}
				<div class="flex gap-2 rounded p-2 hover:bg-bg-modifier/30">
					<div class="mt-0.5 shrink-0">
						<Avatar
							name={msg.author?.display_name ?? msg.author?.username ?? 'U'}
							src={msg.author?.avatar_id ? `/api/v1/files/${msg.author.avatar_id}` : null}
							size="sm"
						/>
					</div>
					<div class="min-w-0 flex-1">
						<div class="flex items-baseline gap-2">
							<span class="text-sm font-medium text-text-primary">
								{msg.author?.display_name ?? msg.author?.username ?? msg.author_id.slice(0, 8)}
							</span>
							<time class="text-2xs text-text-muted">{formatTime(msg.created_at)}</time>
						</div>
						<p class="text-sm text-text-secondary break-words whitespace-pre-wrap">{msg.content}</p>
					</div>
				</div>
			{/each}
		{/if}
	</div>

	<!-- Thread input -->
	<div class="border-t border-bg-floating p-3">
		<div class="flex items-end gap-2 rounded-lg bg-bg-modifier px-3 py-2">
			<textarea
				bind:value={content}
				onkeydown={handleKeydown}
				placeholder="Reply to thread..."
				class="max-h-24 min-h-[20px] flex-1 resize-none bg-transparent text-sm text-text-primary outline-none placeholder:text-text-muted"
				rows="1"
			></textarea>
		</div>
	</div>
</div>
