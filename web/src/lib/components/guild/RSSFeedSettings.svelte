<script lang="ts">
	import { api } from '$lib/api/client';

	let { guildId, integrationId }: { guildId: string; integrationId: string } = $props();

	// --- Types ---
	interface RSSFeed {
		id: string;
		integration_id: string;
		feed_url: string;
		title: string | null;
		description: string | null;
		last_item_id: string | null;
		last_item_published_at: string | null;
		check_interval_seconds: number;
		last_checked_at: string | null;
		error_count: number;
		last_error: string | null;
		created_at: string;
	}

	// --- State ---
	let feeds = $state<RSSFeed[]>([]);
	let loading = $state(false);
	let error = $state('');
	let success = $state('');

	// Add feed form.
	let showAddForm = $state(false);
	let newFeedUrl = $state('');
	let newFeedTitle = $state('');
	let newCheckInterval = $state(900); // 15 min default
	let adding = $state(false);

	// Interval presets.
	const intervalPresets = [
		{ value: 300, label: '5 minutes' },
		{ value: 900, label: '15 minutes' },
		{ value: 1800, label: '30 minutes' },
		{ value: 3600, label: '1 hour' },
		{ value: 7200, label: '2 hours' },
		{ value: 21600, label: '6 hours' },
		{ value: 43200, label: '12 hours' },
		{ value: 86400, label: '24 hours' },
	];

	// --- Data Loading ---
	async function loadFeeds() {
		loading = true;
		error = '';
		try {
			feeds = await api.request('GET', `/guilds/${guildId}/integrations/${integrationId}/rss/feeds`);
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to load RSS feeds';
		} finally {
			loading = false;
		}
	}

	// --- Actions ---
	async function addFeed() {
		if (!newFeedUrl.trim()) return;
		adding = true;
		error = '';
		try {
			const feed: RSSFeed = await api.request('POST', `/guilds/${guildId}/integrations/${integrationId}/rss/feeds`, {
				feed_url: newFeedUrl.trim(),
				title: newFeedTitle.trim() || null,
				check_interval_seconds: newCheckInterval,
			});
			feeds = [feed, ...feeds];
			showAddForm = false;
			newFeedUrl = '';
			newFeedTitle = '';
			newCheckInterval = 900;
			success = 'RSS feed added successfully';
			setTimeout(() => success = '', 3000);
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to add RSS feed';
		} finally {
			adding = false;
		}
	}

	async function removeFeed(feedId: string) {
		if (!confirm('Remove this RSS feed subscription?')) return;
		try {
			await api.request('DELETE', `/guilds/${guildId}/integrations/${integrationId}/rss/feeds/${feedId}`);
			feeds = feeds.filter(f => f.id !== feedId);
			success = 'RSS feed removed';
			setTimeout(() => success = '', 3000);
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to remove feed';
		}
	}

	// --- Helpers ---
	function formatInterval(seconds: number): string {
		const preset = intervalPresets.find(p => p.value === seconds);
		if (preset) return preset.label;
		if (seconds < 3600) return `${Math.round(seconds / 60)} min`;
		return `${Math.round(seconds / 3600)} hr`;
	}

	function timeAgo(dateStr: string | null): string {
		if (!dateStr) return 'Never';
		const date = new Date(dateStr);
		const diff = Date.now() - date.getTime();
		if (diff < 60000) return 'Just now';
		if (diff < 3600000) return `${Math.round(diff / 60000)}m ago`;
		if (diff < 86400000) return `${Math.round(diff / 3600000)}h ago`;
		return `${Math.round(diff / 86400000)}d ago`;
	}

	// Load on mount.
	$effect(() => {
		loadFeeds();
	});
</script>

<div class="space-y-4">
	<div class="flex items-center justify-between">
		<h4 class="font-medium text-text-primary">RSS Feed Subscriptions</h4>
		<button class="btn-secondary text-xs" onclick={() => showAddForm = !showAddForm}>
			{showAddForm ? 'Cancel' : 'Add Feed'}
		</button>
	</div>

	{#if error}
		<div class="bg-red-500/10 border border-red-500/30 text-red-400 px-3 py-2 rounded text-sm">{error}</div>
	{/if}
	{#if success}
		<div class="bg-green-500/10 border border-green-500/30 text-green-400 px-3 py-2 rounded text-sm">{success}</div>
	{/if}

	{#if showAddForm}
		<div class="bg-bg-tertiary rounded-lg p-4 space-y-3">
			<div>
				<label class="block text-sm text-text-secondary mb-1">Feed URL</label>
				<input
					class="input w-full"
					bind:value={newFeedUrl}
					placeholder="https://example.com/feed.xml"
					type="url"
				/>
			</div>
			<div>
				<label class="block text-sm text-text-secondary mb-1">Title (optional)</label>
				<input
					class="input w-full"
					bind:value={newFeedTitle}
					placeholder="My Favorite Blog"
					maxlength="100"
				/>
			</div>
			<div>
				<label class="block text-sm text-text-secondary mb-1">Check Interval</label>
				<select class="input w-full" bind:value={newCheckInterval}>
					{#each intervalPresets as preset}
						<option value={preset.value}>{preset.label}</option>
					{/each}
				</select>
			</div>
			<button
				class="btn-primary text-sm"
				onclick={addFeed}
				disabled={adding || !newFeedUrl.trim()}
			>
				{adding ? 'Adding...' : 'Add Feed'}
			</button>
		</div>
	{/if}

	{#if loading}
		<div class="text-text-muted text-sm">Loading feeds...</div>
	{:else if feeds.length === 0}
		<div class="bg-bg-tertiary rounded-lg p-4 text-center text-text-muted text-sm">
			No RSS feeds configured. Add a feed URL to start receiving updates.
		</div>
	{:else}
		<div class="space-y-2">
			{#each feeds as feed}
				<div class="bg-bg-tertiary rounded-lg p-3 space-y-2">
					<div class="flex items-start justify-between">
						<div class="flex-1 min-w-0">
							<div class="font-medium text-text-primary text-sm truncate">
								{feed.title || feed.feed_url}
							</div>
							{#if feed.title}
								<div class="text-xs text-text-muted truncate">{feed.feed_url}</div>
							{/if}
						</div>
						<button
							class="text-xs text-red-400 hover:text-red-300 ml-2 shrink-0"
							onclick={() => removeFeed(feed.id)}
						>
							Remove
						</button>
					</div>

					<div class="flex flex-wrap gap-3 text-xs text-text-muted">
						<span>Every {formatInterval(feed.check_interval_seconds)}</span>
						<span>Last checked: {timeAgo(feed.last_checked_at)}</span>
						{#if feed.last_item_published_at}
							<span>Last item: {timeAgo(feed.last_item_published_at)}</span>
						{/if}
					</div>

					{#if feed.error_count > 0 && feed.last_error}
						<div class="bg-red-500/10 rounded px-2 py-1 text-xs text-red-400">
							Error ({feed.error_count}x): {feed.last_error}
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}

	<div class="text-xs text-text-muted">
		New items from subscribed feeds will be automatically posted to the linked channel.
		The background worker checks feeds at the configured interval.
	</div>
</div>
