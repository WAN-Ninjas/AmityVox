<script lang="ts">
	import { page } from '$app/stores';
	import type { Channel, Message, ChannelFollower } from '$lib/types';
	import { setChannel, currentChannel, currentChannelId, pendingThreadOpen, activeThreadId, channels as channelsStore } from '$lib/stores/channels';
	import { currentGuild } from '$lib/stores/guilds';
	import { currentTypingUsers } from '$lib/stores/typing';
	import { ackChannel } from '$lib/stores/unreads';
	import { api, type MessageSummary } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { createAsyncOp } from '$lib/utils/asyncOp';
	import { getErrorMessage } from '$lib/utils/apiError';
	import { clientConfig, isFeatureEnabled } from '$lib/stores/clientConfig';
	import TopBar from '$components/layout/TopBar.svelte';
	import MemberList from '$components/layout/MemberList.svelte';
	import ResizeHandle from '$components/common/ResizeHandle.svelte';
	import { memberListWidth } from '$lib/stores/layout';
	import MessageList from '$components/chat/MessageList.svelte';
	import MessageInput from '$components/chat/MessageInput.svelte';
	import TypingIndicator from '$components/chat/TypingIndicator.svelte';
	import PinnedMessages from '$components/chat/PinnedMessages.svelte';
	import ReadOnlyBanner from '$components/chat/ReadOnlyBanner.svelte';
	import ScheduledMessagesPanel from '$components/chat/ScheduledMessagesPanel.svelte';
	import ThreadPanel from '$components/chat/ThreadPanel.svelte';
	import VoiceChannelView from '$components/voice/VoiceChannelView.svelte';
	import ForumChannelView from '$components/channels/ForumChannelView.svelte';
	import GalleryChannelView from '$components/channels/GalleryChannelView.svelte';
	import Whiteboard from '$components/channels/Whiteboard.svelte';
	import KanbanBoard from '$components/channels/KanbanBoard.svelte';
	import ActivityFrame from '$components/channels/ActivityFrame.svelte';
	import WidgetPanel from '$components/channels/WidgetPanel.svelte';
	import LocationShare from '$components/chat/LocationShare.svelte';
	import GalleryPanel from '$lib/components/gallery/GalleryPanel.svelte';

	let showMembers = $state(true);
	let showPins = $state(false);
	let showFollowers = $state(false);
	let showGallery = $state(false);
	let showSummaries = $state(false);
	let showWhiteboard = $state(false);
	let showKanban = $state(false);
	let showActivity = $state(false);
	let showWidgets = $state(false);
	let showLocations = $state(false);
	let showScheduled = $state(false);
	let summaries = $state<MessageSummary[]>([]);
	let activeThread = $state<{ channel: Channel; parentMessage: Message | null } | null>(null);
	let galleryViewRef = $state<GalleryChannelView>();
	let messageInputRef = $state<{ addPendingFiles: (files: File[]) => void }>();
	let isDragging = $state(false);
	let dragCounter = 0;
	let nsfwAccepted = $state(false);
	const routeGuildId = $derived($page.params.guildId ?? '');
	const isArchived = $derived($currentChannel?.archived ?? false);
	const hasWhiteboards = $derived(isFeatureEnabled($clientConfig, 'whiteboards'));
	const hasKanbanBoards = $derived(isFeatureEnabled($clientConfig, 'kanban_boards'));
	const hasActivities = $derived(isFeatureEnabled($clientConfig, 'activities'));
	const hasWidgets = $derived(isFeatureEnabled($clientConfig, 'widgets'));
	const hasLocationShares = $derived(isFeatureEnabled($clientConfig, 'location_sharing'));
	const hasScheduledMessages = $derived(isFeatureEnabled($clientConfig, 'scheduled_messages'));
	const hasMessageSummaries = $derived(isFeatureEnabled($clientConfig, 'message_summaries'));
	const hasThreadsAndReplies = $derived(isFeatureEnabled($clientConfig, 'threads_and_replies'));
	const hasAnnouncementChannels = $derived(isFeatureEnabled($clientConfig, 'announcement_channels'));
	const hasGalleryMedia = $derived(isFeatureEnabled($clientConfig, 'gallery_media'));
	const hasPins = $derived(isFeatureEnabled($clientConfig, 'pins'));

	$effect(() => {
		if (!hasWhiteboards) showWhiteboard = false;
		if (!hasKanbanBoards) showKanban = false;
		if (!hasActivities) showActivity = false;
		if (!hasWidgets) showWidgets = false;
		if (!hasLocationShares) showLocations = false;
		if (!hasScheduledMessages) showScheduled = false;
		if (!hasMessageSummaries) showSummaries = false;
		if (!hasThreadsAndReplies) {
			activeThread = null;
			activeThreadId.set(null);
		}
		if (!hasPins) showPins = false;
		if (!hasAnnouncementChannels) showFollowers = false;
		if (!hasGalleryMedia) showGallery = false;
	});
	// --- Channel Followers (announcement channels) ---
	let followers = $state<ChannelFollower[]>([]);
	let followTargetChannelId = $state('');
	let guildChannelsForFollow = $state<Channel[]>([]);
	let followersOp = $state(createAsyncOp());
	let followOp = $state(createAsyncOp());
	let summariesOp = $state(createAsyncOp());
	let createSummaryOp = $state(createAsyncOp());

	function isNsfwAcceptedForChannel(channelId: string): boolean {
		try {
			const accepted = sessionStorage.getItem(`nsfw_accepted_${channelId}`);
			return accepted === 'true';
		} catch {
			return false;
		}
	}

	function acceptNsfwForChannel(channelId: string) {
		nsfwAccepted = true;
		try {
			sessionStorage.setItem(`nsfw_accepted_${channelId}`, 'true');
		} catch {
			// sessionStorage may be unavailable
		}
	}

	function handleDragEnter(e: DragEvent) {
		e.preventDefault();
		dragCounter++;
		// Gallery/forum channels handle their own drag UX — don't show the page-level overlay.
		const ct = $currentChannel?.channel_type;
		if (ct === 'gallery' || ct === 'forum') return;
		if (e.dataTransfer?.types.includes('Files')) {
			isDragging = true;
		}
	}

	function handleDragLeave(e: DragEvent) {
		e.preventDefault();
		dragCounter--;
		if (dragCounter === 0) {
			isDragging = false;
		}
	}

	function handleDragOver(e: DragEvent) {
		e.preventDefault();
		if (e.dataTransfer) {
			e.dataTransfer.dropEffect = 'copy';
		}
	}

	async function handleDrop(e: DragEvent) {
		e.preventDefault();
		isDragging = false;
		dragCounter = 0;

		if (isArchived) return;

		// Gallery channels: forward dropped files to the gallery post creation form.
		const ct = $currentChannel?.channel_type;
		if (ct === 'gallery') {
			const files = e.dataTransfer?.files;
			if (files?.length && galleryViewRef) {
				galleryViewRef.addDroppedFiles(Array.from(files));
			}
			return;
		}
		// Forum channels handle their own uploads via their post creation forms.
		if (ct === 'forum') return;

		const files = e.dataTransfer?.files;
		const channelId = $currentChannelId;
		if (!files?.length || !channelId) return;

		messageInputRef?.addPendingFiles(Array.from(files));
	}

	// Set current channel when route params change and ack unreads.
	$effect(() => {
		const channelId = $page.params.channelId;
		if (channelId) {
			setChannel(channelId);
			ackChannel(channelId);
			nsfwAccepted = isNsfwAcceptedForChannel(channelId);
		}
	});

	function scrollToMessage(messageId: string) {
		const el = document.getElementById(`msg-${messageId}`);
		if (el) {
			el.scrollIntoView({ behavior: 'smooth', block: 'center' });
			el.classList.add('bg-brand-500/10');
			setTimeout(() => el.classList.remove('bg-brand-500/10'), 2000);
		}
	}

	function openThread(threadChannel: Channel, parentMessage: Message | null = null) {
		if (!hasThreadsAndReplies) return;
		activeThread = { channel: threadChannel, parentMessage };
		activeThreadId.set(threadChannel.id);
		showPins = false;
	}

	// React to sidebar thread/channel clicks via the pendingThreadOpen store.
	// Only clear the signal after successful resolution so it retries when channels load.
	$effect(() => {
		const threadId = $pendingThreadOpen;
		const allChannels = $channelsStore;
		if (threadId) {
			if (!hasThreadsAndReplies) {
				activeThread = null;
				activeThreadId.set(null);
				pendingThreadOpen.set(null);
				return;
			}
			if (threadId === '__close__') {
				activeThread = null;
				activeThreadId.set(null);
				pendingThreadOpen.set(null);
			} else {
				const thread = allChannels.get(threadId);
				if (thread) {
					openThread(thread);
					pendingThreadOpen.set(null);
				}
				// If thread not found yet, leave pendingThreadOpen set so
				// the effect retries when channelsStore updates.
			}
		}
	});

	// --- Channel Followers ---

	function toggleFollowers() {
		if (!hasAnnouncementChannels) return;
		showFollowers = !showFollowers;
		if (showFollowers) {
			activeThread = null;
			activeThreadId.set(null);
			showPins = false;
			loadFollowers();
		}
	}

	async function loadFollowers() {
		const channelId = $currentChannelId;
		if (!channelId || !hasAnnouncementChannels) return;
		await followersOp.run(async () => {
			const [f, channels] = await Promise.all([
				api.getChannelFollowers(channelId),
				$currentGuild ? api.getGuildChannels($currentGuild.id) : Promise.resolve([])
			]);
			followers = f;
			guildChannelsForFollow = channels.filter(c => c.channel_type === 'text');
		});
	}

	async function handleFollowChannel() {
		const channelId = $currentChannelId;
		if (!channelId || !followTargetChannelId || !hasAnnouncementChannels) return;
		await followOp.run(async () => {
			const follower = await api.followChannel(channelId, { target_channel_id: followTargetChannelId });
			followers = [...followers, follower];
			followTargetChannelId = '';
			addToast('Channel followed! Announcements will be forwarded.', 'success');
		}, msg => addToast(msg, 'error'), 'Failed to follow channel');
	}

	async function handleUnfollowChannel(followerId: string) {
		const channelId = $currentChannelId;
		if (!channelId || !hasAnnouncementChannels) return;
		try {
			await api.unfollowChannel(channelId, followerId);
			followers = followers.filter(f => f.id !== followerId);
			addToast('Unfollowed channel', 'success');
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to unfollow'), 'error');
		}
	}

	async function loadSummaries() {
		const channelId = $currentChannelId;
		if (!channelId || !hasMessageSummaries) return;
		const result = await summariesOp.run(
			() => api.getMessageSummaries(channelId),
			msg => addToast(msg, 'error'),
			'Failed to load summaries'
		);
		if (result) summaries = result;
	}

	async function createSummary() {
		const channelId = $currentChannelId;
		if (!channelId || !hasMessageSummaries) return;
		if ($currentChannel?.encrypted) {
			addToast('Encrypted channels cannot be summarized server-side', 'error');
			return;
		}
		const result = await createSummaryOp.run(
			() => api.summarizeMessages(channelId, { message_count: 100 }),
			msg => addToast(msg, 'error'),
			'Failed to summarize messages'
		);
		if (result) {
			summaries = [result, ...summaries.filter(summary => summary.id !== result.id)];
			showSummaries = true;
			addToast('Summary created', 'success');
		}
	}
</script>

<svelte:head>
	<title>
		{$currentChannel?.name ? `#${$currentChannel.name}` : 'Channel'}
		{$currentGuild ? ` — ${$currentGuild.name}` : ''}
		— AmityVox
	</title>
</svelte:head>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
	class="relative flex h-full"
	ondragenter={handleDragEnter}
	ondragleave={handleDragLeave}
	ondragover={handleDragOver}
	ondrop={handleDrop}
>
	<!-- Drop overlay -->
	{#if isDragging}
		<div class="absolute inset-0 z-50 flex items-center justify-center bg-bg-primary/80 backdrop-blur-sm">
			<div class="flex flex-col items-center gap-3 rounded-xl border-2 border-dashed border-brand-500 bg-bg-secondary/90 px-12 py-10">
				<svg class="h-12 w-12 text-brand-400" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
					<path d="M3 16.5v2.25A2.25 2.25 0 005.25 21h13.5A2.25 2.25 0 0021 18.75V16.5m-13.5-9L12 3m0 0l4.5 4.5M12 3v13.5" />
				</svg>
				<span class="text-lg font-medium text-text-primary">Drop files to attach</span>
				<span class="text-sm text-text-muted">Review files in the composer before sending</span>
			</div>
		</div>
	{/if}

	<!-- NSFW age gate overlay -->
	{#if $currentChannel?.nsfw && !nsfwAccepted}
		<div class="absolute inset-0 z-50 flex items-center justify-center bg-bg-primary">
			<div class="flex max-w-md flex-col items-center gap-4 rounded-xl bg-bg-secondary px-10 py-8 text-center shadow-lg">
				<svg class="h-16 w-16 text-red-400" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
				</svg>
				<h2 class="text-xl font-bold text-text-primary">This channel is marked as NSFW</h2>
				<p class="text-sm text-text-muted">
					You must be 18+ to view this content. This channel may contain content that is not suitable for all audiences.
				</p>
				<button
					class="mt-2 rounded-lg bg-red-600 px-6 py-2.5 text-sm font-medium text-white transition-colors hover:bg-red-700"
					onclick={() => { if ($currentChannelId) acceptNsfwForChannel($currentChannelId); }}
				>
					I understand, show content
				</button>
			</div>
		</div>
	{/if}

	<div class="flex min-w-0 flex-1 flex-col">
		<TopBar
			onToggleMembers={() => (showMembers = !showMembers)}
			onTogglePins={() => { if (!hasPins) return; showPins = !showPins; if (showPins) { activeThread = null; activeThreadId.set(null); showFollowers = false; showGallery = false; } }}
			onToggleFollowers={toggleFollowers}
			onToggleGallery={() => { if (!hasGalleryMedia) return; showGallery = !showGallery; if (showGallery) { showPins = false; showFollowers = false; activeThread = null; activeThreadId.set(null); } }}
			{showPins}
			{showFollowers}
			{showGallery}
			canUseAnnouncementChannels={hasAnnouncementChannels}
			canUseGalleryMedia={hasGalleryMedia}
			canUsePins={hasPins}
		/>
		{#if $currentChannel?.channel_type === 'voice' || $currentChannel?.channel_type === 'stage'}
			<VoiceChannelView
				channelId={$currentChannelId ?? ''}
				guildId={routeGuildId}
			/>
		{:else if $currentChannel?.channel_type === 'forum'}
			<ForumChannelView
				channelId={$currentChannelId ?? ''}
				onopenthread={openThread}
			/>
		{:else if $currentChannel?.channel_type === 'gallery' && hasGalleryMedia}
			<GalleryChannelView
				bind:this={galleryViewRef}
				channelId={$currentChannelId ?? ''}
				onopenthread={openThread}
			/>
		{:else if $currentChannel?.channel_type === 'gallery'}
			<div class="flex flex-1 items-center justify-center p-6 text-sm text-text-muted">
				Gallery channels are disabled on this server.
			</div>
		{:else}
			{#if isArchived}
				<div class="flex items-center gap-2 border-b border-bg-floating bg-yellow-500/10 px-4 py-2">
					<svg class="h-5 w-5 shrink-0 text-yellow-500" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4" />
					</svg>
					<span class="text-sm font-medium text-yellow-500">This channel is archived. It is read-only.</span>
				</div>
			{/if}
			<ReadOnlyBanner readOnly={Boolean($currentChannel?.read_only && !isArchived)} />
			{#if !$currentChannel?.encrypted && (hasWhiteboards || hasKanbanBoards || hasActivities || hasWidgets || hasLocationShares || hasScheduledMessages || hasMessageSummaries)}
				<div class="flex items-center justify-end gap-2 border-b border-bg-floating bg-bg-primary px-4 py-2">
					{#if hasWhiteboards}
						<button
							class="btn-secondary text-xs"
							onclick={() => { showWhiteboard = !showWhiteboard; showKanban = false; showActivity = false; showWidgets = false; showLocations = false; showScheduled = false; showSummaries = false; }}
						>
							{showWhiteboard ? 'Hide Whiteboard' : 'Whiteboard'}
						</button>
					{/if}
					{#if hasKanbanBoards}
						<button
							class="btn-secondary text-xs"
							onclick={() => { showKanban = !showKanban; showWhiteboard = false; showActivity = false; showWidgets = false; showLocations = false; showScheduled = false; showSummaries = false; }}
						>
							{showKanban ? 'Hide Board' : 'Board'}
						</button>
					{/if}
					{#if hasActivities}
						<button
							class="btn-secondary text-xs"
							onclick={() => { showActivity = !showActivity; showWhiteboard = false; showKanban = false; showWidgets = false; showLocations = false; showScheduled = false; showSummaries = false; }}
						>
							{showActivity ? 'Hide Activity' : 'Activity'}
						</button>
					{/if}
					{#if hasWidgets}
						<button
							class="btn-secondary text-xs"
							onclick={() => { showWidgets = !showWidgets; showWhiteboard = false; showKanban = false; showActivity = false; showLocations = false; showScheduled = false; showSummaries = false; }}
						>
							{showWidgets ? 'Hide Widgets' : 'Widgets'}
						</button>
					{/if}
					{#if hasLocationShares}
						<button
							class="btn-secondary text-xs"
							onclick={() => { showLocations = !showLocations; showWhiteboard = false; showKanban = false; showActivity = false; showWidgets = false; showScheduled = false; showSummaries = false; }}
						>
							{showLocations ? 'Hide Locations' : 'Locations'}
						</button>
					{/if}
					{#if hasScheduledMessages}
						<button
							class="btn-secondary text-xs"
							onclick={() => { showScheduled = !showScheduled; showWhiteboard = false; showKanban = false; showActivity = false; showWidgets = false; showLocations = false; showSummaries = false; }}
						>
							{showScheduled ? 'Hide Scheduled' : 'Scheduled'}
						</button>
					{/if}
					{#if hasMessageSummaries}
						<button class="btn-secondary text-xs" onclick={() => { showSummaries = !showSummaries; showWhiteboard = false; showKanban = false; showActivity = false; showWidgets = false; showLocations = false; showScheduled = false; if (showSummaries) loadSummaries(); }}>
							{showSummaries ? 'Hide Summaries' : 'Summaries'}
						</button>
						<button class="btn-primary text-xs" onclick={createSummary} disabled={createSummaryOp.loading}>
							{createSummaryOp.loading ? 'Summarizing...' : 'Summarize Recent'}
						</button>
					{/if}
				</div>
			{/if}
			{#if hasWhiteboards && showWhiteboard && $currentChannelId}
				<Whiteboard channelId={$currentChannelId} onclose={() => (showWhiteboard = false)} />
			{:else if hasKanbanBoards && showKanban && $currentChannelId}
				<KanbanBoard channelId={$currentChannelId} />
			{:else if hasActivities && showActivity && $currentChannelId}
				<ActivityFrame channelId={$currentChannelId} onclose={() => (showActivity = false)} />
			{:else if hasWidgets && showWidgets && $currentChannelId && routeGuildId}
				<WidgetPanel channelId={$currentChannelId} guildId={routeGuildId} />
			{:else if hasLocationShares && showLocations && $currentChannelId}
				<div class="border-b border-bg-floating bg-bg-secondary p-4">
					<LocationShare channelId={$currentChannelId} />
				</div>
			{:else if hasScheduledMessages && showScheduled && $currentChannelId}
				<ScheduledMessagesPanel channelId={$currentChannelId} onclose={() => (showScheduled = false)} />
			{:else}
				<MessageList onopenthread={openThread} />
				<TypingIndicator typingUsers={$currentTypingUsers} />
				{#if isArchived}
					<div class="border-t border-bg-floating px-4 pb-4 pt-2">
						<div class="flex items-center justify-center gap-2 rounded-lg bg-bg-modifier px-4 py-3">
							<svg class="h-4 w-4 shrink-0 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
								<path d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4" />
							</svg>
							<span class="text-sm text-text-muted">This channel is archived</span>
						</div>
					</div>
				{:else}
					<MessageInput bind:this={messageInputRef} />
				{/if}
			{/if}
		{/if}
	</div>

	{#if hasThreadsAndReplies && activeThread}
		<ThreadPanel
			threadChannel={activeThread.channel}
			parentMessage={activeThread.parentMessage}
			onclose={() => { activeThread = null; activeThreadId.set(null); }}
		/>
	{/if}

	{#if hasPins && showPins && !activeThread}
		<PinnedMessages onclose={() => (showPins = false)} onscrollto={scrollToMessage} />
	{/if}

	{#if hasAnnouncementChannels && showFollowers && !activeThread && !showPins}
		<aside class="fixed inset-0 z-50 flex flex-col bg-bg-secondary md:relative md:inset-auto md:z-auto md:w-64 md:shrink-0 md:border-l md:border-bg-floating">
			<div class="flex items-center justify-between border-b border-bg-floating px-4 py-3">
				<h2 class="text-sm font-bold text-text-primary">Channel Followers</h2>
				<button
					class="text-text-muted hover:text-text-primary"
					onclick={() => (showFollowers = false)}
					title="Close"
				>
					<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			</div>

			<div class="flex-1 overflow-y-auto p-4">
				{#if followersOp.loading}
					<p class="text-sm text-text-muted">Loading followers...</p>
				{:else}
					<!-- Follow form -->
					<div class="mb-4">
						<h3 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Forward to Channel</h3>
						<p class="mb-2 text-2xs text-text-muted">
							Announcements from this channel will be forwarded to the selected text channel.
						</p>
						<select class="input mb-2 w-full text-sm" bind:value={followTargetChannelId}>
							<option value="">Select a channel...</option>
							{#each guildChannelsForFollow as ch (ch.id)}
								<option value={ch.id}>#{ch.name ?? 'unnamed'}</option>
							{/each}
						</select>
						<button
							class="btn-primary w-full text-xs"
							onclick={handleFollowChannel}
							disabled={followOp.loading || !followTargetChannelId}
						>
							{followOp.loading ? 'Following...' : 'Follow'}
						</button>
					</div>

					<!-- Current followers -->
					<div>
						<h3 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">
							Current Followers ({followers.length})
						</h3>
						{#if followers.length === 0}
							<p class="text-xs text-text-muted">No followers yet.</p>
						{:else}
							<div class="space-y-2">
								{#each followers as follower (follower.id)}
									<div class="flex items-center justify-between rounded-lg bg-bg-primary p-2.5">
										<div class="min-w-0 flex-1">
											<p class="text-sm text-text-primary">
												{#if follower.guild_name}
													{follower.guild_name}
												{:else}
													Guild
												{/if}
											</p>
											<p class="text-xs text-text-muted">
												{#if follower.channel_name}
													#{follower.channel_name}
												{:else}
													Channel
												{/if}
											</p>
											<p class="mt-0.5 text-2xs text-text-muted">
												Since {new Date(follower.created_at).toLocaleDateString()}
											</p>
										</div>
										<button
											class="shrink-0 text-xs text-red-400 hover:text-red-300"
											onclick={() => handleUnfollowChannel(follower.id)}
											title="Unfollow"
										>
											<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
												<path d="M6 18L18 6M6 6l12 12" />
											</svg>
										</button>
									</div>
								{/each}
							</div>
						{/if}
					</div>
				{/if}
			</div>
		</aside>
	{/if}

	{#if hasGalleryMedia && showGallery && !activeThread}
		<aside class="fixed inset-0 z-50 flex flex-col bg-bg-secondary md:relative md:inset-auto md:z-auto md:w-80 md:shrink-0 md:border-l md:border-bg-floating">
			<GalleryPanel channelId={$currentChannelId ?? undefined} guildId={$page.params.guildId} canManage={true} onclose={() => (showGallery = false)} />
		</aside>
	{/if}

	{#if hasMessageSummaries && showSummaries && !activeThread}
		<aside class="fixed inset-0 z-50 flex flex-col bg-bg-secondary md:relative md:inset-auto md:z-auto md:w-96 md:shrink-0 md:border-l md:border-bg-floating">
			<div class="flex items-center justify-between border-b border-bg-floating px-4 py-3">
				<div>
					<h2 class="text-sm font-bold text-text-primary">Local Summaries</h2>
					<p class="text-xs text-text-muted">Extractive highlights from recent unencrypted messages.</p>
				</div>
				<button class="text-text-muted hover:text-text-primary" onclick={() => (showSummaries = false)} title="Close">
					<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			</div>
			<div class="flex-1 overflow-y-auto p-4">
				{#if summariesOp.loading}
					<p class="text-sm text-text-muted">Loading summaries...</p>
				{:else if summaries.length === 0}
					<p class="text-sm text-text-muted">No summaries yet.</p>
				{:else}
					<div class="space-y-3">
						{#each summaries as summary (summary.id)}
							<div class="rounded-lg bg-bg-primary p-3">
								<div class="mb-2 flex items-center justify-between gap-2">
									<span class="text-xs font-medium text-text-muted">{summary.message_count} messages · {summary.model}</span>
									<span class="text-2xs text-text-muted">{new Date(summary.created_at).toLocaleString()}</span>
								</div>
								<p class="whitespace-pre-wrap text-sm text-text-secondary">{summary.summary}</p>
							</div>
						{/each}
					</div>
				{/if}
			</div>
		</aside>
	{/if}

	{#if showMembers && !showPins && !activeThread && !showFollowers && !showGallery && !showSummaries}
		<!-- Desktop: inline sidebar with resize handle -->
		<div class="hidden lg:contents">
			<div class="flex">
				<ResizeHandle
					width={$memberListWidth}
					onresize={(w) => memberListWidth.set(w)}
					onreset={() => memberListWidth.reset()}
					side="right"
				/>
			</div>
			<MemberList />
		</div>
		<!-- Mobile: overlay from right -->
		<button class="fixed inset-0 z-40 bg-black/50 lg:hidden" aria-label="Close member list" onclick={() => (showMembers = false)}></button>
		<aside class="fixed inset-y-0 right-0 z-50 w-72 overflow-y-auto bg-bg-secondary lg:hidden">
			<MemberList />
		</aside>
	{/if}
</div>
