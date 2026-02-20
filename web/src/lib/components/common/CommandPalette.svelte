<script lang="ts">
	import { goto } from '$app/navigation';
	import { guildList, currentGuildId } from '$lib/stores/guilds';
	import { channelList } from '$lib/stores/channels';
	import { dmList } from '$lib/stores/dms';
	import { markAllRead } from '$lib/stores/unreads';
	import { currentUser } from '$lib/stores/auth';
	import { api } from '$lib/api/client';
	import { getGatewayClient } from '$lib/stores/gateway';

	interface Props {
		open?: boolean;
		onclose?: () => void;
	}

	let { open = $bindable(false), onclose }: Props = $props();
	let query = $state('');
	let selectedIndex = $state(0);
	let inputEl = $state<HTMLInputElement | null>(null);

	// Category type for search results.
	interface PaletteItem {
		id: string;
		category: 'channel' | 'guild' | 'dm' | 'action';
		label: string;
		description: string;
		icon: string;
		action: () => void;
	}

	// Build the static actions list.
	const staticActions: PaletteItem[] = [
		{
			id: 'action-settings',
			category: 'action',
			label: 'Go to Settings',
			description: 'Open user settings',
			icon: 'settings',
			action: () => goto('/app/settings')
		},
		{
			id: 'action-dnd',
			category: 'action',
			label: 'Toggle Do Not Disturb',
			description: 'Set your status to DND',
			icon: 'dnd',
			action: () => {
				const client = getGatewayClient();
				const user = $currentUser;
				if (user && client) {
					const newStatus = user.status_presence === 'dnd' ? 'online' : 'dnd';
					api.updateMe({ status_presence: newStatus });
					client.updatePresence(newStatus);
				}
			}
		},
		{
			id: 'action-mark-read',
			category: 'action',
			label: 'Mark All as Read',
			description: 'Clear all unread messages',
			icon: 'check',
			action: () => markAllRead()
		},
		{
			id: 'action-create-guild',
			category: 'action',
			label: 'Create Server',
			description: 'Create a new server',
			icon: 'plus',
			action: () => goto('/app')
		},
		{
			id: 'action-friends',
			category: 'action',
			label: 'Friends',
			description: 'View your friends list',
			icon: 'users',
			action: () => goto('/app/friends')
		},
		{
			id: 'action-discover',
			category: 'action',
			label: 'Discover Servers',
			description: 'Browse public servers',
			icon: 'search',
			action: () => goto('/app/discover')
		},
		{
			id: 'action-bookmarks',
			category: 'action',
			label: 'Saved Messages',
			description: 'View bookmarked messages',
			icon: 'bookmark',
			action: () => goto('/app/bookmarks')
		},
		{
			id: 'action-home',
			category: 'action',
			label: 'Go Home',
			description: 'Return to the home screen',
			icon: 'home',
			action: () => goto('/app')
		}
	];

	// Fuzzy match: checks if all query characters appear in the string in order.
	function fuzzyMatch(text: string, search: string): boolean {
		if (!search) return true;
		const lowerText = text.toLowerCase();
		const lowerSearch = search.toLowerCase();
		let ti = 0;
		for (let si = 0; si < lowerSearch.length; si++) {
			const found = lowerText.indexOf(lowerSearch[si], ti);
			if (found === -1) return false;
			ti = found + 1;
		}
		return true;
	}

	// Score a fuzzy match (lower is better).
	function fuzzyScore(text: string, search: string): number {
		if (!search) return 0;
		const lowerText = text.toLowerCase();
		const lowerSearch = search.toLowerCase();
		// Exact prefix match is best.
		if (lowerText.startsWith(lowerSearch)) return 0;
		// Contains as substring is next best.
		if (lowerText.includes(lowerSearch)) return 1;
		// Otherwise, fuzzy match gets a higher score.
		return 2;
	}

	// Build all items from stores + static actions.
	const allItems = $derived.by(() => {
		const items: PaletteItem[] = [];

		// Channels from current guild.
		const guildId = $currentGuildId;
		for (const channel of $channelList) {
			if (!channel.name) continue;
			const guild = $guildList.find((g) => g.id === channel.guild_id);
			items.push({
				id: `channel-${channel.id}`,
				category: 'channel',
				label: channel.name,
				description: guild ? guild.name : 'Channel',
				icon: channel.channel_type === 'voice' || channel.channel_type === 'stage' ? 'voice' : 'hash',
				action: () => {
					if (channel.guild_id) {
						goto(`/app/guilds/${channel.guild_id}/channels/${channel.id}`);
					}
				}
			});
		}

		// DM channels.
		for (const dm of $dmList) {
			const name = dm.name ?? 'Direct Message';
			items.push({
				id: `dm-${dm.id}`,
				category: 'dm',
				label: name,
				description: 'Direct Message',
				icon: 'at',
				action: () => goto(`/app/dms/${dm.id}`)
			});
		}

		// Guilds.
		for (const guild of $guildList) {
			items.push({
				id: `guild-${guild.id}`,
				category: 'guild',
				label: guild.name,
				description: `${guild.member_count} members`,
				icon: 'guild',
				action: () => goto(`/app/guilds/${guild.id}`)
			});
		}

		// Actions.
		items.push(...staticActions);

		return items;
	});

	// Filter items based on query.
	const filteredItems = $derived.by(() => {
		if (!query.trim()) {
			// Show a sensible default: recent channels then guilds then actions.
			return allItems.slice(0, 20);
		}

		return allItems
			.filter((item) => fuzzyMatch(item.label, query) || fuzzyMatch(item.description, query))
			.sort((a, b) => fuzzyScore(a.label, query) - fuzzyScore(b.label, query))
			.slice(0, 20);
	});

	// Category labels for display.
	const categoryLabels: Record<string, string> = {
		channel: 'Channels',
		guild: 'Servers',
		dm: 'Direct Messages',
		action: 'Actions'
	};

	// Group filtered items by category for display.
	const groupedItems = $derived.by(() => {
		const groups: { category: string; label: string; items: PaletteItem[] }[] = [];
		const categoryOrder = ['channel', 'guild', 'dm', 'action'];
		const byCategory = new Map<string, PaletteItem[]>();

		for (const item of filteredItems) {
			if (!byCategory.has(item.category)) {
				byCategory.set(item.category, []);
			}
			byCategory.get(item.category)!.push(item);
		}

		for (const cat of categoryOrder) {
			const items = byCategory.get(cat);
			if (items && items.length > 0) {
				groups.push({ category: cat, label: categoryLabels[cat] ?? cat, items });
			}
		}

		return groups;
	});

	// Flat list of items (for arrow key navigation).
	const flatItems = $derived(groupedItems.flatMap((g) => g.items));

	// Reset state when opening.
	$effect(() => {
		if (open) {
			query = '';
			selectedIndex = 0;
			// Focus input after next frame.
			requestAnimationFrame(() => inputEl?.focus());
		}
	});

	// Reset selectedIndex when the filter changes.
	$effect(() => {
		// Reference filteredItems to track it.
		filteredItems;
		selectedIndex = 0;
	});

	function handleKeydown(e: KeyboardEvent) {
		switch (e.key) {
			case 'ArrowDown':
				e.preventDefault();
				selectedIndex = Math.min(selectedIndex + 1, flatItems.length - 1);
				scrollSelectedIntoView();
				break;
			case 'ArrowUp':
				e.preventDefault();
				selectedIndex = Math.max(selectedIndex - 1, 0);
				scrollSelectedIntoView();
				break;
			case 'Enter':
				e.preventDefault();
				if (flatItems[selectedIndex]) {
					selectItem(flatItems[selectedIndex]);
				}
				break;
			case 'Escape':
				e.preventDefault();
				close();
				break;
		}
	}

	function selectItem(item: PaletteItem) {
		close();
		item.action();
	}

	function close() {
		open = false;
		onclose?.();
	}

	function handleBackdrop(e: MouseEvent) {
		if (e.target === e.currentTarget) close();
	}

	function scrollSelectedIntoView() {
		requestAnimationFrame(() => {
			const el = document.querySelector('[data-palette-selected="true"]');
			el?.scrollIntoView({ block: 'nearest' });
		});
	}

	function getIconSvg(icon: string): string {
		switch (icon) {
			case 'hash':
				return '<path d="M4 9h16M4 15h16M10 3l-2 18M16 3l-2 18" />';
			case 'voice':
				return '<path d="M12 2c-1.66 0-3 1.34-3 3v6c0 1.66 1.34 3 3 3s3-1.34 3-3V5c0-1.66-1.34-3-3-3zm5 9c0 2.76-2.24 5-5 5s-5-2.24-5-5H5c0 3.53 2.61 6.43 6 6.92V21h2v-3.08c3.39-.49 6-3.39 6-6.92h-2z" />';
			case 'at':
				return '<circle cx="12" cy="12" r="4" /><path d="M16 8v5a3 3 0 0 0 6 0v-1a10 10 0 1 0-3.92 7.94" />';
			case 'guild':
				return '<path d="M18 10h-1.26A8 8 0 1 0 9 20h9a5 5 0 0 0 0-10z" />';
			case 'settings':
				return '<path d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" /><circle cx="12" cy="12" r="3" />';
			case 'dnd':
				return '<circle cx="12" cy="12" r="10" /><path d="M8 12h8" />';
			case 'check':
				return '<path d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />';
			case 'plus':
				return '<path d="M12 5v14m-7-7h14" />';
			case 'users':
				return '<path d="M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5z" />';
			case 'search':
				return '<path d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />';
			case 'bookmark':
				return '<path d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z" />';
			case 'home':
				return '<path d="M10 20v-6h4v6h5v-8h3L12 3 2 12h3v8z" />';
			default:
				return '<path d="M12 5v14m-7-7h14" />';
		}
	}
</script>

{#if open}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-[100] flex items-start justify-center bg-black/60 pt-[15vh]"
		onclick={handleBackdrop}
		onkeydown={handleKeydown}
		role="dialog"
		aria-modal="true"
		aria-label="Command palette"
		tabindex="-1"
	>
		<div class="w-full max-w-lg overflow-hidden rounded-lg bg-bg-floating shadow-2xl">
			<!-- Search input -->
			<div class="flex items-center gap-3 border-b border-bg-modifier px-4 py-3">
				<svg class="h-5 w-5 shrink-0 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
				</svg>
				<input
					bind:this={inputEl}
					bind:value={query}
					type="text"
					class="flex-1 bg-transparent text-base text-text-primary outline-none placeholder:text-text-muted"
					placeholder="Search channels, servers, or actions..."
					spellcheck="false"
					autocomplete="off"
				/>
				<kbd class="hidden rounded bg-bg-primary px-1.5 py-0.5 text-xs font-mono text-text-muted sm:inline-block">ESC</kbd>
			</div>

			<!-- Results -->
			<div class="max-h-[400px] overflow-y-auto px-2 py-2">
				{#if flatItems.length === 0}
					<div class="px-4 py-8 text-center text-sm text-text-muted">
						No results found for "{query}"
					</div>
				{:else}
					{#each groupedItems as group}
						<div class="mb-1">
							<h3 class="px-2 pb-1 pt-2 text-2xs font-bold uppercase tracking-wide text-text-muted">
								{group.label}
							</h3>
							{#each group.items as item, i}
								{@const globalIndex = flatItems.indexOf(item)}
								{@const isSelected = globalIndex === selectedIndex}
								<button
									class="flex w-full items-center gap-3 rounded-md px-3 py-2 text-left transition-colors {isSelected ? 'bg-brand-500 text-white' : 'text-text-secondary hover:bg-bg-modifier hover:text-text-primary'}"
									data-palette-selected={isSelected}
									onclick={() => selectItem(item)}
									onmouseenter={() => (selectedIndex = globalIndex)}
								>
									<svg
										class="h-4 w-4 shrink-0 {isSelected ? 'text-white' : 'text-text-muted'}"
										fill={item.icon === 'guild' || item.icon === 'users' || item.icon === 'voice' ? 'currentColor' : 'none'}
										stroke={item.icon === 'guild' || item.icon === 'users' || item.icon === 'voice' ? 'none' : 'currentColor'}
										stroke-width="2"
										viewBox="0 0 24 24"
									>
										{@html getIconSvg(item.icon)}
									</svg>
									<div class="min-w-0 flex-1">
										<span class="block truncate text-sm font-medium">{item.label}</span>
									</div>
									<span class="shrink-0 text-xs {isSelected ? 'text-white/70' : 'text-text-muted'}">{item.description}</span>
								</button>
							{/each}
						</div>
					{/each}
				{/if}
			</div>

			<!-- Footer hints -->
			<div class="flex items-center gap-4 border-t border-bg-modifier px-4 py-2">
				<span class="flex items-center gap-1 text-xs text-text-muted">
					<kbd class="rounded bg-bg-primary px-1 py-0.5 font-mono text-2xs">&#8593;&#8595;</kbd>
					Navigate
				</span>
				<span class="flex items-center gap-1 text-xs text-text-muted">
					<kbd class="rounded bg-bg-primary px-1 py-0.5 font-mono text-2xs">&#9166;</kbd>
					Select
				</span>
				<span class="flex items-center gap-1 text-xs text-text-muted">
					<kbd class="rounded bg-bg-primary px-1 py-0.5 font-mono text-2xs">ESC</kbd>
					Close
				</span>
			</div>
		</div>
	</div>
{/if}
