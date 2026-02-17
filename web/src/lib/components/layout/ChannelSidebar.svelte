<script lang="ts">
	import { currentGuild, currentGuildId } from '$lib/stores/guilds';
	import { textChannels, voiceChannels, currentChannelId, setChannel, updateChannel as updateChannelStore, removeChannel as removeChannelStore, threadsByParent, hideThread as hideThreadStore, getThreadActivityFilter, setThreadActivityFilter, pendingThreadOpen, activeThreadId } from '$lib/stores/channels';
	import { channelVoiceUsers, voiceChannelId } from '$lib/stores/voice';
	import { currentUser } from '$lib/stores/auth';
	import Avatar from '$components/common/Avatar.svelte';
	import Modal from '$components/common/Modal.svelte';
	import { presenceMap } from '$lib/stores/presence';
	import { dmList, removeDMChannel } from '$lib/stores/dms';
	import ContextMenu from '$components/common/ContextMenu.svelte';
	import ContextMenuItem from '$components/common/ContextMenuItem.svelte';
	import ContextMenuDivider from '$components/common/ContextMenuDivider.svelte';
	import { unreadCounts, mentionCounts, markAllRead, totalUnreads } from '$lib/stores/unreads';
	import { addToast } from '$lib/stores/toast';
	import { pendingIncomingCount, relationships, addOrUpdateRelationship } from '$lib/stores/relationships';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import InviteModal from '$components/guild/InviteModal.svelte';
	import ChannelGroups from '$components/layout/ChannelGroups.svelte';
	import EncryptionPanel from '$components/encryption/EncryptionPanel.svelte';
	import VoiceConnectionBar from '$components/layout/VoiceConnectionBar.svelte';
	import { getDMDisplayName, getDMRecipient } from '$lib/utils/dm';
	import { canManageChannels, canManageGuild, canManageThreads } from '$lib/stores/permissions';
	import { channelMutePrefs, guildMutePrefs, isChannelMuted, isGuildMuted, muteChannel, unmuteChannel, muteGuild, unmuteGuild } from '$lib/stores/muting';
	import StatusPicker from '$components/common/StatusPicker.svelte';
	import GroupDMCreateModal from '$components/common/GroupDMCreateModal.svelte';
	import type { Channel, GuildEvent } from '$lib/types';

	interface Props {
		/** Width in pixels, controlled by the layout store / resize handle. */
		width?: number;
	}

	let { width = 224 }: Props = $props();

	// Status picker
	let showStatusPicker = $state(false);

	// Group DM creation modal
	let showGroupDMCreate = $state(false);

	// Report issue modal
	let showReportIssue = $state(false);
	let reportIssueTitle = $state('');
	let reportIssueDescription = $state('');
	let reportIssueCategory = $state('general');
	let reportIssueSubmitting = $state(false);

	async function submitReportIssue() {
		if (!reportIssueTitle.trim() || !reportIssueDescription.trim()) return;
		reportIssueSubmitting = true;
		try {
			await api.createIssue(reportIssueTitle.trim(), reportIssueDescription.trim(), reportIssueCategory);
			addToast('Issue reported successfully', 'success');
			showReportIssue = false;
			reportIssueTitle = '';
			reportIssueDescription = '';
			reportIssueCategory = 'general';
		} catch (err: any) {
			addToast(err.message || 'Failed to report issue', 'error');
		} finally {
			reportIssueSubmitting = false;
		}
	}

	let upcomingEvents = $state<GuildEvent[]>([]);

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

	async function handleArchiveChannel(channelId: string, archive: boolean) {
		try {
			const updated = await api.updateChannel(channelId, { archived: archive });
			updateChannelStore(updated);
		} catch (err: any) {
			alert(err.message || `Failed to ${archive ? 'archive' : 'unarchive'} channel`);
		}
	}

	$effect(() => {
		const gid = $currentGuildId;
		if (gid) {
			api.getGuildEvents(gid, { status: 'scheduled', limit: 3 })
				.then((events) => { upcomingEvents = events; })
				.catch(() => { upcomingEvents = []; });
		} else {
			upcomingEvents = [];
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
	let newChannelName = $state('');
	let newChannelType = $state<'text' | 'voice'>('text');
	let creatingChannel = $state(false);
	let channelError = $state('');

	// Edit channel modal
	let showEditChannel = $state(false);
	let editChannelId = $state('');
	let editChannelName = $state('');
	let editChannelTopic = $state('');
	let editChannelNsfw = $state(false);
	let editChannelEncrypted = $state(false);
	let editChannelType = $state<'text' | 'voice'>('text');
	let editChannelUserLimit = $state(0);
	let editChannelBitrate = $state(64000);
	let editingChannel = $state(false);

	const userLimitOptions = [0, 5, 10, 15, 20, 25, 50, 99];
	const bitrateOptions = [32000, 64000, 96000, 128000, 192000, 256000, 384000];

	// Invite modal
	let showInvite = $state(false);

	// Context menu (channel)
	let channelContextMenu = $state<{ x: number; y: number; channelId: string; channelName: string; archived: boolean } | null>(null);

	// Thread context menu
	let threadContextMenu = $state<{ x: number; y: number; thread: Channel } | null>(null);

	// Show Threads submenu
	let showThreadFilterSubmenu = $state(false);

	// DM context menu
	let dmContextMenu = $state<{ x: number; y: number; channel: Channel } | null>(null);

	// Guild context menu
	let guildContextMenu = $state<{ x: number; y: number } | null>(null);

	// Mute duration submenu state
	let showMuteSubmenu = $state<'channel' | 'dm' | 'guild' | null>(null);

	const muteDurations = [
		{ label: '15 Minutes', ms: 15 * 60 * 1000 },
		{ label: '1 Hour', ms: 60 * 60 * 1000 },
		{ label: '8 Hours', ms: 8 * 60 * 60 * 1000 },
		{ label: '24 Hours', ms: 24 * 60 * 60 * 1000 },
		{ label: 'Until I turn it back on', ms: 0 }
	];

	// Thread activity filter state — triggers reactivity when changed.
	let threadFilterVersion = $state(0);

	function getFilteredThreads(channelId: string): Channel[] {
		// Access threadFilterVersion to trigger reactivity.
		void threadFilterVersion;
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
		showThreadFilterSubmenu = false;
	}

	async function handleHideThread(thread: Channel) {
		if (!thread.parent_channel_id) return;
		try {
			await hideThreadStore(thread.parent_channel_id, thread.id);
		} catch (err: any) {
			addToast(err.message || 'Failed to hide thread', 'error');
		} finally {
			threadContextMenu = null;
		}
	}

	async function handleArchiveThread(thread: Channel, archive: boolean) {
		try {
			const updated = await api.updateChannel(thread.id, { archived: archive });
			updateChannelStore(updated);
		} catch (err: any) {
			addToast(err.message || `Failed to ${archive ? 'archive' : 'unarchive'} thread`, 'error');
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

	async function handleCreateChannel() {
		const guildId = $currentGuildId;
		if (!guildId || !newChannelName.trim()) return;
		creatingChannel = true;
		channelError = '';
		try {
			const channel = await api.createChannel(guildId, newChannelName.trim(), newChannelType);
			updateChannelStore(channel);
			showCreateChannel = false;
			newChannelName = '';
			newChannelType = 'text';
		} catch (err: any) {
			channelError = err.message || 'Failed to create channel';
		} finally {
			creatingChannel = false;
		}
	}

	async function handleEditChannel() {
		if (!editChannelId || !editChannelName.trim()) return;
		editingChannel = true;
		channelError = '';
		try {
			const updateData: Record<string, unknown> = {
				name: editChannelName.trim(),
				topic: editChannelTopic || undefined,
				nsfw: editChannelNsfw
			};
			if (editChannelType === 'voice') {
				updateData.user_limit = editChannelUserLimit;
				updateData.bitrate = editChannelBitrate;
			}
			// Encryption is now managed via EncryptionPanel, not here
			const updated = await api.updateChannel(editChannelId, updateData as any);
			updateChannelStore(updated);
			showEditChannel = false;
		} catch (err: any) {
			channelError = err.message || 'Failed to update channel';
		} finally {
			editingChannel = false;
		}
	}

	async function handleDeleteChannel(channelId: string) {
		if (!confirm('Are you sure you want to delete this channel?')) return;
		try {
			await api.deleteChannel(channelId);
			removeChannelStore(channelId);
		} catch (err: any) {
			alert(err.message || 'Failed to delete channel');
		}
	}

	function openContextMenu(e: MouseEvent, channel: Channel) {
		e.preventDefault();
		channelContextMenu = { x: e.clientX, y: e.clientY, channelId: channel.id, channelName: channel.name ?? '', archived: channel.archived };
		dmContextMenu = null;
		threadContextMenu = null;
		showThreadFilterSubmenu = false;
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
		showThreadFilterSubmenu = false;
		showMuteSubmenu = null;
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
		} catch (err: any) {
			addToast(err.message || 'Failed to send friend request', 'error');
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
		editChannelId = channelId;
		editChannelName = channelName;
		editChannelTopic = '';
		channelError = '';
		// Look up current channel data to pre-populate fields
		const allChannels = [...$textChannels, ...$voiceChannels];
		const ch = allChannels.find(c => c.id === channelId);
		editChannelNsfw = ch?.nsfw ?? false;
		editChannelEncrypted = ch?.encrypted ?? false;
		editChannelType = (ch?.channel_type === 'voice' ? 'voice' : 'text');
		editChannelUserLimit = ch?.user_limit ?? 0;
		editChannelBitrate = ch?.bitrate ?? 64000;
		if (ch?.topic) editChannelTopic = ch.topic;
		showEditChannel = true;
		closeContextMenu();
	}

	function channelTypeButtonClass(type: 'text' | 'voice'): string {
		const base = 'rounded-lg border-2 px-4 py-2 text-sm transition-colors';
		if (newChannelType === type) return `${base} border-brand-500 bg-brand-500/10 text-text-primary`;
		return `${base} border-bg-modifier text-text-muted`;
	}
</script>

<svelte:window onclick={() => { closeContextMenu(); dmContextMenu = null; guildContextMenu = null; showStatusPicker = false; }} />

<aside class="flex h-full shrink-0 flex-col border-r border-[--border-primary] bg-bg-secondary" style="width: {width}px;" aria-label="Channel list">
	<!-- Guild header -->
	{#if $currentGuild}
		<div
			class="flex h-12 items-center justify-between border-b border-bg-floating px-4"
			oncontextmenu={(e) => { e.preventDefault(); guildContextMenu = { x: e.clientX, y: e.clientY }; channelContextMenu = null; dmContextMenu = null; }}
		>
			<h2 class="truncate text-sm font-semibold text-text-primary">{$currentGuild.name}</h2>
			<div class="flex items-center gap-1">
				{#if $totalUnreads > 0}
					<button
						class="text-text-muted hover:text-text-primary"
						onclick={() => markAllRead()}
						title="Mark All as Read"
					>
						<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
						</svg>
					</button>
				{/if}
				<button
					class="text-text-muted hover:text-text-primary"
					onclick={() => (showInvite = true)}
					title="Create Invite"
				>
					<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M18 9v3m0 0v3m0-3h3m-3 0h-3m-2-5a4 4 0 11-8 0 4 4 0 018 0zM3 20a6 6 0 0112 0v1H3v-1z" />
					</svg>
				</button>
				{#if $canManageGuild}
				<button
					class="text-text-muted hover:text-text-primary"
					onclick={() => goto(`/app/guilds/${$currentGuild?.id}/settings`)}
					title="Guild Settings"
				>
					<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
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
			<!-- Text Channels -->
			{#if activeTextChannels.length > 0 || $currentGuild}
				<div class="mb-1 flex items-center justify-between px-1 pt-4 first:pt-0">
					<button
						class="flex items-center gap-1 font-mono text-2xs font-bold uppercase tracking-wide text-text-muted hover:text-text-secondary"
						onclick={() => toggleSection('text-channels')}
						title={isSectionCollapsed('text-channels') ? 'Expand Text Channels' : 'Collapse Text Channels'}
					>
						<svg
							class="h-3 w-3 shrink-0 transition-transform duration-200 {isSectionCollapsed('text-channels') ? '-rotate-90' : ''}"
							fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"
						>
							<path d="M19 9l-7 7-7-7" />
						</svg>
						Text Channels
					</button>
					<button
						class="text-text-muted hover:text-text-primary"
						onclick={() => { newChannelType = 'text'; showCreateChannel = true; channelError = ''; }}
						title="Create Text Channel"
					>
						<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M12 5v14m-7-7h14" />
						</svg>
					</button>
				</div>
				{#if !isSectionCollapsed('text-channels')}
					{#each activeTextChannels as channel (channel.id)}
						{@const unread = $unreadCounts.get(channel.id) ?? 0}
						{@const mentions = $mentionCounts.get(channel.id) ?? 0}
						{@const chMuted = isChannelMuted(channel.id)}
						<button
							class="mb-0.5 flex w-full items-center gap-1.5 rounded px-2 py-1.5 text-left text-sm transition-colors {chMuted ? 'opacity-60' : ''} {$currentChannelId === channel.id ? 'bg-bg-modifier text-text-primary' : unread > 0 && !chMuted ? 'text-text-primary font-semibold hover:bg-bg-modifier' : 'text-text-muted hover:bg-bg-modifier hover:text-text-secondary'}"
							onclick={() => handleChannelClick(channel.id)}
							oncontextmenu={(e) => openContextMenu(e, channel)}
						>
							<span class="text-lg leading-none text-brand-500 font-mono">#</span>
							<span class="flex-1 truncate font-mono">{channel.name}</span>
							{#if chMuted}
								<svg class="h-3.5 w-3.5 shrink-0 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" title="Muted">
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
				{/if}
			{/if}

			<!-- Voice Channels -->
			{#if activeVoiceChannels.length > 0 || $currentGuild}
				<div class="mb-1 flex items-center justify-between px-1 pt-4">
					<button
						class="flex items-center gap-1 font-mono text-2xs font-bold uppercase tracking-wide text-text-muted hover:text-text-secondary"
						onclick={() => toggleSection('voice-channels')}
						title={isSectionCollapsed('voice-channels') ? 'Expand Voice Channels' : 'Collapse Voice Channels'}
					>
						<svg
							class="h-3 w-3 shrink-0 transition-transform duration-200 {isSectionCollapsed('voice-channels') ? '-rotate-90' : ''}"
							fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"
						>
							<path d="M19 9l-7 7-7-7" />
						</svg>
						Voice Channels
					</button>
					<button
						class="text-text-muted hover:text-text-primary"
						onclick={() => { newChannelType = 'voice'; showCreateChannel = true; channelError = ''; }}
						title="Create Voice Channel"
					>
						<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M12 5v14m-7-7h14" />
						</svg>
					</button>
				</div>
				{#if !isSectionCollapsed('voice-channels')}
					{#each activeVoiceChannels as channel (channel.id)}
						{@const voiceUsers = $channelVoiceUsers.get(channel.id)}
						<button
							class="mb-0.5 flex w-full items-center gap-1.5 rounded px-2 py-1.5 text-left text-sm transition-colors {$currentChannelId === channel.id ? 'bg-bg-modifier text-text-primary' : 'text-text-muted hover:bg-bg-modifier hover:text-text-secondary'}"
							onclick={() => handleChannelClick(channel.id)}
							oncontextmenu={(e) => openContextMenu(e, channel)}
						>
							<svg class="h-4 w-4 shrink-0" fill="currentColor" viewBox="0 0 24 24">
								<path d="M12 3a1 1 0 0 0-1 1v8a3 3 0 1 0 6 0V4a1 1 0 1 0-2 0v8a1 1 0 1 1-2 0V4a1 1 0 0 0-1-1zM7 12a5 5 0 0 0 10 0h2a7 7 0 0 1-6 6.92V21h-2v-2.08A7 7 0 0 1 5 12h2z" />
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
										<div class="{participant.speaking && $voiceChannelId === channel.id ? 'ring-2 ring-green-500 ring-offset-1 ring-offset-bg-secondary rounded-full shadow-[0_0_8px_rgba(34,197,94,0.35)]' : ''}">
											<Avatar name={participant.displayName ?? participant.username} src={participant.avatarId ? `/api/v1/files/${participant.avatarId}` : null} size="sm" />
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
				{/if}
			{/if}

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

			<!-- User Channel Groups -->
			<ChannelGroups />
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
							<Avatar name={dmName} src={dmRecipient?.avatar_id ? `/api/v1/files/${dmRecipient.avatar_id}` : null} size="sm" status={dmRecipient ? ($presenceMap.get(dmRecipient.id) ?? undefined) : undefined} />
							<span class="flex-1 truncate">{dmName}</span>
							{#if dmMuted}
								<svg class="h-3.5 w-3.5 shrink-0 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" title="Muted">
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
	{#if $currentUser}
		{@const myStatus = $presenceMap.get($currentUser.id) ?? $currentUser.status_presence ?? 'online'}
		<div class="relative border-t border-bg-floating bg-bg-primary/50 p-2">
			<StatusPicker bind:open={showStatusPicker} onclose={() => (showStatusPicker = false)} />
			<div class="flex items-center gap-2">
				<button
					class="flex min-w-0 flex-1 items-center gap-2 rounded-md px-1 py-0.5 transition-colors hover:bg-bg-modifier"
					onclick={(e) => { e.stopPropagation(); showStatusPicker = !showStatusPicker; }}
					title="Set status"
				>
					<Avatar name={$currentUser.display_name ?? $currentUser.username} src={$currentUser.avatar_id ? `/api/v1/files/${$currentUser.avatar_id}` : null} size="sm" status={myStatus} />
					<div class="min-w-0 flex-1 text-left">
						<p class="truncate text-sm font-medium text-text-primary">
							{$currentUser.display_name ?? $currentUser.username}
						</p>
						<p class="truncate text-xs text-text-muted">
							{$currentUser.status_text ?? myStatus}
						</p>
					</div>
				</button>
				<button
					class="text-orange-400 hover:text-orange-300"
					onclick={() => (showReportIssue = true)}
					title="Report Issue"
				>
					<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
					</svg>
				</button>
				<button
					class="text-text-muted hover:text-text-primary"
					onclick={() => goto('/app/settings')}
					title="User Settings"
				>
					<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
						<circle cx="12" cy="12" r="3" />
					</svg>
				</button>
			</div>
		</div>
	{/if}
</aside>

<!-- Channel context menu -->
{#if channelContextMenu}
	<div
		class="fixed z-50 min-w-[160px] rounded-md bg-bg-floating p-1 shadow-lg"
		style="left: {channelContextMenu.x}px; top: {channelContextMenu.y}px;"
		onclick={(e) => e.stopPropagation()}
	>
		{#if $canManageChannels}
		<button
			class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
			onclick={() => openEditModal(channelContextMenu!.channelId, channelContextMenu!.channelName)}
		>
			<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
			</svg>
			Edit Channel
		</button>
		{/if}
		<!-- Show Threads submenu -->
		<div class="relative">
			<button
				class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
				onclick={() => (showThreadFilterSubmenu = !showThreadFilterSubmenu)}
			>
				<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z" />
				</svg>
				Show Threads
				<svg class="ml-auto h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M9 5l7 7-7 7" />
				</svg>
			</button>
			{#if showThreadFilterSubmenu}
				{@const currentFilter = getThreadActivityFilter(channelContextMenu.channelId)}
				{@const submenuLeft = channelContextMenu.x + 300 < window.innerWidth}
				<div class="absolute top-0 max-h-[50vh] min-w-[140px] overflow-y-auto rounded-md bg-bg-floating p-1 shadow-lg {submenuLeft ? 'left-full ml-1' : 'right-full mr-1'}"
				>
					{#each [
						{ label: 'All', value: null },
						{ label: 'Last Hour', value: 60 },
						{ label: 'Last 6 Hours', value: 360 },
						{ label: 'Last 12 Hours', value: 720 },
						{ label: 'Last Day', value: 1440 }
					] as option}
						<button
							class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm transition-colors {currentFilter === option.value ? 'text-brand-400' : 'text-text-secondary'} hover:bg-brand-500 hover:text-white"
							onclick={() => handleSetThreadFilter(channelContextMenu!.channelId, option.value)}
						>
							{option.label}
						</button>
					{/each}
				</div>
			{/if}
		</div>
		<!-- Mute / Unmute -->
		{#if isChannelMuted(channelContextMenu.channelId)}
			<button
				class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
				onclick={() => { unmuteChannel(channelContextMenu!.channelId); closeContextMenu(); }}
			>
				<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M15.536 8.464a5 5 0 010 7.072M18.364 5.636a9 9 0 010 12.728M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
				</svg>
				Unmute Channel
			</button>
		{:else}
			<div class="relative">
				<button
					class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
					onclick={() => (showMuteSubmenu = showMuteSubmenu === 'channel' ? null : 'channel')}
				>
					<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
						<path d="M17 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2" />
					</svg>
					Mute Channel
					<svg class="ml-auto h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M9 5l7 7-7 7" />
					</svg>
				</button>
				{#if showMuteSubmenu === 'channel'}
					{@const submenuLeft = channelContextMenu.x + 300 < window.innerWidth}
					<div class="absolute top-0 min-w-[180px] rounded-md bg-bg-floating p-1 shadow-lg {submenuLeft ? 'left-full ml-1' : 'right-full mr-1'}">
						{#each muteDurations as opt}
							<button
								class="flex w-full items-center rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
								onclick={() => { muteChannel(channelContextMenu!.channelId, opt.ms || undefined); closeContextMenu(); }}
							>
								{opt.label}
							</button>
						{/each}
					</div>
				{/if}
			</div>
		{/if}
		{#if $canManageChannels}
		<button
			class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-red-400 hover:bg-red-500 hover:text-white"
			onclick={() => { handleDeleteChannel(channelContextMenu!.channelId); closeContextMenu(); }}
		>
			<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
			</svg>
			Delete Channel
		</button>
		{/if}
	</div>
{/if}

<!-- Thread context menu -->
{#if threadContextMenu}
	<div
		class="fixed z-50 min-w-[160px] rounded-md bg-bg-floating p-1 shadow-lg"
		style="left: {threadContextMenu.x}px; top: {threadContextMenu.y}px;"
		onclick={(e) => e.stopPropagation()}
	>
		<button
			class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
			onclick={() => { handleThreadClick(threadContextMenu!.thread); threadContextMenu = null; }}
		>
			<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z" />
			</svg>
			Open Thread
		</button>
		<button
			class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
			onclick={() => handleHideThread(threadContextMenu!.thread)}
		>
			<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l3.59 3.59m0 0A9.953 9.953 0 0112 5c4.478 0 8.268 2.943 9.543 7a10.025 10.025 0 01-4.132 5.411m0 0L21 21" />
			</svg>
			Hide Thread
		</button>
		{#if $canManageThreads}
		{#if threadContextMenu.thread.archived}
			<button
				class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
				onclick={() => handleArchiveThread(threadContextMenu!.thread, false)}
			>
				<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4" />
				</svg>
				Unarchive Thread
			</button>
		{:else}
			<button
				class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
				onclick={() => handleArchiveThread(threadContextMenu!.thread, true)}
			>
				<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4" />
				</svg>
				Archive Thread
			</button>
		{/if}
		{/if}
	</div>
{/if}

<!-- DM context menu -->
{#if dmContextMenu}
	<ContextMenu x={dmContextMenu.x} y={dmContextMenu.y} onclose={() => (dmContextMenu = null)}>
		<ContextMenuItem label="Open Message" onclick={() => { goto(`/app/dms/${dmContextMenu!.channel.id}`); dmContextMenu = null; }} />
		<ContextMenuItem label="Mark as Read" onclick={() => { markDMRead(dmContextMenu!.channel.id); dmContextMenu = null; }} />
		{@const dmRecip = getDMRecipient(dmContextMenu.channel, $currentUser?.id)}
		{#if dmRecip}
			{@const dmRel = $relationships.get(dmRecip.id)}
			{#if !dmRel || dmRel.type === 'pending_incoming'}
				<ContextMenuItem
					label={dmRel?.type === 'pending_incoming' ? 'Accept Request' : 'Add Friend'}
					onclick={() => { addFriendFromDM(dmContextMenu!.channel); dmContextMenu = null; }}
				/>
			{:else if dmRel.type === 'pending_outgoing'}
				<ContextMenuItem label="Request Sent" disabled />
			{/if}
		{/if}
		<ContextMenuDivider />
		{@const dmCtxMuted = isChannelMuted(dmContextMenu.channel.id)}
		{#if dmCtxMuted}
			<ContextMenuItem label="Unmute Conversation" onclick={() => { unmuteChannel(dmContextMenu!.channel.id); dmContextMenu = null; }} />
		{:else}
			<ContextMenuItem label="Mute for 15 Minutes" onclick={() => { muteChannel(dmContextMenu!.channel.id, 15 * 60 * 1000); dmContextMenu = null; }} />
			<ContextMenuItem label="Mute for 1 Hour" onclick={() => { muteChannel(dmContextMenu!.channel.id, 60 * 60 * 1000); dmContextMenu = null; }} />
			<ContextMenuItem label="Mute for 8 Hours" onclick={() => { muteChannel(dmContextMenu!.channel.id, 8 * 60 * 60 * 1000); dmContextMenu = null; }} />
			<ContextMenuItem label="Mute for 24 Hours" onclick={() => { muteChannel(dmContextMenu!.channel.id, 24 * 60 * 60 * 1000); dmContextMenu = null; }} />
			<ContextMenuItem label="Mute Until I Turn It Back On" onclick={() => { muteChannel(dmContextMenu!.channel.id); dmContextMenu = null; }} />
		{/if}
		<ContextMenuDivider />
		<ContextMenuItem label="Close DM" danger onclick={() => { closeDM(dmContextMenu!.channel.id); dmContextMenu = null; }} />
	</ContextMenu>
{/if}

<!-- Guild context menu (mute/unmute) -->
{#if guildContextMenu && $currentGuild}
	{@const gMuted = isGuildMuted($currentGuild.id)}
	<div
		class="fixed z-50 min-w-[180px] rounded-md bg-bg-floating p-1 shadow-lg"
		style="left: {guildContextMenu.x}px; top: {guildContextMenu.y}px;"
		onclick={(e) => e.stopPropagation()}
	>
		{#if gMuted}
			<button
				class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
				onclick={() => { unmuteGuild($currentGuild!.id); closeContextMenu(); }}
			>
				<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M15.536 8.464a5 5 0 010 7.072M18.364 5.636a9 9 0 010 12.728M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
				</svg>
				Unmute Guild
			</button>
		{:else}
			<div class="relative">
				<button
					class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
					onclick={() => (showMuteSubmenu = showMuteSubmenu === 'guild' ? null : 'guild')}
				>
					<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
						<path d="M17 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2" />
					</svg>
					Mute Guild
					<svg class="ml-auto h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M9 5l7 7-7 7" />
					</svg>
				</button>
				{#if showMuteSubmenu === 'guild'}
					{@const submenuLeft = guildContextMenu.x + 300 < window.innerWidth}
					<div class="absolute top-0 min-w-[180px] rounded-md bg-bg-floating p-1 shadow-lg {submenuLeft ? 'left-full ml-1' : 'right-full mr-1'}">
						{#each muteDurations as opt}
							<button
								class="flex w-full items-center rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
								onclick={() => { muteGuild($currentGuild!.id, opt.ms || undefined); closeContextMenu(); }}
							>
								{opt.label}
							</button>
						{/each}
					</div>
				{/if}
			</div>
		{/if}
		<button
			class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
			onclick={() => { showInvite = true; closeContextMenu(); }}
		>
			<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M18 9v3m0 0v3m0-3h3m-3 0h-3m-2-5a4 4 0 11-8 0 4 4 0 018 0zM3 20a6 6 0 0112 0v1H3v-1z" />
			</svg>
			Invite People
		</button>
		{#if $canManageGuild}
			<button
				class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
				onclick={() => { goto(`/app/guilds/${$currentGuild?.id}/settings`); closeContextMenu(); }}
			>
				<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M12 6V4m0 2a2 2 0 100 4m0-4a2 2 0 110 4m-6 8a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4m6 6v10m6-2a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4" />
				</svg>
				Guild Settings
			</button>
		{/if}
	</div>
{/if}

<!-- Invite Modal -->
<InviteModal bind:open={showInvite} onclose={() => (showInvite = false)} />

<!-- Create Channel Modal -->
<Modal open={showCreateChannel} title="Create Channel" onclose={() => (showCreateChannel = false)}>
	{#if channelError}
		<div class="mb-4 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{channelError}</div>
	{/if}

	<div class="mb-4">
		<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Channel Type</label>
		<div class="flex gap-2">
			<button
				class={channelTypeButtonClass('text')}
				onclick={() => (newChannelType = 'text')}
			>
				# Text
			</button>
			<button
				class={channelTypeButtonClass('voice')}
				onclick={() => (newChannelType = 'voice')}
			>
				Voice
			</button>
		</div>
	</div>

	<div class="mb-4">
		<label for="channelName" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
			Channel Name
		</label>
		<input
			id="channelName"
			type="text"
			class="input w-full"
			bind:value={newChannelName}
			placeholder={newChannelType === 'text' ? 'new-channel' : 'General'}
			maxlength="100"
			onkeydown={(e) => e.key === 'Enter' && handleCreateChannel()}
		/>
	</div>

	<div class="flex justify-end gap-2">
		<button class="btn-secondary" onclick={() => (showCreateChannel = false)}>Cancel</button>
		<button class="btn-primary" onclick={handleCreateChannel} disabled={creatingChannel || !newChannelName.trim()}>
			{creatingChannel ? 'Creating...' : 'Create'}
		</button>
	</div>
</Modal>

<!-- Edit Channel Modal -->
<Modal open={showEditChannel} title="Edit Channel" onclose={() => (showEditChannel = false)}>
	{#if channelError}
		<div class="mb-4 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{channelError}</div>
	{/if}

	<div class="mb-4">
		<label for="editName" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
			Channel Name
		</label>
		<input
			id="editName"
			type="text"
			class="input w-full"
			bind:value={editChannelName}
			maxlength="100"
		/>
	</div>

	<div class="mb-4">
		<label for="editTopic" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
			Topic
		</label>
		<input
			id="editTopic"
			type="text"
			class="input w-full"
			bind:value={editChannelTopic}
			placeholder="Set a channel topic"
			maxlength="1024"
		/>
	</div>

	<div class="mb-4">
		<label class="flex items-center gap-3 cursor-pointer">
			<button
				type="button"
				role="switch"
				aria-checked={editChannelNsfw}
				class="relative inline-flex h-6 w-11 shrink-0 rounded-full transition-colors {editChannelNsfw ? 'bg-red-500' : 'bg-bg-modifier'}"
				onclick={() => (editChannelNsfw = !editChannelNsfw)}
			>
				<span
					class="pointer-events-none inline-block h-5 w-5 translate-y-0.5 rounded-full bg-white shadow transition-transform {editChannelNsfw ? 'translate-x-5' : 'translate-x-0.5'}"
				></span>
			</button>
			<div>
				<span class="text-sm font-medium text-text-primary">NSFW Channel</span>
				<p class="text-xs text-text-muted">Mark this channel as age-restricted. Users will see a warning before viewing.</p>
			</div>
		</label>
	</div>

	<!-- Encryption (text channels only) -->
	{#if editChannelType === 'text' && editChannelId}
		<div class="mb-4">
			<EncryptionPanel
				channelId={editChannelId}
				encrypted={editChannelEncrypted}
				onchange={() => { showEditChannel = false; }}
			/>
		</div>
	{/if}

	{#if editChannelType === 'voice'}
		<!-- Voice channel configuration -->
		<div class="mb-4">
			<label for="editUserLimit" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
				User Limit
			</label>
			<select
				id="editUserLimit"
				class="input w-full"
				bind:value={editChannelUserLimit}
			>
				{#each userLimitOptions as limit}
					<option value={limit}>
						{limit === 0 ? 'No limit' : `${limit} users`}
					</option>
				{/each}
			</select>
			<p class="mt-1 text-xs text-text-muted">Maximum number of users that can join this voice channel. Set to "No limit" for unlimited.</p>
		</div>

		<div class="mb-4">
			<label for="editBitrate" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
				Bitrate
			</label>
			<select
				id="editBitrate"
				class="input w-full"
				bind:value={editChannelBitrate}
			>
				{#each bitrateOptions as rate}
					<option value={rate}>
						{Math.floor(rate / 1000)}kbps
					</option>
				{/each}
			</select>
			<p class="mt-1 text-xs text-text-muted">Higher bitrate means better audio quality but uses more bandwidth.</p>
		</div>
	{/if}

	<div class="flex justify-end gap-2">
		<button class="btn-secondary" onclick={() => (showEditChannel = false)}>Cancel</button>
		<button class="btn-primary" onclick={handleEditChannel} disabled={editingChannel || !editChannelName.trim()}>
			{editingChannel ? 'Saving...' : 'Save'}
		</button>
	</div>
</Modal>

<!-- Report Issue Modal -->
<Modal open={showReportIssue} title="Report Issue" persistent onclose={() => (showReportIssue = false)}>
	<div class="mb-4">
		<label for="issueTitle" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Title</label>
		<input id="issueTitle" type="text" class="input w-full" bind:value={reportIssueTitle} placeholder="Brief summary" maxlength="200" />
	</div>
	<div class="mb-4">
		<label for="issueCategory" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Category</label>
		<select id="issueCategory" class="input w-full" bind:value={reportIssueCategory}>
			<option value="general">General</option>
			<option value="bug">Bug</option>
			<option value="abuse">Abuse</option>
			<option value="suggestion">Suggestion</option>
		</select>
	</div>
	<div class="mb-4">
		<label for="issueDesc" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Description</label>
		<textarea id="issueDesc" class="input w-full" rows="4" bind:value={reportIssueDescription} placeholder="Describe the issue in detail..."></textarea>
	</div>
	<div class="flex justify-end gap-2">
		<button class="btn-secondary" onclick={() => (showReportIssue = false)}>Cancel</button>
		<button class="btn-primary" onclick={submitReportIssue} disabled={reportIssueSubmitting || !reportIssueTitle.trim() || !reportIssueDescription.trim()}>
			{reportIssueSubmitting ? 'Submitting...' : 'Submit'}
		</button>
	</div>
</Modal>

<GroupDMCreateModal bind:open={showGroupDMCreate} onclose={() => (showGroupDMCreate = false)} />
