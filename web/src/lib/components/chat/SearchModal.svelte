<script lang="ts">
	import type { Message } from '$lib/types';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import { currentGuildId } from '$lib/stores/guilds';
	import { currentChannelId } from '$lib/stores/channels';
	import Modal from '$components/common/Modal.svelte';
	import Avatar from '$components/common/Avatar.svelte';
	import { avatarUrl } from '$lib/utils/avatar';

	interface Props {
		open?: boolean;
		onclose?: () => void;
	}

	let { open = $bindable(false), onclose }: Props = $props();

	let query = $state('');
	let results = $state<Message[]>([]);
	let searching = $state(false);
	let searched = $state(false);
	let searchScope = $state<'guild' | 'channel' | 'all'>('guild');
	let inputEl: HTMLInputElement;

	// Auto-focus search input when modal opens.
	$effect(() => {
		if (open) {
			setTimeout(() => inputEl?.focus(), 100);
		} else {
			query = '';
			results = [];
			searched = false;
		}
	});

	async function handleSearch() {
		if (!query.trim()) return;
		searching = true;
		searched = true;
		results = [];

		try {
			const guildId = searchScope === 'guild' ? $currentGuildId ?? undefined : undefined;
			const channelId = searchScope === 'channel' ? $currentChannelId ?? undefined : undefined;
			results = await api.searchMessages(query.trim(), guildId, channelId);
		} catch (err: any) {
			console.error('Search failed:', err);
		} finally {
			searching = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') handleSearch();
	}

	function jumpToMessage(msg: Message) {
		onclose?.();
		// Navigate to the channel containing this message.
		const guildId = $currentGuildId;
		if (guildId) {
			goto(`/app/guilds/${guildId}/channels/${msg.channel_id}#msg-${msg.id}`);
		}
	}

	function formatTime(ts: string): string {
		const d = new Date(ts);
		return d.toLocaleDateString() + ' ' + d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
	}

	function highlightMatch(text: string, q: string): string {
		if (!q.trim() || !text) return text;
		const escaped = q.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
		return text.replace(new RegExp(`(${escaped})`, 'gi'), '<mark class="bg-yellow-500/30 text-text-primary rounded px-0.5">$1</mark>');
	}
</script>

<Modal {open} title="Search Messages" {onclose}>
	<div class="mb-4 flex gap-2">
		<input
			bind:this={inputEl}
			type="text"
			class="input flex-1"
			placeholder="Search messages..."
			bind:value={query}
			onkeydown={handleKeydown}
		/>
		<button class="btn-primary" onclick={handleSearch} disabled={searching || !query.trim()}>
			{searching ? 'Searching...' : 'Search'}
		</button>
	</div>

	<div class="mb-4 flex gap-2 text-xs">
		<button
			class="rounded px-2 py-1 transition-colors"
			class:bg-brand-500={searchScope === 'guild'}
			class:text-white={searchScope === 'guild'}
			class:bg-bg-modifier={searchScope !== 'guild'}
			class:text-text-muted={searchScope !== 'guild'}
			onclick={() => (searchScope = 'guild')}
		>
			This Server
		</button>
		<button
			class="rounded px-2 py-1 transition-colors"
			class:bg-brand-500={searchScope === 'channel'}
			class:text-white={searchScope === 'channel'}
			class:bg-bg-modifier={searchScope !== 'channel'}
			class:text-text-muted={searchScope !== 'channel'}
			onclick={() => (searchScope = 'channel')}
		>
			This Channel
		</button>
		<button
			class="rounded px-2 py-1 transition-colors"
			class:bg-brand-500={searchScope === 'all'}
			class:text-white={searchScope === 'all'}
			class:bg-bg-modifier={searchScope !== 'all'}
			class:text-text-muted={searchScope !== 'all'}
			onclick={() => (searchScope = 'all')}
		>
			Everywhere
		</button>
	</div>

	<div class="max-h-80 overflow-y-auto">
		{#if searching}
			<div class="flex items-center justify-center py-8">
				<div class="h-5 w-5 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
			</div>
		{:else if searched && results.length === 0}
			<p class="py-4 text-center text-sm text-text-muted">No results found.</p>
		{:else}
			{#each results as msg (msg.id)}
				<button
					class="mb-2 w-full rounded bg-bg-primary p-3 text-left transition-colors hover:bg-bg-modifier"
					onclick={() => jumpToMessage(msg)}
				>
					<div class="flex items-center gap-2 text-xs">
						<Avatar
							name={msg.author?.display_name ?? msg.author?.username ?? 'U'}
							src={avatarUrl(msg.author?.avatar_id, msg.author?.instance_domain)}
							size="sm"
						/>
						<span class="font-medium text-text-primary">
							{msg.author?.display_name ?? msg.author?.username ?? msg.author_id.slice(0, 8)}
						</span>
						<time class="text-text-muted">{formatTime(msg.created_at)}</time>
					</div>
					{#if msg.content}
						<p class="mt-1 text-sm text-text-secondary">
							{@html highlightMatch(msg.content, query)}
						</p>
					{/if}
				</button>
			{/each}
		{/if}
	</div>
</Modal>
