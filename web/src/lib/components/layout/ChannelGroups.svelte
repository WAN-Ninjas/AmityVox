<script lang="ts">
	import { api } from '$lib/api/client';
	import { onMount, onDestroy, untrack } from 'svelte';
	import DragHandle from '$components/common/DragHandle.svelte';
	import { DragController, calculateInsertionIndex } from '$lib/utils/dragDrop';
	import { currentGuildId } from '$lib/stores/guilds';
	import { textChannels, voiceChannels, forumChannels, galleryChannels, currentChannelId, threadsByParent, activeThreadId, pendingThreadOpen } from '$lib/stores/channels';
	import { unreadCounts, mentionCounts } from '$lib/stores/unreads';
	import type { Channel } from '$lib/types';
	import { goto } from '$app/navigation';
	import { addToast } from '$lib/stores/toast';
	import { unlockedChannels } from '$lib/encryption/e2eeManager';
	import { canManageChannels } from '$lib/stores/permissions';

	interface ChannelGroup {
		id: string;
		user_id: string;
		name: string;
		position: number;
		color: string;
		channels: string[];
		created_at: string;
	}

	interface Props {
		onGroupsLoaded?: (channelIds: Set<string>) => void;
		onChannelContextMenu?: (e: MouseEvent, channel: { id: string; name: string; archived: boolean }) => void;
		onThreadContextMenu?: (e: MouseEvent, thread: Channel) => void;
		onGroupsChanged?: (groups: ChannelGroup[]) => void;
		onReady?: (api: { reload: () => Promise<void> }) => void;
	}

	let { onGroupsLoaded, onChannelContextMenu, onThreadContextMenu, onGroupsChanged, onReady }: Props = $props();

	let groups = $state<ChannelGroup[]>([]);
	let loading = $state(true);
	let showCreateModal = $state(false);
	let newGroupName = $state('');
	let newGroupColor = $state('#5c6bc0');
	let creating = $state(false);

	// Edit state
	let editingGroupId = $state<string | null>(null);
	let editGroupName = $state('');
	let editGroupColor = $state('');

	// Collapsed state
	let collapsedGroups = $state<Set<string>>(new Set());

	// Report grouped channel IDs to parent whenever groups change.
	$effect(() => {
		const ids = new Set<string>();
		for (const g of groups) {
			for (const ch of g.channels) ids.add(ch);
		}
		onGroupsLoaded?.(ids);
		onGroupsChanged?.(groups);
	});

	onMount(async () => {
		await loadGroups();
		// Expose reload function to parent.
		onReady?.({ reload: loadGroups });
		// Restore collapsed state from localStorage.
		try {
			const stored = localStorage.getItem('amityvox_collapsed_channel_groups');
			if (stored) {
				const parsed = JSON.parse(stored);
				if (Array.isArray(parsed)) collapsedGroups = new Set(parsed);
			}
		} catch {
			// Ignore.
		}
	});

	async function loadGroups() {
		const guildId = $currentGuildId;
		if (!guildId) {
			loading = false;
			return;
		}
		loading = true;
		try {
			groups = await api.getChannelGroups(guildId);
		} catch {
			groups = [];
		} finally {
			loading = false;
		}
	}

	function toggleGroup(groupId: string) {
		const next = new Set(collapsedGroups);
		if (next.has(groupId)) {
			next.delete(groupId);
		} else {
			next.add(groupId);
		}
		collapsedGroups = next;
		localStorage.setItem('amityvox_collapsed_channel_groups', JSON.stringify([...next]));
	}

	async function createGroup() {
		const guildId = $currentGuildId;
		if (!newGroupName.trim() || !guildId) return;
		creating = true;
		try {
			const group = await api.createChannelGroup(guildId, {
				name: newGroupName.trim(),
				color: newGroupColor
			});
			groups = [...groups, group];
			showCreateModal = false;
			newGroupName = '';
			newGroupColor = '#5c6bc0';
			addToast('Channel group created', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to create group', 'error');
		} finally {
			creating = false;
		}
	}

	async function updateGroup(groupId: string) {
		const guildId = $currentGuildId;
		if (!editGroupName.trim() || !guildId) return;
		try {
			const updated = await api.updateChannelGroup(guildId, groupId, {
				name: editGroupName.trim(),
				color: editGroupColor
			});
			groups = groups.map(g => g.id === groupId ? updated : g);
			editingGroupId = null;
		} catch (err: any) {
			addToast(err.message || 'Failed to update group', 'error');
		}
	}

	async function deleteGroup(groupId: string) {
		const guildId = $currentGuildId;
		if (!confirm('Delete this channel group?') || !guildId) return;
		try {
			await api.deleteChannelGroup(guildId, groupId);
			groups = groups.filter(g => g.id !== groupId);
			addToast('Channel group deleted', 'info');
		} catch (err: any) {
			addToast(err.message || 'Failed to delete group', 'error');
		}
	}

	async function removeChannel(groupId: string, channelId: string) {
		const guildId = $currentGuildId;
		if (!guildId) return;
		try {
			await api.removeChannelFromGroup(guildId, groupId, channelId);
			groups = groups.map(g => {
				if (g.id === groupId) {
					return { ...g, channels: g.channels.filter(c => c !== channelId) };
				}
				return g;
			});
		} catch (err: any) {
			addToast(err.message || 'Failed to remove channel', 'error');
		}
	}

	function handleChannelClick(channelId: string) {
		const guildId = $currentGuildId;
		if (guildId) {
			goto(`/app/guilds/${guildId}/channels/${channelId}`);
		}
	}

	function handleThreadClick(thread: Channel) {
		const guildId = $currentGuildId;
		if (!guildId || !thread.parent_channel_id) return;
		pendingThreadOpen.set(thread.id);
		if ($currentChannelId !== thread.parent_channel_id) {
			goto(`/app/guilds/${guildId}/channels/${thread.parent_channel_id}`);
		}
	}

	function getFilteredThreads(channelId: string): Channel[] {
		// Hide threads if parent channel is encrypted and not unlocked.
		if (isChannelEncrypted(channelId) && !$unlockedChannels.has(channelId)) return [];
		const threads = $threadsByParent.get(channelId) ?? [];
		return threads.filter((t) => !t.archived);
	}

	const channelsMap = $derived.by(() => {
		const map = new Map<string, { id: string; name: string; channel_type: string; encrypted?: boolean }>();
		for (const ch of $textChannels) map.set(ch.id, ch);
		for (const ch of $voiceChannels) map.set(ch.id, ch);
		for (const ch of $forumChannels) map.set(ch.id, ch);
		for (const ch of $galleryChannels) map.set(ch.id, ch);
		return map;
	});

	function getChannelName(channelId: string): string {
		return channelsMap.get(channelId)?.name ?? 'Unknown Channel';
	}

	function getChannelType(channelId: string): string {
		return channelsMap.get(channelId)?.channel_type ?? 'text';
	}

	function isChannelEncrypted(channelId: string): boolean {
		return channelsMap.get(channelId)?.encrypted ?? false;
	}

	function startEdit(group: ChannelGroup) {
		editingGroupId = group.id;
		editGroupName = group.name;
		editGroupColor = group.color;
	}

	function cancelEdit() {
		editingGroupId = null;
	}

	// --- Cross-group channel drag (custom pointer-based) ---
	let isDraggingInGroup = $state(false);
	let groupContainerEls = new Map<string, HTMLElement>();

	// Channel drag state
	let chDrag = $state<{
		channelId: string;
		sourceGroupId: string;
		startX: number;
		startY: number;
		activated: boolean;
		preview: HTMLElement | null;
		indicator: HTMLElement | null;
		targetGroupId: string;
		insertIndex: number;
	} | null>(null);

	function handleChPointerDown(e: PointerEvent, channelId: string, groupId: string) {
		if (e.button !== 0 || !$canManageChannels) return;
		e.stopPropagation();
		chDrag = {
			channelId,
			sourceGroupId: groupId,
			startX: e.clientX,
			startY: e.clientY,
			activated: false,
			preview: null,
			indicator: null,
			targetGroupId: groupId,
			insertIndex: -1,
		};
	}

	function handleChPointerMove(e: PointerEvent) {
		if (!chDrag) return;

		if (!chDrag.activated) {
			const dx = e.clientX - chDrag.startX;
			const dy = e.clientY - chDrag.startY;
			if (Math.sqrt(dx * dx + dy * dy) < 5) return;
			chDrag.activated = true;
			isDraggingInGroup = true;
			createChPreview(e);
			createChIndicator();
			dimSourceChannel();
		}

		// Update preview position
		if (chDrag.preview) {
			chDrag.preview.style.left = `${e.clientX + 8}px`;
			chDrag.preview.style.top = `${e.clientY - 16}px`;
		}

		// Hit-test: which group container is the cursor over?
		let foundGroup: string | null = null;
		for (const [gid, container] of groupContainerEls) {
			const rect = container.getBoundingClientRect();
			if (e.clientY >= rect.top - 20 && e.clientY <= rect.bottom + 20) {
				foundGroup = gid;
				break;
			}
		}
		chDrag.targetGroupId = foundGroup ?? chDrag.sourceGroupId;

		// Highlight target group
		for (const [gid, container] of groupContainerEls) {
			if (gid === chDrag.targetGroupId && gid !== chDrag.sourceGroupId) {
				container.style.outline = '1px solid var(--brand-500, #5c6bc0)';
				container.style.outlineOffset = '2px';
				container.style.borderRadius = '4px';
			} else {
				container.style.outline = '';
				container.style.outlineOffset = '';
				container.style.borderRadius = '';
			}
		}

		// Calculate insertion index within target group
		const targetGroup = groups.find(g => g.id === chDrag!.targetGroupId);
		const container = groupContainerEls.get(chDrag.targetGroupId);
		if (targetGroup && container) {
			const channels = [...new Set(targetGroup.channels)];
			const rects = channels.map(cid => {
				const el = container.querySelector(`[data-channel-id="${cid}"]`) as HTMLElement | null;
				if (!el) return { top: 0, bottom: 0, height: 0 };
				const r = el.getBoundingClientRect();
				return { top: r.top, bottom: r.bottom, height: r.height };
			});

			const sourceIdx = chDrag.targetGroupId === chDrag.sourceGroupId
				? channels.indexOf(chDrag.channelId) : -1;

			chDrag.insertIndex = calculateInsertionIndex(e.clientY, rects, sourceIdx);
			updateChIndicator(container, rects);
		}
	}

	function handleChPointerUp(_e: PointerEvent) {
		if (!chDrag) return;

		if (chDrag.activated && chDrag.insertIndex >= 0) {
			if (chDrag.targetGroupId === chDrag.sourceGroupId) {
				// Same-group reorder
				let adjustedIndex = chDrag.insertIndex;
				const group = groups.find(g => g.id === chDrag!.sourceGroupId);
				if (group) {
					const channels = [...new Set(group.channels)];
					const srcIdx = channels.indexOf(chDrag.channelId);
					if (srcIdx >= 0 && chDrag.insertIndex > srcIdx) adjustedIndex--;
				}
				handleChannelReorderInGroup(chDrag.channelId, adjustedIndex, chDrag.sourceGroupId);
			} else {
				// Cross-group move
				handleChannelMoveToGroup(
					chDrag.channelId,
					chDrag.sourceGroupId,
					chDrag.targetGroupId,
					chDrag.insertIndex
				);
			}
		}

		cleanupChDrag();
	}

	function handleChPointerCancel(_e: PointerEvent) {
		cleanupChDrag();
	}

	function handleChKeyDown(e: KeyboardEvent) {
		if (e.key === 'Escape' && chDrag?.activated) {
			cleanupChDrag();
		}
	}

	function createChPreview(e: PointerEvent) {
		if (!chDrag) return;
		const container = groupContainerEls.get(chDrag.sourceGroupId);
		if (!container) return;
		const sourceEl = container.querySelector(`[data-channel-id="${chDrag.channelId}"]`) as HTMLElement | null;
		if (!sourceEl) return;

		const clone = sourceEl.cloneNode(true) as HTMLElement;
		const rect = sourceEl.getBoundingClientRect();
		clone.style.cssText = `
			position: fixed;
			width: ${rect.width}px;
			height: ${rect.height}px;
			opacity: 0.9;
			transform: scale(1.02);
			box-shadow: 0 10px 25px -5px rgba(0,0,0,0.3), 0 4px 6px -2px rgba(0,0,0,0.2);
			border: 1px solid var(--brand-500, #5c6bc0);
			border-radius: 6px;
			pointer-events: none;
			z-index: 9999;
			transition: none;
			cursor: grabbing;
		`;
		document.body.appendChild(clone);
		chDrag.preview = clone;
	}

	function createChIndicator() {
		const indicator = document.createElement('div');
		indicator.style.cssText = `
			position: absolute; left: 0; right: 0; height: 2px;
			background: var(--brand-500, #5c6bc0); border-radius: 1px;
			pointer-events: none; z-index: 50; display: none;
		`;
		const dot = (side: string) => {
			const d = document.createElement('div');
			d.style.cssText = `
				position: absolute; ${side}: -3px; top: -2px;
				width: 6px; height: 6px; border-radius: 50%;
				background: var(--brand-500, #5c6bc0);
			`;
			return d;
		};
		indicator.appendChild(dot('left'));
		indicator.appendChild(dot('right'));
		if (chDrag) chDrag.indicator = indicator;
	}

	function updateChIndicator(container: HTMLElement, rects: { top: number; bottom: number; height: number }[]) {
		if (!chDrag?.indicator) return;

		// Move indicator to current target container
		if (chDrag.indicator.parentElement !== container) {
			chDrag.indicator.remove();
			const style = getComputedStyle(container);
			if (style.position === 'static') container.style.position = 'relative';
			container.appendChild(chDrag.indicator);
		}

		if (rects.length === 0) {
			chDrag.indicator.style.display = 'none';
			return;
		}

		const containerRect = container.getBoundingClientRect();
		let y: number;
		if (chDrag.insertIndex <= 0) {
			y = rects[0].top - containerRect.top + container.scrollTop - 1;
		} else if (chDrag.insertIndex >= rects.length) {
			y = rects[rects.length - 1].bottom - containerRect.top + container.scrollTop - 1;
		} else {
			const above = rects[chDrag.insertIndex - 1];
			const below = rects[chDrag.insertIndex];
			y = (above.bottom + below.top) / 2 - containerRect.top + container.scrollTop - 1;
		}

		chDrag.indicator.style.top = `${y}px`;
		chDrag.indicator.style.display = 'block';
	}

	function dimSourceChannel() {
		if (!chDrag) return;
		const container = groupContainerEls.get(chDrag.sourceGroupId);
		if (!container) return;
		const el = container.querySelector(`[data-channel-id="${chDrag.channelId}"]`) as HTMLElement | null;
		if (el) {
			el.style.opacity = '0.3';
			el.style.transition = 'opacity 150ms ease';
		}
	}

	function cleanupChDrag() {
		if (chDrag) {
			// Restore source element
			const container = groupContainerEls.get(chDrag.sourceGroupId);
			if (container) {
				const el = container.querySelector(`[data-channel-id="${chDrag.channelId}"]`) as HTMLElement | null;
				if (el) { el.style.opacity = ''; el.style.transition = ''; }
			}
			// Remove preview and indicator
			chDrag.preview?.remove();
			chDrag.indicator?.remove();
			// Clear group highlights
			for (const c of groupContainerEls.values()) {
				c.style.outline = '';
				c.style.outlineOffset = '';
				c.style.borderRadius = '';
			}
		}
		chDrag = null;
		isDraggingInGroup = false;
	}

	onDestroy(() => {
		cleanupChDrag();
		groupListController?.destroy();
	});

	async function handleChannelReorderInGroup(sourceId: string, targetIndex: number, groupId: string) {
		const guildId = $currentGuildId;
		if (!guildId) return;
		const group = groups.find(g => g.id === groupId);
		if (!group) return;

		const channels = [...new Set(group.channels)];
		const sourceIdx = channels.indexOf(sourceId);
		if (sourceIdx === -1) return;

		channels.splice(sourceIdx, 1);
		channels.splice(targetIndex, 0, sourceId);

		const prevGroups = groups;
		groups = groups.map(g => g.id === groupId ? { ...g, channels } : g);

		try {
			await api.setChannelGroupChannels(guildId, groupId, channels);
		} catch (err: any) {
			groups = prevGroups;
			addToast(err.message || 'Failed to reorder channels', 'error');
			await loadGroups();
		}
	}

	async function handleChannelMoveToGroup(channelId: string, fromGroupId: string, toGroupId: string, insertIndex: number) {
		const guildId = $currentGuildId;
		if (!guildId) return;
		const fromGroup = groups.find(g => g.id === fromGroupId);
		const toGroup = groups.find(g => g.id === toGroupId);
		if (!fromGroup || !toGroup) return;

		// Remove from source
		const fromChannels = [...new Set(fromGroup.channels)].filter(c => c !== channelId);
		// Insert into target
		const toChannels = [...new Set(toGroup.channels)];
		toChannels.splice(insertIndex, 0, channelId);

		// Optimistic update
		const prevGroups = groups;
		groups = groups.map(g => {
			if (g.id === fromGroupId) return { ...g, channels: fromChannels };
			if (g.id === toGroupId) return { ...g, channels: toChannels };
			return g;
		});

		try {
			await Promise.all([
				api.setChannelGroupChannels(guildId, fromGroupId, fromChannels),
				api.setChannelGroupChannels(guildId, toGroupId, toChannels),
			]);
		} catch (err: any) {
			groups = prevGroups;
			addToast(err.message || 'Failed to move channel', 'error');
			await loadGroups();
		}
	}

	// --- Group reorder (pointer-based) ---
	let groupListEl = $state<HTMLElement | null>(null);
	let groupListController = $state<DragController | null>(null);

	$effect(() => {
		const el = groupListEl;
		if (!el || groups.length === 0) {
			untrack(() => {
				groupListController?.destroy();
				groupListController = null;
			});
			return;
		}
		untrack(() => {
			groupListController?.destroy();
			groupListController = new DragController({
				container: el,
				items: () => groups.map(g => g.id),
				getElement: (id) => el.querySelector(`[data-group-id="${id}"]`) as HTMLElement | null,
				canDrag: $canManageChannels,
				onDrop: handleGroupReorder,
			});
		});
		return () => {
			groupListController?.destroy();
			groupListController = null;
		};
	});

	async function handleGroupReorder(sourceId: string, targetIndex: number) {
		const guildId = $currentGuildId;
		if (!guildId) return;
		const reordered = [...groups];
		const sourceIdx = reordered.findIndex(g => g.id === sourceId);
		if (sourceIdx === -1) return;

		const [moved] = reordered.splice(sourceIdx, 1);
		reordered.splice(targetIndex, 0, moved);

		const prevGroups = groups;
		groups = reordered.map((g, i) => ({ ...g, position: i }));

		try {
			await Promise.all(
				reordered.map((g, i) => api.updateChannelGroup(guildId, g.id, { position: i }))
			);
		} catch (err: any) {
			groups = prevGroups;
			addToast(err.message || 'Failed to reorder groups', 'error');
			await loadGroups();
		}
	}

	// Forwarding pointer events from window
	function handleWindowPointerMove(e: PointerEvent) {
		handleChPointerMove(e);
		groupListController?.handlePointerMove(e);
	}
	function handleWindowPointerUp(e: PointerEvent) {
		handleChPointerUp(e);
		groupListController?.handlePointerUp(e);
	}
	function handleWindowPointerCancel(e: PointerEvent) {
		handleChPointerCancel(e);
		groupListController?.handlePointerCancel(e);
	}
	function handleWindowKeyDown(e: KeyboardEvent) {
		handleChKeyDown(e);
		groupListController?.handleKeyDown(e);
	}

	// Svelte action to track group container elements for hit-testing
	function registerGroupContainer(node: HTMLElement, groupId: string) {
		let currentId = groupId;
		groupContainerEls.set(currentId, node);
		return {
			update(newGroupId: string) {
				groupContainerEls.delete(currentId);
				currentId = newGroupId;
				groupContainerEls.set(currentId, node);
			},
			destroy() {
				groupContainerEls.delete(currentId);
			},
		};
	}
</script>

<svelte:window
	onpointermove={handleWindowPointerMove}
	onpointerup={handleWindowPointerUp}
	onpointercancel={handleWindowPointerCancel}
	onkeydown={handleWindowKeyDown}
/>

{#if !loading && groups.length > 0}
	<div bind:this={groupListEl} class="relative">
	{#each groups as group (group.id)}
		{@const uniqueChannels = [...new Set(group.channels)]}
		<div
			class="group/drag mb-1"
			data-group-id={group.id}
			onpointerdown={(e) => groupListController?.handlePointerDown(e, group.id)}
			role="group"
		>
			<!-- Group header -->
			<div class="flex items-center justify-between px-1 pt-3">
				{#if editingGroupId === group.id}
					<div class="flex flex-1 items-center gap-1">
						<input
							type="text"
							class="flex-1 rounded bg-bg-primary px-1 py-0.5 text-2xs font-bold uppercase tracking-wide text-text-primary"
							bind:value={editGroupName}
							onkeydown={(e) => {
								if (e.key === 'Enter') updateGroup(group.id);
								if (e.key === 'Escape') cancelEdit();
							}}
						/>
						<input
							type="color"
							class="h-4 w-4 cursor-pointer border-0 bg-transparent p-0"
							bind:value={editGroupColor}
						/>
						<button
							class="text-text-muted hover:text-green-400"
							onclick={() => updateGroup(group.id)}
							title="Save"
						>
							<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
								<path d="M5 13l4 4L19 7" />
							</svg>
						</button>
						<button
							class="text-text-muted hover:text-red-400"
							onclick={cancelEdit}
							title="Cancel"
						>
							<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
								<path d="M6 18L18 6M6 6l12 12" />
							</svg>
						</button>
					</div>
				{:else}
					<button
						class="flex items-center gap-1 text-2xs font-bold uppercase tracking-wide hover:text-text-secondary"
						style="color: {group.color || 'var(--text-muted)'};"
						onclick={() => toggleGroup(group.id)}
						title={collapsedGroups.has(group.id) ? 'Expand' : 'Collapse'}
					>
						<svg
							class="h-3 w-3 shrink-0 transition-transform duration-200 {collapsedGroups.has(group.id) ? '-rotate-90' : ''}"
							fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"
						>
							<path d="M19 9l-7 7-7-7" />
						</svg>
						{group.name}
						<span class="ml-1 text-text-muted">({uniqueChannels.length})</span>
					</button>
					<div class="flex items-center gap-0.5">
						<button
							class="text-text-muted hover:text-text-primary"
							onclick={() => startEdit(group)}
							title="Edit group"
						>
							<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
								<path d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
							</svg>
						</button>
						<button
							class="text-text-muted hover:text-red-400"
							onclick={() => deleteGroup(group.id)}
							title="Delete group"
						>
							<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
								<path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
							</svg>
						</button>
					</div>
				{/if}
			</div>

			<!-- Group channels -->
			{#if !collapsedGroups.has(group.id)}
				<!-- svelte-ignore binding_property_non_reactive -->
				<div class="relative" data-channel-group-id={group.id} use:registerGroupContainer={group.id}>
				{#if group.channels.length === 0}
					<p class="px-3 py-1 text-2xs text-text-muted italic">No channels in this group</p>
				{:else}
					{#each uniqueChannels as channelId (channelId)}
						{@const unread = $unreadCounts.get(channelId) ?? 0}
						{@const mentions = $mentionCounts.get(channelId) ?? 0}
						{@const channelType = getChannelType(channelId)}
						{@const encrypted = isChannelEncrypted(channelId)}
						<div
							class="group/item flex items-center"
							data-channel-id={channelId}
							onpointerdown={(e) => handleChPointerDown(e, channelId, group.id)}
						>
							{#if $canManageChannels}<DragHandle />{/if}
							<button
								class="mb-0.5 flex flex-1 items-center gap-1.5 rounded px-2 py-1.5 text-left text-sm transition-colors {$currentChannelId === channelId ? 'bg-bg-modifier text-text-primary' : unread > 0 ? 'text-text-primary font-semibold hover:bg-bg-modifier' : 'text-text-muted hover:bg-bg-modifier hover:text-text-secondary'}"
								onclick={() => handleChannelClick(channelId)}
								oncontextmenu={(e) => {
									const ch = channelsMap.get(channelId);
									if (ch) onChannelContextMenu?.(e, { id: ch.id, name: ch.name, archived: false });
								}}
							>
								{#if channelType === 'voice' || channelType === 'stage'}
									<svg class="h-4 w-4 shrink-0" fill="currentColor" viewBox="0 0 24 24">
										<path d="M12 2c-1.66 0-3 1.34-3 3v6c0 1.66 1.34 3 3 3s3-1.34 3-3V5c0-1.66-1.34-3-3-3zm5 9c0 2.76-2.24 5-5 5s-5-2.24-5-5H5c0 3.53 2.61 6.43 6 6.92V21h2v-3.08c3.39-.49 6-3.39 6-6.92h-2z" />
									</svg>
								{:else if channelType === 'forum'}
									<svg class="h-4 w-4 shrink-0" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
										<path d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z" />
									</svg>
								{:else if channelType === 'gallery'}
									<svg class="h-4 w-4 shrink-0" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
										<rect x="3" y="3" width="7" height="7" rx="1" />
										<rect x="14" y="3" width="7" height="7" rx="1" />
										<rect x="3" y="14" width="7" height="7" rx="1" />
										<rect x="14" y="14" width="7" height="7" rx="1" />
									</svg>
								{:else}
									<span class="text-lg leading-none text-brand-500 font-mono">#</span>
								{/if}
								{#if encrypted}
									{@const unlocked = $unlockedChannels.has(channelId)}
									<svg class="h-3 w-3 shrink-0 {unlocked ? 'text-green-400' : 'text-red-400'}" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" title={unlocked ? 'Encrypted (unlocked)' : 'Encrypted (locked)'}>
										{#if unlocked}
											<path stroke-linecap="round" stroke-linejoin="round" d="M13.5 10.5V6.75a4.5 4.5 0 119 0v3.75M3.75 21.75h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H3.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z" />
										{:else}
											<path stroke-linecap="round" stroke-linejoin="round" d="M16.5 10.5V6.75a4.5 4.5 0 10-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H6.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z" />
										{/if}
									</svg>
								{/if}
								<span class="flex-1 truncate">{getChannelName(channelId)}</span>
								{#if mentions > 0 && $currentChannelId !== channelId}
									<span class="ml-auto flex h-4 min-w-4 items-center justify-center rounded-full bg-red-500 px-1 text-2xs font-bold text-white">
										@{mentions > 99 ? '99+' : mentions}
									</span>
								{:else if unread > 0 && $currentChannelId !== channelId}
									<span class="ml-auto flex h-4 min-w-4 items-center justify-center rounded-full bg-text-muted px-1 text-2xs font-bold text-white">
										{unread > 99 ? '99+' : unread}
									</span>
								{/if}
							</button>
							<button
								class="mr-1 hidden shrink-0 rounded p-0.5 text-text-muted hover:text-red-400 group-hover/item:block"
								onclick={() => removeChannel(group.id, channelId)}
								title="Remove from group"
							>
								<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
									<path d="M6 18L18 6M6 6l12 12" />
								</svg>
							</button>
						</div>
						{@const threads = (channelType === 'text' || channelType === 'announcement') ? getFilteredThreads(channelId) : []}
						{#if threads.length > 0}
							<div class="ml-3 border-l border-bg-floating/50 pl-1">
								{#each threads as thread (thread.id)}
									{@const threadUnread = $unreadCounts.get(thread.id) ?? 0}
									{@const threadMentions = $mentionCounts.get(thread.id) ?? 0}
									<button
										class="mb-0.5 flex w-full items-center gap-1 rounded px-1.5 py-1 text-left text-xs transition-colors {$activeThreadId === thread.id ? 'bg-bg-modifier text-text-primary' : threadUnread > 0 ? 'text-text-primary font-semibold hover:bg-bg-modifier' : 'text-text-muted hover:bg-bg-modifier hover:text-text-secondary'}"
										onclick={() => handleThreadClick(thread)}
										oncontextmenu={(e) => { e.preventDefault(); onThreadContextMenu?.(e, thread); }}
									>
										<svg class="h-3.5 w-3.5 shrink-0 text-brand-400" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
											<path d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z" />
										</svg>
										<span class="flex-1 truncate">{thread.name}</span>
										{#if threadMentions > 0 && $activeThreadId !== thread.id}
											<span class="ml-auto flex h-3.5 min-w-3.5 items-center justify-center rounded-full bg-red-500 px-0.5 text-2xs font-bold text-white">
												@{threadMentions > 99 ? '99+' : threadMentions}
											</span>
										{:else if threadUnread > 0 && $activeThreadId !== thread.id}
											<span class="ml-auto flex h-3.5 min-w-3.5 items-center justify-center rounded-full bg-text-muted px-0.5 text-2xs font-bold text-white">
												{threadUnread > 99 ? '99+' : threadUnread}
											</span>
										{/if}
									</button>
								{/each}
							</div>
						{/if}
					{/each}
				{/if}
				</div>
			{/if}
		</div>
	{/each}
	</div>
{/if}

<!-- Create group button (shown when there are existing groups or always at bottom) -->
{#if !loading}
	<button
		class="mt-2 flex w-full items-center gap-1.5 rounded px-2 py-1 text-left text-xs text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-secondary"
		onclick={() => (showCreateModal = true)}
	>
		<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path d="M12 5v14m-7-7h14" />
		</svg>
		New Channel Group
	</button>
{/if}

<!-- Create Group Modal -->
{#if showCreateModal}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
		onclick={() => (showCreateModal = false)}
		onkeydown={(e) => e.key === 'Escape' && (showCreateModal = false)}
		role="dialog"
		aria-modal="true"
		tabindex="-1"
	>
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="w-full max-w-sm rounded-lg bg-bg-floating p-5 shadow-xl"
			onclick={(e) => e.stopPropagation()}
		>
			<h3 class="mb-4 text-base font-semibold text-text-primary">Create Channel Group</h3>

			<div class="mb-3">
				<label for="groupName" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
					Group Name
				</label>
				<input
					id="groupName"
					type="text"
					class="input w-full"
					bind:value={newGroupName}
					placeholder="My Favorites"
					maxlength="64"
					onkeydown={(e) => e.key === 'Enter' && createGroup()}
				/>
			</div>

			<div class="mb-4">
				<label for="groupColor" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
					Color
				</label>
				<div class="flex items-center gap-2">
					<input
						id="groupColor"
						type="color"
						class="h-8 w-8 cursor-pointer rounded border-0 bg-transparent p-0"
						bind:value={newGroupColor}
					/>
					<span class="text-xs text-text-muted">{newGroupColor}</span>
				</div>
			</div>

			<div class="flex justify-end gap-2">
				<button class="btn-secondary" onclick={() => (showCreateModal = false)}>Cancel</button>
				<button
					class="btn-primary"
					onclick={createGroup}
					disabled={creating || !newGroupName.trim()}
				>
					{creating ? 'Creating...' : 'Create'}
				</button>
			</div>
		</div>
	</div>
{/if}
