<script lang="ts">
	import {
		notifications,
		unreadNotificationCount,
		markAllNotificationsRead,
		clearAllNotifications,
		loadMoreNotifications,
		hasMore,
		loadingMore,
		searchNotifications,
	} from '$lib/stores/notifications';
	import { getCategoryForType } from '$lib/utils/notificationHelpers';
	import NotificationItem from '$components/common/NotificationItem.svelte';
	import Modal from '$components/common/Modal.svelte';
	import type { ServerNotification, NotificationCategory } from '$lib/types';

	type TabFilter = 'all' | 'unread' | NotificationCategory;

	let activeTab: TabFilter = $state('all');
	let searchQuery = $state('');
	let searchResults: ServerNotification[] = $state([]);
	let isSearching = $state(false);
	let showClearConfirm = $state(false);
	let searchTimeout: ReturnType<typeof setTimeout> | undefined;
	let sentinelEl = $state<HTMLElement | null>(null);

	const tabs: { id: TabFilter; label: string }[] = [
		{ id: 'all', label: 'All' },
		{ id: 'unread', label: 'Unread' },
		{ id: 'messages', label: 'Messages' },
		{ id: 'social', label: 'Social' },
		{ id: 'moderation', label: 'Moderation' },
		{ id: 'content', label: 'Content' },
	];

	const filteredNotifications = $derived.by(() => {
		if (searchQuery.trim()) return searchResults;
		const all = $notifications;
		if (activeTab === 'all') return all;
		if (activeTab === 'unread') return all.filter(n => !n.read);
		return all.filter(n => getCategoryForType(n.type) === activeTab);
	});

	let searchVersion = 0;

	function handleSearch() {
		clearTimeout(searchTimeout);
		const q = searchQuery.trim();
		if (!q) {
			searchResults = [];
			isSearching = false;
			return;
		}
		isSearching = true;
		const version = ++searchVersion;
		searchTimeout = setTimeout(async () => {
			try {
				const results = await searchNotifications(q, { limit: 50 });
				if (version === searchVersion) {
					searchResults = results;
				}
			} catch {
				if (version === searchVersion) {
					searchResults = [];
				}
			} finally {
				if (version === searchVersion) {
					isSearching = false;
				}
			}
		}, 300);
	}

	function clearSearch() {
		searchQuery = '';
		searchResults = [];
		isSearching = false;
	}

	async function handleClearAll() {
		showClearConfirm = false;
		await clearAllNotifications();
	}

	// Infinite scroll via IntersectionObserver.
	$effect(() => {
		const el = sentinelEl;
		if (!el) return;

		const observer = new IntersectionObserver(
			(entries) => {
				if (entries[0].isIntersecting && !searchQuery.trim() && !$loadingMore) {
					loadMoreNotifications();
				}
			},
			{ rootMargin: '200px' }
		);
		observer.observe(el);
		return () => observer.disconnect();
	});
</script>

<div class="flex h-full flex-col overflow-hidden">
	<!-- Header -->
	<div class="flex items-center justify-between border-b border-bg-modifier px-6 py-4">
		<div class="flex items-center gap-3">
			<button
				class="rounded p-1 text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-primary md:hidden"
				onclick={() => history.back()}
			>
				<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M15 19l-7-7 7-7" />
				</svg>
			</button>
			<h1 class="text-lg font-bold text-text-primary">Notifications</h1>
			{#if $unreadNotificationCount > 0}
				<span class="flex h-5 min-w-5 items-center justify-center rounded-full bg-red-500 px-1.5 text-2xs font-bold text-white">
					{$unreadNotificationCount > 99 ? '99+' : $unreadNotificationCount}
				</span>
			{/if}
		</div>
		<div class="flex items-center gap-2">
			{#if $notifications.length > 0}
				<button
					class="rounded px-3 py-1.5 text-xs font-medium text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-primary"
					onclick={() => markAllNotificationsRead()}
				>
					Mark all read
				</button>
				<button
					class="rounded px-3 py-1.5 text-xs font-medium text-text-muted transition-colors hover:bg-bg-modifier hover:text-red-400"
					onclick={() => (showClearConfirm = true)}
				>
					Clear all
				</button>
			{/if}
		</div>
	</div>

	<!-- Search bar -->
	<div class="border-b border-bg-modifier px-6 py-3">
		<div class="relative">
			<svg class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
			</svg>
			<input
				type="text"
				bind:value={searchQuery}
				oninput={handleSearch}
				placeholder="Search notifications..."
				class="w-full rounded-md border border-bg-modifier bg-bg-tertiary py-2 pl-9 pr-8 text-sm text-text-primary placeholder:text-text-muted focus:border-brand-500 focus:outline-none"
			/>
			{#if searchQuery}
				<button
					class="absolute right-2 top-1/2 -translate-y-1/2 rounded p-0.5 text-text-muted hover:text-text-primary"
					onclick={clearSearch}
				>
					<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			{/if}
		</div>
	</div>

	<!-- Tab bar -->
	<div class="flex gap-1 border-b border-bg-modifier px-6 py-2">
		{#each tabs as tab}
			<button
				class="rounded-md px-3 py-1.5 text-xs font-medium transition-colors {activeTab === tab.id ? 'bg-brand-500/15 text-brand-400' : 'text-text-muted hover:bg-bg-modifier hover:text-text-primary'}"
				onclick={() => { activeTab = tab.id; clearSearch(); }}
			>
				{tab.label}
				{#if tab.id === 'unread' && $unreadNotificationCount > 0}
					<span class="ml-1 text-2xs">({$unreadNotificationCount})</span>
				{/if}
			</button>
		{/each}
	</div>

	<!-- Notification list -->
	<div class="flex-1 overflow-y-auto">
		{#if isSearching}
			<div class="flex items-center justify-center py-12">
				<div class="h-6 w-6 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
			</div>
		{:else if filteredNotifications.length === 0}
			<div class="flex flex-col items-center justify-center px-4 py-16 text-center">
				<svg class="mb-4 h-16 w-16 text-text-muted/30" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
					<path d="M14.857 17.082a23.848 23.848 0 005.454-1.31A8.967 8.967 0 0118 9.75v-.7V9A6 6 0 006 9v.75a8.967 8.967 0 01-2.312 6.022c1.733.64 3.56 1.085 5.455 1.31m5.714 0a24.255 24.255 0 01-5.714 0m5.714 0a3 3 0 11-5.714 0" />
				</svg>
				<h3 class="mb-1 text-sm font-medium text-text-secondary">
					{#if searchQuery.trim()}
						No results for "{searchQuery}"
					{:else if activeTab === 'unread'}
						No unread notifications
					{:else if activeTab !== 'all'}
						No {activeTab} notifications
					{:else}
						No notifications yet
					{/if}
				</h3>
				<p class="text-xs text-text-muted">
					{#if searchQuery.trim()}
						Try a different search term.
					{:else}
						New mentions, replies, and events will appear here.
					{/if}
				</p>
			</div>
		{:else}
			{#each filteredNotifications as notification (notification.id)}
				<NotificationItem {notification} />
			{/each}

			<!-- Infinite scroll sentinel -->
			{#if $hasMore && !searchQuery.trim()}
				<div bind:this={sentinelEl} class="flex items-center justify-center py-4">
					{#if $loadingMore}
						<div class="h-5 w-5 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
					{/if}
				</div>
			{/if}
		{/if}
	</div>
</div>

<!-- Clear all confirmation modal -->
<Modal open={showClearConfirm} title="Clear all notifications" onclose={() => (showClearConfirm = false)}>
	<p class="text-sm text-text-secondary">
		Are you sure you want to clear all notifications? This action cannot be undone.
	</p>
	<div class="mt-4 flex justify-end gap-2">
		<button
			class="rounded-md bg-bg-tertiary px-4 py-2 text-sm font-medium text-text-primary transition-colors hover:bg-bg-modifier"
			onclick={() => (showClearConfirm = false)}
		>
			Cancel
		</button>
		<button
			class="rounded-md bg-red-500 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-red-600"
			onclick={handleClearAll}
		>
			Clear all
		</button>
	</div>
</Modal>
