<script lang="ts">
	import { currentGuild, currentGuildId } from '$lib/stores/guilds';
	import { channelList, textChannels, voiceChannels, forumChannels, galleryChannels, currentChannelId, setChannel, updateChannel as updateChannelStore, removeChannel as removeChannelStore, threadsByParent, hideThread as hideThreadStore, getThreadActivityFilter, setThreadActivityFilter, pendingThreadOpen, editChannelSignal } from '$lib/stores/channels';
	import { currentUser } from '$lib/stores/auth';
	import { guildEventsByGuild, loadGuildEvents } from '$lib/stores/guildEvents';
	import { dmList, removeDMChannel } from '$lib/stores/dms';
	import { unreadCounts, mentionCounts, markAllRead, totalUnreads } from '$lib/stores/unreads';
	import { addToast } from '$lib/stores/toast';
	import { confirmAction } from '$lib/stores/confirm';
	import { relationships, addOrUpdateRelationship } from '$lib/stores/relationships';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import { onMount, untrack } from 'svelte';
	import InviteModal from '$components/guild/InviteModal.svelte';
	import ChannelContextMenu from '$components/layout/ChannelContextMenu.svelte';
	import ChannelGroups from '$components/layout/ChannelGroups.svelte';
	import { e2ee, unlockedChannels } from '$lib/encryption/e2eeManager';
	import VoiceConnectionBar from '$components/layout/VoiceConnectionBar.svelte';
	import { getDMRecipient } from '$lib/utils/dm';
	import { canManageChannels, canManageGuild, canManageThreads } from '$lib/stores/permissions';
	import GroupDMCreateModal from '$components/common/GroupDMCreateModal.svelte';
	import ProfileModal from '$components/common/ProfileModal.svelte';
	import CreateChannelModal from '$components/layout/CreateChannelModal.svelte';
	import DMContextMenu from '$components/layout/DMContextMenu.svelte';
	import DirectMessagesSection from '$components/layout/DirectMessagesSection.svelte';
	import EditChannelModal from '$components/layout/EditChannelModal.svelte';
	import GuildContextMenu from '$components/layout/GuildContextMenu.svelte';
	import GuildSidebarHeader from '$components/layout/GuildSidebarHeader.svelte';
	import ReportIssueModal from '$components/layout/ReportIssueModal.svelte';
	import TextChannelsSection from '$components/layout/TextChannelsSection.svelte';
	import ThreadContextMenu from '$components/layout/ThreadContextMenu.svelte';
	import TypedChannelsSection from '$components/layout/TypedChannelsSection.svelte';
	import UpcomingEventsSection from '$components/layout/UpcomingEventsSection.svelte';
	import UserPanel from '$components/layout/UserPanel.svelte';
	import VoiceChannelsSection from '$components/layout/VoiceChannelsSection.svelte';
	import type { Channel } from '$lib/types';
	import { getErrorMessage } from '$lib/utils/apiError';
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
		<GuildSidebarHeader
			guild={$currentGuild}
			currentUser={$currentUser}
			totalUnreads={$totalUnreads}
			canManageGuild={$canManageGuild}
			onmarkallread={() => markAllRead()}
			oninvite={() => (showInvite = true)}
			onsettings={() => goto(`/app/guilds/${$currentGuild?.id}/settings`)}
			oncontextmenu={(e) => { e.preventDefault(); guildContextMenu = { x: e.clientX, y: e.clientY }; channelContextMenu = null; dmContextMenu = null; }}
		/>
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
				<TextChannelsSection
					channels={ungroupedTextChannels}
					canManageChannels={$canManageChannels}
					dragController={channelDragController}
					getfilteredthreads={getFilteredThreads}
					onchannelclick={handleChannelClick}
					onthreadclick={handleThreadClick}
					oncontextmenu={openContextMenu}
					onthreadcontextmenu={openThreadContextMenu}
				/>

				<TypedChannelsSection
					channels={ungroupedForumChannels}
					kind="forum"
					canManageChannels={$canManageChannels}
					dragController={channelDragController}
					onchannelclick={handleChannelClick}
					oncontextmenu={openContextMenu}
				/>
				<TypedChannelsSection
					channels={ungroupedGalleryChannels}
					kind="gallery"
					canManageChannels={$canManageChannels}
					dragController={channelDragController}
					onchannelclick={handleChannelClick}
					oncontextmenu={openContextMenu}
				/>
			</div>

			<VoiceChannelsSection
				channels={ungroupedVoiceChannels}
				currentChannelId={$currentChannelId}
				guildId={$currentGuildId}
				onchannelclick={handleChannelClick}
				oncontextmenu={openContextMenu}
			/>

			<!-- Channel Groups -->
			<ChannelGroups
			onGroupsLoaded={(ids) => { groupedChannelIds = ids; }}
			onChannelContextMenu={(e, channel) => openContextMenu(e, channel as any)}
			onThreadContextMenu={(e, thread) => openThreadContextMenu(e, thread)}
			onGroupsChanged={(g) => { channelGroupsData = g; }}
			onReady={(api) => { reloadChannelGroups = api.reload; }}
		/>

			<UpcomingEventsSection
				events={upcomingEvents}
				collapsed={isSectionCollapsed('upcoming-events')}
				ontoggle={() => toggleSection('upcoming-events')}
				onviewall={() => goto(`/app/guilds/${$currentGuildId}/events`)}
			/>

		{:else}
			<DirectMessagesSection
				collapsed={isSectionCollapsed('dm-list')}
				ontoggle={() => toggleSection('dm-list')}
				oncreategroup={() => (showGroupDMCreate = true)}
				onprofile={(userId) => (dmProfileUserId = userId)}
				oncontextmenu={(e, dm) => { e.preventDefault(); dmContextMenu = { x: e.clientX, y: e.clientY, channel: dm }; channelContextMenu = null; threadContextMenu = null; }}
			/>
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
