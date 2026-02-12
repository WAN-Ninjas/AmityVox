<script lang="ts">
	import type { Message } from '$lib/types';
	import { api } from '$lib/api/client';
	import { currentGuildId } from '$lib/stores/guilds';
	import { currentChannelId } from '$lib/stores/channels';
	import Modal from '$components/common/Modal.svelte';

	interface Props {
		open?: boolean;
		onclose?: () => void;
	}

	let { open = false, onclose }: Props = $props();

	let query = $state('');
	let results = $state<Message[]>([]);
	let searching = $state(false);
	let searched = $state(false);
	let searchScope = $state<'guild' | 'channel' | 'all'>('guild');

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

	function formatTime(ts: string): string {
		const d = new Date(ts);
		return d.toLocaleDateString() + ' ' + d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
	}
</script>

<Modal {open} title="Search Messages" {onclose}>
	<div class="mb-4 flex gap-2">
		<input
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
			This Guild
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
			<p class="py-4 text-center text-sm text-text-muted">Searching...</p>
		{:else if searched && results.length === 0}
			<p class="py-4 text-center text-sm text-text-muted">No results found.</p>
		{:else}
			{#each results as msg (msg.id)}
				<div class="mb-2 rounded bg-bg-primary p-3">
					<div class="flex items-baseline gap-2 text-xs text-text-muted">
						<span class="font-medium text-text-primary">{msg.author_id}</span>
						<time>{formatTime(msg.created_at)}</time>
					</div>
					{#if msg.content}
						<p class="mt-1 text-sm text-text-secondary">{msg.content}</p>
					{/if}
				</div>
			{/each}
		{/if}
	</div>
</Modal>
