<script lang="ts">
	import { goto } from '$app/navigation';
	import { guildList } from '$lib/stores/guilds';
	import { channelList, channels } from '$lib/stores/channels';
	import { dmList } from '$lib/stores/dms';
	import { recentChannels } from '$lib/stores/navigation';

	interface Props {
		open?: boolean;
		onclose?: () => void;
	}

	let { open = $bindable(false), onclose }: Props = $props();
	let query = $state('');
	let selectedIndex = $state(0);
	let inputEl = $state<HTMLInputElement | null>(null);

	interface SwitcherItem {
		id: string;
		category: 'recent' | 'guild' | 'channel' | 'dm';
		label: string;
		description: string;
		icon: string;
		action: () => void;
	}

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
		if (lowerText.startsWith(lowerSearch)) return 0;
		if (lowerText.includes(lowerSearch)) return 1;
		return 2;
	}

	// Build recent channel items for display when query is empty.
	const recentItems = $derived.by(() => {
		const items: SwitcherItem[] = [];
		const recent = $recentChannels;
		const guilds = $guildList;
		const channelMap = $channels;
		const dms = $dmList;

		for (const entry of recent) {
			// Try to find channel in the channels store (guild channels)
			const channel = channelMap.get(entry.channelId);
			if (channel && channel.name) {
				const guild = guilds.find((g) => g.id === channel.guild_id);
				items.push({
					id: `recent-${channel.id}`,
					category: 'recent',
					label: channel.name,
					description: guild ? guild.name : 'Channel',
					icon: channel.channel_type === 'voice' || channel.channel_type === 'stage' ? 'voice' : 'hash',
					action: () => {
						if (channel.guild_id) {
							goto(`/app/guilds/${channel.guild_id}/channels/${channel.id}`);
						}
					}
				});
				continue;
			}

			// Try DM channels
			const dm = dms.find((d) => d.id === entry.channelId);
			if (dm) {
				items.push({
					id: `recent-${dm.id}`,
					category: 'recent',
					label: dm.name ?? 'Direct Message',
					description: 'Direct Message',
					icon: 'at',
					action: () => goto(`/app/dms/${dm.id}`)
				});
			}
		}

		return items;
	});

	// Build all searchable items from stores.
	const allItems = $derived.by(() => {
		const items: SwitcherItem[] = [];
		const guilds = $guildList;

		// Guilds
		for (const guild of guilds) {
			items.push({
				id: `guild-${guild.id}`,
				category: 'guild',
				label: guild.name,
				description: `${guild.member_count} members`,
				icon: 'guild',
				action: () => goto(`/app/guilds/${guild.id}`)
			});
		}

		// Channels (from all loaded guilds)
		for (const channel of $channelList) {
			if (!channel.name) continue;
			const guild = guilds.find((g) => g.id === channel.guild_id);
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

		// DM channels
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

		return items;
	});

	// Filter and group items based on query.
	const filteredItems = $derived.by(() => {
		if (!query.trim()) {
			// No query: show recent channels, then guilds, then channels
			return recentItems.length > 0
				? recentItems
				: allItems.slice(0, 15);
		}

		return allItems
			.filter((item) => fuzzyMatch(item.label, query) || fuzzyMatch(item.description, query))
			.sort((a, b) => fuzzyScore(a.label, query) - fuzzyScore(b.label, query))
			.slice(0, 20);
	});

	// Category labels for display.
	const categoryLabels: Record<string, string> = {
		recent: 'Recent Channels',
		guild: 'Servers',
		channel: 'Channels',
		dm: 'Direct Messages'
	};

	// Group filtered items by category for display.
	const groupedItems = $derived.by(() => {
		const groups: { category: string; label: string; items: SwitcherItem[] }[] = [];
		const categoryOrder = ['recent', 'guild', 'channel', 'dm'];
		const byCategory = new Map<string, SwitcherItem[]>();

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
			requestAnimationFrame(() => inputEl?.focus());
		}
	});

	// Reset selectedIndex when the filter changes.
	$effect(() => {
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

	function selectItem(item: SwitcherItem) {
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
			const el = document.querySelector('[data-switcher-selected="true"]');
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
			case 'clock':
				return '<circle cx="12" cy="12" r="10" /><path d="M12 6v6l4 2" />';
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
		aria-label="Quick switcher"
		tabindex="-1"
	>
		<div class="w-full max-w-lg overflow-hidden rounded-lg bg-bg-floating shadow-2xl">
			<!-- Search input -->
			<div class="flex items-center gap-3 border-b border-bg-modifier px-4 py-3">
				<svg class="h-5 w-5 shrink-0 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M13 10V3L4 14h7v7l9-11h-7z" />
				</svg>
				<input
					bind:this={inputEl}
					bind:value={query}
					type="text"
					class="flex-1 bg-transparent text-base text-text-primary outline-none placeholder:text-text-muted"
					placeholder="Jump to a channel or server..."
					spellcheck="false"
					autocomplete="off"
				/>
				<kbd class="hidden rounded bg-bg-primary px-1.5 py-0.5 text-xs font-mono text-text-muted sm:inline-block">ESC</kbd>
			</div>

			<!-- Results -->
			<div class="max-h-[400px] overflow-y-auto px-2 py-2">
				{#if flatItems.length === 0}
					<div class="px-4 py-8 text-center text-sm text-text-muted">
						{#if query.trim()}
							No results found for "{query}"
						{:else}
							No channels or servers found
						{/if}
					</div>
				{:else}
					{#each groupedItems as group}
						<div class="mb-1">
							<h3 class="px-2 pb-1 pt-2 text-2xs font-bold uppercase tracking-wide text-text-muted">
								{group.label}
							</h3>
							{#each group.items as item}
								{@const globalIndex = flatItems.indexOf(item)}
								{@const isSelected = globalIndex === selectedIndex}
								<button
									class="flex w-full items-center gap-3 rounded-md px-3 py-2 text-left transition-colors {isSelected ? 'bg-brand-500 text-white' : 'text-text-secondary hover:bg-bg-modifier hover:text-text-primary'}"
									data-switcher-selected={isSelected}
									onclick={() => selectItem(item)}
									onmouseenter={() => (selectedIndex = globalIndex)}
								>
									<svg
										class="h-4 w-4 shrink-0 {isSelected ? 'text-white' : 'text-text-muted'}"
										fill={item.icon === 'guild' || item.icon === 'voice' ? 'currentColor' : 'none'}
										stroke={item.icon === 'guild' || item.icon === 'voice' ? 'none' : 'currentColor'}
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
					Jump
				</span>
				<span class="flex items-center gap-1 text-xs text-text-muted">
					<kbd class="rounded bg-bg-primary px-1 py-0.5 font-mono text-2xs">ESC</kbd>
					Close
				</span>
			</div>
		</div>
	</div>
{/if}
