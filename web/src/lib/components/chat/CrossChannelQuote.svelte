<script lang="ts">
	import { api } from '$lib/api/client';
	import MarkdownRenderer from '$components/chat/MarkdownRenderer.svelte';
	import { guildMembers, guildRolesMap } from '$lib/stores/members';

	interface Props {
		quoteMessageId: string;
		quoteChannelId: string;
	}

	let { quoteMessageId, quoteChannelId }: Props = $props();

	let loading = $state(true);
	let error = $state(false);
	let quotedMessage = $state<{
		id: string;
		content: string | null;
		author_id: string;
		channel_id: string;
		created_at: string;
		author?: { username: string; display_name?: string | null; avatar_id?: string | null };
	} | null>(null);
	let quotedChannel = $state<{ id: string; name: string; guild_id?: string | null } | null>(null);

	$effect(() => {
		loadQuote();
	});

	async function loadQuote() {
		loading = true;
		error = false;
		try {
			const [msg, ch] = await Promise.all([
				api.getMessage(quoteChannelId, quoteMessageId),
				api.getChannel(quoteChannelId)
			]);
			quotedMessage = msg;
			quotedChannel = ch;
		} catch {
			error = true;
		} finally {
			loading = false;
		}
	}

	const authorName = $derived(
		quotedMessage?.author?.display_name ?? quotedMessage?.author?.username ?? quotedMessage?.author_id ?? 'Unknown'
	);

	const channelName = $derived(quotedChannel?.name ?? 'unknown channel');

	const truncatedContent = $derived.by(() => {
		const content = quotedMessage?.content ?? '';
		if (content.length > 200) return content.slice(0, 200) + '...';
		return content;
	});

	const timestamp = $derived(
		quotedMessage ? new Date(quotedMessage.created_at).toLocaleString() : ''
	);
</script>

<div class="mt-1 max-w-lg overflow-hidden rounded border-l-4 border-purple-500 bg-bg-secondary/60 p-2.5">
	<div class="mb-1 flex items-center gap-1.5 text-2xs text-text-muted">
		<svg class="h-3 w-3 text-purple-400" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
		</svg>
		<span>Quoted from <span class="font-medium text-purple-400">#{channelName}</span></span>
	</div>

	{#if loading}
		<div class="flex items-center gap-2 py-1">
			<div class="h-3 w-3 animate-spin rounded-full border-2 border-text-muted border-t-transparent"></div>
			<span class="text-xs text-text-muted">Loading quoted message...</span>
		</div>
	{:else if error}
		<div class="flex items-center gap-1.5 py-1 text-xs text-red-400">
			<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<circle cx="12" cy="12" r="10" />
				<path d="M15 9l-6 6M9 9l6 6" />
			</svg>
			<span>Could not load quoted message</span>
		</div>
	{:else if quotedMessage}
		<div class="flex items-baseline gap-1.5">
			<span class="text-xs font-medium text-text-primary">{authorName}</span>
			<time class="text-2xs text-text-muted">{timestamp}</time>
		</div>
		{#if truncatedContent}
			<div class="mt-0.5 text-xs text-text-secondary leading-relaxed break-words">
				<MarkdownRenderer content={truncatedContent} members={$guildMembers} roles={$guildRolesMap} />
			</div>
		{:else}
			<p class="mt-0.5 text-xs italic text-text-muted">No text content</p>
		{/if}
	{/if}
</div>
