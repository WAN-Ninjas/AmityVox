<script lang="ts">
	import { currentGuild, currentGuildId } from '$lib/stores/guilds';
	import { channelList, textChannels, voiceChannels, forumChannels, galleryChannels, currentChannelId, setChannel, updateChannel as updateChannelStore, removeChannel as removeChannelStore, threadsByParent, hideThread as hideThreadStore, getThreadActivityFilter, setThreadActivityFilter, pendingThreadOpen, activeThreadId, editChannelSignal } from '$lib/stores/channels';
	import { channelVoiceUsers, voiceChannelId, joinVoice } from '$lib/stores/voice';
	import { currentUser } from '$lib/stores/auth';
	import { guildEventsByGuild, loadGuildEvents } from '$lib/stores/guildEvents';
	import Avatar from '$components/common/Avatar.svelte';
	import { presenceMap } from '$lib/stores/presence';
	import { dmList, removeDMChannel } from '$lib/stores/dms';
	import { unreadCounts, mentionCounts, markAllRead, totalUnreads } from '$lib/stores/unreads';
	import { addToast } from '$lib/stores/toast';
	import { confirmAction } from '$lib/stores/confirm';
	import { pendingIncomingCount, relationships, addOrUpdateRelationship } from '$lib/stores/relationships';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import { onMount, untrack } from 'svelte';
	import InviteModal from '$components/guild/InviteModal.svelte';
	import ChannelContextMenu from '$components/layout/ChannelContextMenu.svelte';
	import ChannelGroups from '$components/layout/ChannelGroups.svelte';
	import { e2ee, unlockedChannels } from '$lib/encryption/e2eeManager';
	import VoiceConnectionBar from '$components/layout/VoiceConnectionBar.svelte';
	import { getDMDisplayName, getDMRecipient } from '$lib/utils/dm';
	import { avatarUrl } from '$lib/utils/avatar';
	import { canManageChannels, canManageGuild, canManageThreads } from '$lib/stores/permissions';
	import { isChannelMuted } from '$lib/stores/muting';
	import GroupDMCreateModal from '$components/common/GroupDMCreateModal.svelte';
	import ProfileModal from '$components/common/ProfileModal.svelte';
	import CreateChannelModal from '$components/layout/CreateChannelModal.svelte';
	import DMContextMenu from '$components/layout/DMContextMenu.svelte';
	import EditChannelModal from '$components/layout/EditChannelModal.svelte';
	import GuildContextMenu from '$components/layout/GuildContextMenu.svelte';
	import ReportIssueModal from '$components/layout/ReportIssueModal.svelte';
	import ThreadContextMenu from '$components/layout/ThreadContextMenu.svelte';
	import UserPanel from '$components/layout/UserPanel.svelte';
	import type { Channel } from '$lib/types';
	import { getErrorMessage } from '$lib/utils/apiError';
	import DragHandle from '$components/common/DragHandle.svelte';
	import FederationBadge from '$components/common/FederationBadge.svelte';
	import { DragController, calculateInsertionIndex } from '$lib/utils/dragDrop';
	import { onDestroy } from 'svelte';

	let dmProfileUserId = $state<string | null>(null);

	interface Props {
		/** Width in pixels, controlled by the layout store / resize handle. */
		width?: number;
	}

	let { width = 224 }: Props = $props();

	// Group DM creation modal
	let showGroupDMCreate = $state(false);

	// Report issue modal
	let showReportIssue = $state(false);

	const upcomingEvents = $derived(
		$currentGuildId ? ($guildEventsByGuild.get($currentGuildId) ?? []).slice(0, 3) : []
	);

	// Archived channels
	let showArchived = $state(false);

	// Collapsible sidebar sections -- persisted to localStorage.
	const COLLAPSED_STORAGE_KEY = 'amityvox_collapsed_categories';
	let collapsedSections = $state<Set<string>>(new Set());

	onMount(() => {
		try {
			const stored = localStorage.getItem(COLLAPSED_STORAGE_KEY);
			if (stored) {
				const parsed = JSON.parse(stored);
				if (Array.isArray(parsed)) {
					collapsedSections = new Set(parsed);
				}
			}
		} catch {
			// Ignore malformed JSON.
		}
	});

	function toggleSection(sectionId: string) {
		const next = new Set(collapsedSections);
		if (next.has(sectionId)) {
			next.delete(sectionId);
		} else {
			next.add(sectionId);
		}
		collapsedSections = next;
		localStorage.setItem(COLLAPSED_STORAGE_KEY, JSON.stringify([...next]));
	}

	function isSectionCollapsed(sectionId: string): boolean {
		return collapsedSections.has(sectionId);
	}

	const activeTextChannels = $derived($textChannels.filter(c => !c.archived));
	const activeVoiceChannels = $derived($voiceChannels.filter(c => !c.archived));
	const archivedChannels = $derived([...$textChannels, ...$voiceChannels].filter(c => c.archived));

	// Channels that belong to a channel group (reported by ChannelGroups).
	let groupedChannelIds = $state<Set<string>>(new Set());

	// Full groups data for "Move to Group" context menu.
	let channelGroupsData = $state<{ id: string; name: string; color: string; channels: string[] }[]>([]);

	// Reload function exposed by ChannelGroups via onReady.
	let reloadChannelGroups: (() => Promise<void>) | null = null;

	// Find which group a channel belongs to (if any).
	function findChannelGroup(channelId: string): { id: string; name: string } | null {
		for (const g of channelGroupsData) {
			if (g.channels.includes(channelId)) return { id: g.id, name: g.name };
		}
		return null;
	}

	// Move to Group submenu state
	async function addChannelToGroup(groupId: string, channelId: string, insertIndex?: number) {
		const guildId = $currentGuildId;
		if (!guildId) return;
		const group = channelGroupsData.find(g => g.id === groupId);
		if (!group) return;
		// Build new channel list with insertion at the specified index.
		const existing = [...new Set(group.channels)].filter(c => c !== channelId);
		const idx = insertIndex != null ? Math.min(insertIndex, existing.length) : existing.length;
		existing.splice(idx, 0, channelId);
		try {
			await api.setChannelGroupChannels(guildId, groupId, existing);
			addToast('Channel moved to group', 'success');
			await reloadChannelGroups?.();
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to move channel'), 'error');
		}
		closeContextMenu();
	}

	async function removeChannelFromGroupCtx(channelId: string) {
		const guildId = $currentGuildId;
		if (!guildId) return;
		const group = findChannelGroup(channelId);
		if (!group) return;
		try {
			await api.removeChannelFromGroup(guildId, group.id, channelId);
			addToast('Channel removed from group', 'success');
			// Reload ChannelGroups so its internal state is in sync.
			await reloadChannelGroups?.();
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to remove channel from group'), 'error');
		}
		closeContextMenu();
	}

	const ungroupedTextChannels = $derived(activeTextChannels.filter(c => !groupedChannelIds.has(c.id)));
	const ungroupedForumChannels = $derived($forumChannels.filter(c => !groupedChannelIds.has(c.id)));
	const ungroupedGalleryChannels = $derived($galleryChannels.filter(c => !groupedChannelIds.has(c.id)));
	const ungroupedVoiceChannels = $derived(activeVoiceChannels.filter(c => !groupedChannelIds.has(c.id)));

	async function handleArchiveChannel(channelId: string, archive: boolean) {
		try {
			const updated = await api.updateChannel(channelId, { archived: archive });
			updateChannelStore(updated);
		} catch (err: unknown) {
			addToast(getErrorMessage(err, `Failed to ${archive ? 'archive' : 'unarchive'} channel`), 'error');
		}
	}

	$effect(() => {
		const gid = $currentGuildId;
		if (gid) {
			loadGuildEvents(gid).catch(() => {});
		}
	});

	// Refresh E2EE key status for all encrypted channels visible in sidebar.
	$effect(() => {
		const encIds = [
			...$textChannels.filter(c => c.encrypted).map(c => c.id),
			...$dmList.filter(c => c.encrypted).map(c => c.id),
		];
		if (encIds.length > 0) {
			e2ee.refreshKeyStatus(encIds);
		}
	});

	// Watch editChannelSignal from TopBar gear icon.
	$effect(() => {
		const channelId = $editChannelSignal;
		if (channelId) {
			const allChannels = [...$textChannels, ...$voiceChannels, ...$forumChannels, ...$galleryChannels];
			const ch = allChannels.find(c => c.id === channelId);
			if (ch) {
				openEditModal(channelId, ch.name ?? '');
			}
			editChannelSignal.set(null);
		}
	});

	function formatEventDate(dateStr: string): string {
		const d = new Date(dateStr);
		const now = new Date();
		const diffMs = d.getTime() - now.getTime();
		const diffH = Math.floor(diffMs / 3600000);
		if (diffH < 1) return 'Starting soon';
		if (diffH < 24) return `In ${diffH}h`;
		const diffD = Math.floor(diffH / 24);
		if (diffD === 1) return 'Tomorrow';
		return d.toLocaleDateString([], { month: 'short', day: 'numeric' });
	}

	// Create channel modal
	let showCreateChannel = $state(false);

	// Edit channel modal
	let showEditChannel = $state(false);
	let editChannel = $state<Channel | null>(null);

	// Invite modal
	let showInvite = $state(false);

	// Context menu (channel)
	let channelContextMenu = $state<{ x: number; y: number; channelId: string; channelName: string; archived: boolean } | null>(null);

	// Thread context menu
	let threadContextMenu = $state<{ x: number; y: number; thread: Channel } | null>(null);

	// DM context menu
	let dmContextMenu = $state<{ x: number; y: number; channel: Channel } | null>(null);

	// Guild context menu
	let guildContextMenu = $state<{ x: number; y: number } | null>(null);

	// Thread activity filter state — triggers reactivity when changed.
	let threadFilterVersion = $state(0);

	function getFilteredThreads(channelId: string): Channel[] {
		// Access threadFilterVersion to trigger reactivity.
		void threadFilterVersion;
		// Hide threads if the parent channel is encrypted and not unlocked.
		const parentChannel = $textChannels.find(c => c.id === channelId);
		if (parentChannel?.encrypted && !$unlockedChannels.has(channelId)) return [];
		const threads = $threadsByParent.get(channelId) ?? [];
		const filterMinutes = getThreadActivityFilter(channelId);

		return threads.filter((t) => {
			// Never show archived threads.
			if (t.archived) return false;
			// Always show threads with unreads regardless of filter.
			const unread = $unreadCounts.get(t.id) ?? 0;
			const mentions = $mentionCounts.get(t.id) ?? 0;
			if (unread > 0 || mentions > 0) return true;
			// Apply activity time filter.
			if (filterMinutes === null) return true; // "All" — no filter.
			if (!t.last_activity_at) return false;
			const activityTime = new Date(t.last_activity_at).getTime();
			const cutoff = Date.now() - filterMinutes * 60 * 1000;
			return activityTime >= cutoff;
		});
	}

	function handleSetThreadFilter(channelId: string, minutes: number | null) {
		setThreadActivityFilter(channelId, minutes);
		threadFilterVersion++;
	}

	async function handleHideThread(thread: Channel) {
		if (!thread.parent_channel_id) return;
		try {
			await hideThreadStore(thread.parent_channel_id, thread.id);
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to hide thread'), 'error');
		} finally {
			threadContextMenu = null;
		}
	}

	async function handleArchiveThread(thread: Channel, archive: boolean) {
		try {
			const updated = await api.updateChannel(thread.id, { archived: archive });
			updateChannelStore(updated);
		} catch (err: unknown) {
			addToast(getErrorMessage(err, `Failed to ${archive ? 'archive' : 'unarchive'} thread`), 'error');
		}
		threadContextMenu = null;
	}

	async function handleDeleteThread(thread: Channel) {
		if (!(await confirmAction({ title: 'Delete Thread', message: `Delete thread "${thread.name}"? This will permanently remove the thread and all its messages.`, confirmLabel: 'Delete Thread' }))) return;
		try {
			await api.deleteChannel(thread.id);
			removeChannelStore(thread.id);
			addToast('Thread deleted', 'info');
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to delete thread'), 'error');
		}
		threadContextMenu = null;
	}

	function handleChannelClick(channelId: string) {
		const guildId = $currentGuildId;
		if (!guildId) return;
		// Close any open thread panel.
		pendingThreadOpen.set('__close__');
		goto(`/app/guilds/${guildId}/channels/${channelId}`);
	}

	function handleThreadClick(thread: Channel) {
		const guildId = $currentGuildId;
		if (!guildId || !thread.parent_channel_id) return;
		// Signal the channel page to open this thread in the side panel.
		pendingThreadOpen.set(thread.id);
		// Navigate to the parent channel (if not already there).
		if ($currentChannelId !== thread.parent_channel_id) {
			goto(`/app/guilds/${guildId}/channels/${thread.parent_channel_id}`);
		}
	}

	async function handleDeleteChannel(channelId: string) {
		if (!(await confirmAction({ title: 'Delete Channel', message: 'Are you sure you want to delete this channel?', confirmLabel: 'Delete Channel' }))) return;
		try {
			await api.deleteChannel(channelId);
			removeChannelStore(channelId);
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to delete channel'), 'error');
		}
	}

	function openContextMenu(e: MouseEvent, channel: Channel) {
		e.preventDefault();
		channelContextMenu = { x: e.clientX, y: e.clientY, channelId: channel.id, channelName: channel.name ?? '', archived: channel.archived };
		dmContextMenu = null;
		threadContextMenu = null;
	}

	function openThreadContextMenu(e: MouseEvent, thread: Channel) {
		e.preventDefault();
		threadContextMenu = { x: e.clientX, y: e.clientY, thread };
		channelContextMenu = null;
		dmContextMenu = null;
	}

	function closeContextMenu() {
		channelContextMenu = null;
		threadContextMenu = null;
		guildContextMenu = null;
	}

	function markDMRead(channelId: string) {
		api.ackChannel(channelId).catch((err) => console.error('Failed to mark DM as read:', err));
	}

	async function addFriendFromDM(channel: Channel) {
		const recipient = getDMRecipient(channel, $currentUser?.id);
		if (!recipient) return;
		try {
			const rel = await api.addFriend(recipient.id);
			addOrUpdateRelationship(rel);
			addToast(rel.type === 'friend' ? 'Friend request accepted!' : 'Friend request sent!', 'success');
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to send friend request'), 'error');
		}
	}

	async function closeDM(channelId: string) {
		try {
			await api.deleteChannel(channelId);
			removeDMChannel(channelId);
			if ($currentChannelId === channelId) {
				goto('/app/friends');
			}
		} catch (err) {
			console.error('Failed to close DM:', err);
			addToast('Failed to close DM', 'error');
		}
	}

	function openEditModal(channelId: string, channelName: string) {
		const allChannels = [...$textChannels, ...$voiceChannels];
		editChannel = allChannels.find(c => c.id === channelId) ?? {
			id: channelId,
			name: channelName,
			channel_type: 'text'
		} as Channel;
		showEditChannel = true;
		closeContextMenu();
	}

	// --- Channel Drag Reorder (pointer-based) ---
	let channelListEl = $state<HTMLElement | null>(null);
	let channelDragController = $state<DragController | null>(null);
	let isDraggingChannel = $state(false);

	$effect(() => {
		const el = channelListEl;
		if (!el) return;
		untrack(() => {
			channelDragController?.destroy();
			channelDragController = new DragController({
				container: el,
				items: () => [...ungroupedTextChannels, ...ungroupedForumChannels, ...ungroupedGalleryChannels].map(c => c.id),
				getElement: (id) => el.querySelector(`[data-channel-id="${id}"]`) as HTMLElement | null,
				canDrag: $canManageChannels,
				onDrop: handleChannelReorder,
				onDragStateChange: (dragging) => { isDraggingChannel = dragging; },
			});
		});
	});

	onDestroy(() => { channelDragController?.destroy(); });

	// --- External drag into group: highlight + insertion indicator ---
	let highlightedGroupEl: HTMLElement | null = null;
	let externalDropIndicator: HTMLElement | null = null;
	let externalDropInsertIndex = -1;

	function clearGroupHighlight() {
		if (highlightedGroupEl) {
			highlightedGroupEl.style.outline = '';
			highlightedGroupEl.style.outlineOffset = '';
			highlightedGroupEl.style.borderRadius = '';
			highlightedGroupEl = null;
		}
		if (externalDropIndicator) {
			externalDropIndicator.remove();
			externalDropIndicator = null;
		}
		externalDropInsertIndex = -1;
	}

	function findGroupAtPoint(x: number, y: number): HTMLElement | null {
		const els = document.elementsFromPoint(x, y);
		for (const el of els) {
			const groupEl = (el as HTMLElement).closest?.('[data-channel-group-id]');
			if (groupEl) return groupEl as HTMLElement;
		}
		return null;
	}

	function ensureDropIndicator(container: HTMLElement): HTMLElement {
		if (externalDropIndicator && externalDropIndicator.parentElement === container) {
			return externalDropIndicator;
		}
		externalDropIndicator?.remove();
		const indicator = document.createElement('div');
		indicator.style.cssText = `
			position: absolute; left: 0; right: 0; height: 2px;
			background: var(--brand-500, #5c6bc0); border-radius: 1px;
			pointer-events: none; z-index: 50; display: none;
		`;
		const makeDot = (side: string) => {
			const d = document.createElement('div');
			d.style.cssText = `
				position: absolute; ${side}: -3px; top: -2px;
				width: 6px; height: 6px; border-radius: 50%;
				background: var(--brand-500, #5c6bc0);
			`;
			return d;
		};
		indicator.appendChild(makeDot('left'));
		indicator.appendChild(makeDot('right'));
		const style = getComputedStyle(container);
		if (style.position === 'static') container.style.position = 'relative';
		container.appendChild(indicator);
		externalDropIndicator = indicator;
		return indicator;
	}

	function updateDropIndicatorPosition(container: HTMLElement, cursorY: number) {
		const channelEls = container.querySelectorAll<HTMLElement>('[data-channel-id]');
		const rects = Array.from(channelEls).map(el => {
			const r = el.getBoundingClientRect();
			return { top: r.top, bottom: r.bottom, height: r.height };
		});

		externalDropInsertIndex = calculateInsertionIndex(cursorY, rects, -1);
		const indicator = ensureDropIndicator(container);

		if (rects.length === 0) {
			// Empty group — show line at the top.
			indicator.style.top = '4px';
			indicator.style.display = 'block';
			externalDropInsertIndex = 0;
			return;
		}

		const containerRect = container.getBoundingClientRect();
		let y: number;
		if (externalDropInsertIndex <= 0) {
			y = rects[0].top - containerRect.top + container.scrollTop - 1;
		} else if (externalDropInsertIndex >= rects.length) {
			y = rects[rects.length - 1].bottom - containerRect.top + container.scrollTop - 1;
		} else {
			const above = rects[externalDropInsertIndex - 1];
			const below = rects[externalDropInsertIndex];
			y = (above.bottom + below.top) / 2 - containerRect.top + container.scrollTop - 1;
		}
		indicator.style.top = `${y}px`;
		indicator.style.display = 'block';
	}

	function handlePointerMoveWithGroupHighlight(e: PointerEvent) {
		channelDragController?.handlePointerMove(e);
		if (!channelDragController?.draggingId) {
			clearGroupHighlight();
			return;
		}
		const groupEl = findGroupAtPoint(e.clientX, e.clientY);
		if (groupEl !== highlightedGroupEl) {
			clearGroupHighlight();
		}
		if (groupEl) {
			if (!highlightedGroupEl) {
				groupEl.style.outline = '2px solid var(--brand-500, #5c6bc0)';
				groupEl.style.outlineOffset = '2px';
				groupEl.style.borderRadius = '6px';
				highlightedGroupEl = groupEl;
			}
			updateDropIndicatorPosition(groupEl, e.clientY);
		}
	}

	function handlePointerUpWithGroupDetection(e: PointerEvent) {
		const draggingId = channelDragController?.draggingId;
		if (draggingId) {
			const groupEl = findGroupAtPoint(e.clientX, e.clientY);
			if (groupEl) {
				const groupId = groupEl.getAttribute('data-channel-group-id');
				if (groupId) {
					const insertIdx = externalDropInsertIndex >= 0 ? externalDropInsertIndex : undefined;
					clearGroupHighlight();
					channelDragController?.handlePointerCancel(e);
					addChannelToGroup(groupId, draggingId, insertIdx);
					return;
				}
			}
		}
		clearGroupHighlight();
		channelDragController?.handlePointerUp(e);
	}

	async function handleChannelReorder(sourceId: string, targetIndex: number) {
		const guildId = $currentGuildId;
		if (!guildId) return;

		const reordered = [...ungroupedTextChannels, ...ungroupedForumChannels, ...ungroupedGalleryChannels];
		const sourceIdx = reordered.findIndex(c => c.id === sourceId);
		if (sourceIdx === -1) return;

		const [moved] = reordered.splice(sourceIdx, 1);
		reordered.splice(targetIndex, 0, moved);

		const positions = reordered.map((c, i) => ({ id: c.id, position: i }));

		// Optimistic update
		for (const p of positions) {
			const ch = [...$textChannels, ...$voiceChannels, ...$forumChannels, ...$galleryChannels].find(c => c.id === p.id);
			if (ch) updateChannelStore({ ...ch, position: p.position });
		}

		try {
			await api.reorderChannels(guildId, positions);
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to reorder channels'), 'error');
		}
	}
</script>

<svelte:window
	onclick={() => { closeContextMenu(); dmContextMenu = null; guildContextMenu = null; }}
	onpointermove={(e) => handlePointerMoveWithGroupHighlight(e)}
	onpointerup={(e) => handlePointerUpWithGroupDetection(e)}
	onpointercancel={(e) => { clearGroupHighlight(); channelDragController?.handlePointerCancel(e); }}
	onkeydown={(e) => channelDragController?.handleKeyDown(e)}
/>

<aside class="flex h-full shrink-0 flex-col border-r border-[--border-primary] bg-bg-secondary" style="width: {width}px;" aria-label="Channel list">
	<!-- Guild header -->
	{#if $currentGuild}
		<div
			class="flex h-12 items-center justify-between border-b border-bg-floating px-4"
			oncontextmenu={(e) => { e.preventDefault(); guildContextMenu = { x: e.clientX, y: e.clientY }; channelContextMenu = null; dmContextMenu = null; }}
			role="button"
			tabindex="0"
		>
			<div class="flex min-w-0 items-center gap-1.5">
				<h2 class="truncate text-sm font-semibold text-text-primary">{$currentGuild.name}</h2>
				{#if $currentGuild.instance_id && $currentUser && $currentGuild.instance_id !== $currentUser.instance_id}
					<FederationBadge domain={$currentGuild.instance_domain || $currentGuild.instance_id} compact />
				{/if}
			</div>
			<div class="flex items-center gap-1">
				{#if $totalUnreads > 0}
					<button
						class="rounded p-1 text-text-muted hover:bg-bg-modifier hover:text-text-primary"
						onclick={() => markAllRead()}
						title="Mark All as Read"
					>
						<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
						</svg>
					</button>
				{/if}
				<button
					class="rounded p-1 text-text-muted hover:bg-bg-modifier hover:text-text-primary"
					onclick={() => (showInvite = true)}
					title="Create Invite"
				>
					<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M18 9v3m0 0v3m0-3h3m-3 0h-3m-2-5a4 4 0 11-8 0 4 4 0 018 0zM3 20a6 6 0 0112 0v1H3v-1z" />
					</svg>
				</button>
				{#if $canManageGuild}
				<button
					class="rounded p-1 text-text-muted hover:bg-bg-modifier hover:text-text-primary"
					onclick={() => goto(`/app/guilds/${$currentGuild?.id}/settings`)}
					title="Server Settings"
				>
					<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M12 6V4m0 2a2 2 0 100 4m0-4a2 2 0 110 4m-6 8a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4m6 6v10m6-2a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4" />
					</svg>
				</button>
			{/if}
			</div>
		</div>
	{:else}
		<div class="flex h-12 items-center border-b border-bg-floating px-4">
			<h2 class="text-sm font-semibold text-text-primary">Direct Messages</h2>
		</div>
	{/if}

	<!-- Channel list -->
	<div class="flex-1 overflow-y-auto px-2 py-2">
		{#if $currentGuild}
			<!-- Create Channel button -->
			{#if $canManageChannels}
				<button
					class="mb-2 flex w-full items-center gap-1.5 rounded px-2 py-1.5 text-left text-sm text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-secondary"
					onclick={() => { showCreateChannel = true; }}
					title="Create Channel"
				>
					<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M12 5v14m-7-7h14" />
					</svg>
					Create Channel
				</button>
			{/if}

			<!-- Text Channels -->
			<div bind:this={channelListEl} class="relative">
			{#each ungroupedTextChannels as channel (channel.id)}
						{@const unread = $unreadCounts.get(channel.id) ?? 0}
						{@const mentions = $mentionCounts.get(channel.id) ?? 0}
						{@const chMuted = isChannelMuted(channel.id)}
						<div
							class="group/drag flex items-center"
							data-channel-id={channel.id}
							onpointerdown={(e) => channelDragController?.handlePointerDown(e, channel.id)}
							role="listitem"
						>
							<DragHandle visible={$canManageChannels} />
							<button
								class="mb-0.5 flex flex-1 items-center gap-1.5 rounded px-2 py-1.5 text-left text-sm transition-colors {chMuted ? 'opacity-60' : ''} {$currentChannelId === channel.id ? 'bg-bg-modifier text-text-primary' : unread > 0 && !chMuted ? 'text-text-primary font-semibold hover:bg-bg-modifier' : 'text-text-muted hover:bg-bg-modifier hover:text-text-secondary'}"
								onclick={() => handleChannelClick(channel.id)}
								oncontextmenu={(e) => openContextMenu(e, channel)}
							>
								{#if channel.encrypted}
									{@const unlocked = $unlockedChannels.has(channel.id)}
										<svg class="h-4 w-4 shrink-0 {unlocked ? 'text-green-400' : 'text-red-400'}" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
											<title>{unlocked ? 'Encrypted (unlocked)' : 'Encrypted (locked)'}</title>
										{#if unlocked}
											<path stroke-linecap="round" stroke-linejoin="round" d="M13.5 10.5V6.75a4.5 4.5 0 119 0v3.75M3.75 21.75h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H3.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z" />
										{:else}
											<path stroke-linecap="round" stroke-linejoin="round" d="M16.5 10.5V6.75a4.5 4.5 0 10-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H6.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z" />
										{/if}
									</svg>
								{:else}
									<span class="text-lg leading-none text-brand-500 font-mono">#</span>
								{/if}
								<span class="flex-1 truncate font-mono">{channel.name}</span>
							{#if chMuted}
									<svg class="h-3.5 w-3.5 shrink-0 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
										<title>Muted</title>
									<path d="M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
									<path d="M17 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2" />
								</svg>
							{/if}
							{#if mentions > 0 && $currentChannelId !== channel.id}
								<span class="ml-auto flex h-4 min-w-4 items-center justify-center rounded-full {chMuted ? 'bg-text-muted/50' : 'bg-red-500'} px-1 text-2xs font-bold text-white" title="{mentions} mention{mentions !== 1 ? 's' : ''}">
									@{mentions > 99 ? '99+' : mentions}
								</span>
							{:else if unread > 0 && $currentChannelId !== channel.id}
								<span class="ml-auto flex h-4 min-w-4 items-center justify-center rounded-full {chMuted ? 'bg-text-muted/30' : 'bg-text-muted'} px-1 text-2xs font-bold text-white">
									{unread > 99 ? '99+' : unread}
								</span>
							{/if}
							</button>
						</div>
						<!-- Nested threads under this channel -->
						{@const filteredThreads = getFilteredThreads(channel.id)}
						{#if filteredThreads.length > 0}
							<div class="ml-3 border-l border-bg-floating/50 pl-1">
								{#each filteredThreads as thread (thread.id)}
									{@const threadUnread = $unreadCounts.get(thread.id) ?? 0}
									{@const threadMentions = $mentionCounts.get(thread.id) ?? 0}
									<button
										class="mb-0.5 flex w-full items-center gap-1 rounded px-1.5 py-1 text-left text-xs transition-colors {$activeThreadId === thread.id ? 'bg-bg-modifier text-text-primary' : threadUnread > 0 ? 'text-text-primary font-semibold hover:bg-bg-modifier' : 'text-text-muted hover:bg-bg-modifier hover:text-text-secondary'}"
										onclick={() => handleThreadClick(thread)}
										oncontextmenu={(e) => openThreadContextMenu(e, thread)}
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

			<!-- Forum Channels -->
			{#each ungroupedForumChannels as ch (ch.id)}
				{@const isActive = $currentChannelId === ch.id}
				{@const unread = $unreadCounts.get(ch.id) ?? 0}
				{@const mentions = $mentionCounts.get(ch.id) ?? 0}
				<div
					class="group/drag flex items-center"
					data-channel-id={ch.id}
					onpointerdown={(e) => channelDragController?.handlePointerDown(e, ch.id)}
					role="listitem"
				>
					<DragHandle visible={$canManageChannels} />
					<button
						class="flex flex-1 items-center gap-1.5 rounded px-1.5 py-1 text-left text-sm transition-colors
							{isActive
								? 'bg-bg-modifier text-text-primary'
								: unread > 0
								? 'text-text-primary hover:bg-bg-modifier/50'
								: 'text-text-muted hover:bg-bg-modifier/50 hover:text-text-secondary'}"
						onclick={() => handleChannelClick(ch.id)}
						oncontextmenu={(e) => openContextMenu(e, ch)}
					>
						<svg class="h-5 w-5 shrink-0 {isActive ? 'text-text-primary' : 'text-text-muted'}" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z" />
						</svg>
						<span class="truncate {unread > 0 ? 'font-semibold' : ''}">{ch.name ?? 'forum'}</span>
						{#if mentions > 0}
							<span class="ml-auto flex h-4 min-w-[16px] items-center justify-center rounded-full bg-red-500 px-1 text-2xs font-bold text-white">{mentions}</span>
						{:else if unread > 0}
							<span class="ml-auto h-2 w-2 rounded-full bg-text-primary"></span>
						{/if}
					</button>
				</div>
			{/each}

			<!-- Gallery Channels -->
			{#each ungroupedGalleryChannels as ch (ch.id)}
				{@const isActive = $currentChannelId === ch.id}
				{@const unread = $unreadCounts.get(ch.id) ?? 0}
				{@const mentions = $mentionCounts.get(ch.id) ?? 0}
				<div
					class="group/drag flex items-center"
					data-channel-id={ch.id}
					onpointerdown={(e) => channelDragController?.handlePointerDown(e, ch.id)}
					role="listitem"
				>
					<DragHandle visible={$canManageChannels} />
					<button
						class="flex flex-1 items-center gap-1.5 rounded px-1.5 py-1 text-left text-sm transition-colors
							{isActive
								? 'bg-bg-modifier text-text-primary'
								: unread > 0
								? 'text-text-primary hover:bg-bg-modifier/50'
								: 'text-text-muted hover:bg-bg-modifier/50 hover:text-text-secondary'}"
						onclick={() => handleChannelClick(ch.id)}
						oncontextmenu={(e) => openContextMenu(e, ch)}
					>
						<svg class="h-5 w-5 shrink-0 {isActive ? 'text-text-primary' : 'text-text-muted'}" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<rect x="3" y="3" width="7" height="7" rx="1" />
							<rect x="14" y="3" width="7" height="7" rx="1" />
							<rect x="3" y="14" width="7" height="7" rx="1" />
							<rect x="14" y="14" width="7" height="7" rx="1" />
						</svg>
						<span class="truncate {unread > 0 ? 'font-semibold' : ''}">{ch.name ?? 'gallery'}</span>
						{#if mentions > 0}
							<span class="ml-auto flex h-4 min-w-[16px] items-center justify-center rounded-full bg-red-500 px-1 text-2xs font-bold text-white">{mentions}</span>
						{:else if unread > 0}
							<span class="ml-auto h-2 w-2 rounded-full bg-text-primary"></span>
						{/if}
					</button>
				</div>
			{/each}
			</div>

			<!-- Voice Channels -->
			{#each ungroupedVoiceChannels as channel (channel.id)}
				{@const voiceUsers = $channelVoiceUsers.get(channel.id)}
				<button
					class="mb-0.5 flex w-full items-center gap-1.5 rounded px-2 py-1.5 text-left text-sm transition-colors {$currentChannelId === channel.id ? 'bg-bg-modifier text-text-primary' : 'text-text-muted hover:bg-bg-modifier hover:text-text-secondary'}"
					onclick={() => handleChannelClick(channel.id)}
					ondblclick={() => { const gid = $currentGuildId; if (gid) joinVoice(channel.id, gid, channel.name ?? ''); }}
					oncontextmenu={(e) => openContextMenu(e, channel)}
				>
					<svg class="h-4 w-4 shrink-0" fill="currentColor" viewBox="0 0 24 24">
						<path d="M12 2c-1.66 0-3 1.34-3 3v6c0 1.66 1.34 3 3 3s3-1.34 3-3V5c0-1.66-1.34-3-3-3zm5 9c0 2.76-2.24 5-5 5s-5-2.24-5-5H5c0 3.53 2.61 6.43 6 6.92V21h2v-3.08c3.39-.49 6-3.39 6-6.92h-2z" />
					</svg>
					<span class="flex-1 truncate">{channel.name}</span>
					{#if voiceUsers && voiceUsers.size > 0}
						<span class="text-2xs text-green-400">{voiceUsers.size}</span>
					{/if}
				</button>
				{#if voiceUsers && voiceUsers.size > 0}
					<div class="mb-1 ml-3 space-y-0.5 border-l border-bg-floating pl-3">
						{#each [...voiceUsers.values()] as participant (participant.userId)}
							<div class="flex items-center gap-1.5 py-0.5">
								<div class="relative">
									<Avatar name={participant.displayName ?? participant.username} src={avatarUrl(participant.avatarId, participant.instanceId || undefined)} size="sm" />
									{#if participant.speaking && $voiceChannelId === channel.id}
										<div class="pointer-events-none absolute -inset-0.5 z-10 rounded-full border-2 border-green-500 shadow-[0_0_8px_rgba(34,197,94,0.35)]"></div>
									{/if}
								</div>
								<span class="flex-1 truncate text-xs text-text-muted">{participant.displayName ?? participant.username}</span>
								{#if participant.muted}
									<svg class="h-3 w-3 shrink-0 text-red-400" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
										<path d="M19 19L5 5m14 0v8a3 3 0 01-5.12 2.12M12 19v2m-4-4h8" />
									</svg>
								{/if}
								{#if participant.deafened}
									<svg class="h-3 w-3 shrink-0 text-red-400" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
										<path d="M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
										<path d="M17 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2" />
									</svg>
								{/if}
							</div>
						{/each}
					</div>
				{/if}
			{/each}

			<!-- Channel Groups -->
			<ChannelGroups
			onGroupsLoaded={(ids) => { groupedChannelIds = ids; }}
			onChannelContextMenu={(e, channel) => openContextMenu(e, channel as any)}
			onThreadContextMenu={(e, thread) => openThreadContextMenu(e, thread)}
			onGroupsChanged={(g) => { channelGroupsData = g; }}
			onReady={(api) => { reloadChannelGroups = api.reload; }}
		/>

			<!-- Upcoming Events -->
			{#if upcomingEvents.length > 0}
				<div class="mb-1 flex items-center justify-between px-1 pt-4">
					<button
						class="flex items-center gap-1 text-2xs font-bold uppercase tracking-wide text-text-muted hover:text-text-secondary"
						onclick={() => toggleSection('upcoming-events')}
						title={isSectionCollapsed('upcoming-events') ? 'Expand Upcoming Events' : 'Collapse Upcoming Events'}
					>
						<svg
							class="h-3 w-3 shrink-0 transition-transform duration-200 {isSectionCollapsed('upcoming-events') ? '-rotate-90' : ''}"
							fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"
						>
							<path d="M19 9l-7 7-7-7" />
						</svg>
						Upcoming Events
					</button>
					<button
						class="text-text-muted hover:text-text-primary"
						onclick={() => goto(`/app/guilds/${$currentGuildId}/events`)}
						title="View All Events"
					>
						<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
						</svg>
					</button>
				</div>
				{#if !isSectionCollapsed('upcoming-events')}
					{#each upcomingEvents as event (event.id)}
						<button
							class="mb-0.5 flex w-full items-start gap-2 rounded px-2 py-1.5 text-left text-sm text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-secondary"
							onclick={() => goto(`/app/guilds/${$currentGuildId}/events`)}
						>
							<svg class="mt-0.5 h-4 w-4 shrink-0 text-brand-400" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
								<path d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
							</svg>
							<div class="min-w-0 flex-1">
								<span class="block truncate text-xs font-medium text-text-primary">{event.name}</span>
								<span class="text-2xs text-text-muted">{formatEventDate(event.scheduled_start)}</span>
							</div>
						</button>
					{/each}
				{/if}
			{/if}

		{:else}
			<!-- DM List (when no guild is selected) -->
			<div class="mb-1 flex items-center justify-between px-1">
				<button
					class="mb-0.5 flex w-full items-center gap-1.5 rounded px-2 py-1.5 text-left text-sm text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-secondary"
					onclick={() => goto('/app/friends')}
				>
					<svg class="h-4 w-4 shrink-0" fill="currentColor" viewBox="0 0 24 24">
						<path d="M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z" />
					</svg>
					<span class="flex-1">Friends</span>
					{#if $pendingIncomingCount > 0}
						<span class="flex h-4 min-w-4 items-center justify-center rounded-full bg-red-500 px-1 text-2xs font-bold text-white">
							{$pendingIncomingCount > 99 ? '99+' : $pendingIncomingCount}
						</span>
					{/if}
				</button>
			</div>

			<div class="mb-1 flex items-center justify-between px-1 pt-2">
				<button
					class="flex items-center gap-1 text-2xs font-bold uppercase tracking-wide text-text-muted hover:text-text-secondary"
					onclick={() => toggleSection('dm-list')}
					title={isSectionCollapsed('dm-list') ? 'Expand Direct Messages' : 'Collapse Direct Messages'}
				>
					<svg
						class="h-3 w-3 shrink-0 transition-transform duration-200 {isSectionCollapsed('dm-list') ? '-rotate-90' : ''}"
						fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"
					>
						<path d="M19 9l-7 7-7-7" />
					</svg>
					Direct Messages
				</button>
				<button
					class="rounded p-0.5 text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-secondary"
					onclick={() => (showGroupDMCreate = true)}
					title="Create Group DM"
				>
					<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" />
					</svg>
				</button>
			</div>

			{#if !isSectionCollapsed('dm-list')}
				{#if $dmList.length === 0}
					<p class="px-2 py-2 text-xs text-text-muted">No conversations yet.</p>
				{:else}
					{#each $dmList as dm (dm.id)}
						{@const dmUnread = $unreadCounts.get(dm.id) ?? 0}
						{@const dmMentions = $mentionCounts.get(dm.id) ?? 0}
						{@const dmName = getDMDisplayName(dm, $currentUser?.id)}
						{@const dmRecipient = getDMRecipient(dm, $currentUser?.id)}
						{@const dmMuted = isChannelMuted(dm.id)}
						<button
							class="mb-0.5 flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm transition-colors {dmMuted ? 'opacity-60' : ''} {$currentChannelId === dm.id ? 'bg-bg-modifier text-text-primary' : dmUnread > 0 && !dmMuted ? 'text-text-primary font-semibold hover:bg-bg-modifier' : 'text-text-muted hover:bg-bg-modifier hover:text-text-secondary'}"
							onclick={() => goto(`/app/dms/${dm.id}`)}
							oncontextmenu={(e) => { e.preventDefault(); dmContextMenu = { x: e.clientX, y: e.clientY, channel: dm }; channelContextMenu = null; threadContextMenu = null; }}
						>
						<span
							class="cursor-pointer"
							onclick={(e) => { if (dmRecipient) { e.stopPropagation(); dmProfileUserId = dmRecipient.id; } }}
							onkeydown={(e) => { if ((e.key === 'Enter' || e.key === ' ') && dmRecipient) { e.preventDefault(); e.stopPropagation(); dmProfileUserId = dmRecipient.id; } }}
							role="button"
							tabindex="0"
						>
							<Avatar name={dmName} src={dmRecipient?.avatar_id ? avatarUrl(dmRecipient.avatar_id, dmRecipient.instance_id || undefined) : null} size="sm" status={dmRecipient ? ($presenceMap.get(dmRecipient.id) ?? undefined) : undefined} />
						</span>
							{#if dm.encrypted}
								{@const unlocked = $unlockedChannels.has(dm.id)}
									<svg class="h-3.5 w-3.5 shrink-0 {unlocked ? 'text-green-400' : 'text-red-400'}" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
										<title>{unlocked ? 'Encrypted (unlocked)' : 'Encrypted (locked)'}</title>
									{#if unlocked}
										<path stroke-linecap="round" stroke-linejoin="round" d="M13.5 10.5V6.75a4.5 4.5 0 119 0v3.75M3.75 21.75h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H3.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z" />
									{:else}
										<path stroke-linecap="round" stroke-linejoin="round" d="M16.5 10.5V6.75a4.5 4.5 0 10-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H6.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z" />
									{/if}
								</svg>
							{/if}
							<span class="flex-1 truncate">{dmName}</span>
							{#if dmMuted}
									<svg class="h-3.5 w-3.5 shrink-0 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
										<title>Muted</title>
									<path d="M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
									<path d="M17 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2" />
								</svg>
							{/if}
							{#if dmMentions > 0}
								<span class="ml-auto flex h-4 min-w-4 items-center justify-center rounded-full {dmMuted ? 'bg-text-muted/50' : 'bg-red-500'} px-1 text-2xs font-bold text-white" title="{dmMentions} mention{dmMentions !== 1 ? 's' : ''}">
									@{dmMentions > 99 ? '99+' : dmMentions}
								</span>
							{:else if dmUnread > 0}
								<span class="ml-auto flex h-4 min-w-4 items-center justify-center rounded-full {dmMuted ? 'bg-text-muted/30' : 'bg-text-muted'} px-1 text-2xs font-bold text-white">
									{dmUnread > 99 ? '99+' : dmUnread}
								</span>
							{/if}
						</button>
					{/each}
				{/if}
			{/if}
		{/if}
	</div>

	<!-- Voice connection bar (above user panel) -->
	<VoiceConnectionBar />

	<!-- User panel (bottom) -->
	<UserPanel onreportissue={() => (showReportIssue = true)} />
</aside>

<!-- Channel context menu -->
{#if channelContextMenu}
	<ChannelContextMenu
		menu={channelContextMenu}
		canManageChannels={$canManageChannels}
		channelGroups={channelGroupsData}
		getthreadfilter={getThreadActivityFilter}
		onthreadfilter={handleSetThreadFilter}
		onedit={openEditModal}
		onremovefromgroup={removeChannelFromGroupCtx}
		onaddtogroup={addChannelToGroup}
		ondelete={handleDeleteChannel}
		onclose={closeContextMenu}
	/>
{/if}

<!-- Thread context menu -->
{#if threadContextMenu}
	<ThreadContextMenu
		x={threadContextMenu.x}
		y={threadContextMenu.y}
		thread={threadContextMenu.thread}
		canManageThreads={$canManageThreads}
		onopen={(thread) => { handleThreadClick(thread); threadContextMenu = null; }}
		onhide={handleHideThread}
		onarchive={handleArchiveThread}
		ondelete={handleDeleteThread}
	/>
{/if}

<!-- DM context menu -->
{#if dmContextMenu}
	<DMContextMenu
		x={dmContextMenu.x}
		y={dmContextMenu.y}
		channel={dmContextMenu.channel}
		onclose={() => (dmContextMenu = null)}
		onmarkread={markDMRead}
		onaddfriend={addFriendFromDM}
		onclosedm={closeDM}
	/>
{/if}

<!-- Guild context menu (mute/unmute) -->
{#if guildContextMenu && $currentGuild}
	<GuildContextMenu
		x={guildContextMenu.x}
		y={guildContextMenu.y}
		guild={$currentGuild}
		canManageGuild={$canManageGuild}
		onclose={closeContextMenu}
		oninvite={() => (showInvite = true)}
	/>
{/if}

<!-- Invite Modal -->
<InviteModal bind:open={showInvite} onclose={() => (showInvite = false)} />

<CreateChannelModal bind:open={showCreateChannel} onclose={() => (showCreateChannel = false)} />
<EditChannelModal bind:open={showEditChannel} channel={editChannel} onclose={() => (showEditChannel = false)} />

<ReportIssueModal bind:open={showReportIssue} onclose={() => (showReportIssue = false)} />

<GroupDMCreateModal bind:open={showGroupDMCreate} onclose={() => (showGroupDMCreate = false)} />

	{#if dmProfileUserId}
		<ProfileModal userId={dmProfileUserId} open={!!dmProfileUserId} onclose={() => (dmProfileUserId = null)} />
	{/if}
