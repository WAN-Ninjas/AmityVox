<script lang="ts">
	import { page } from '$app/stores';
	import type { Channel, Message, ChannelFollower } from '$lib/types';
	import { setChannel, currentChannel, currentChannelId, pendingThreadOpen, activeThreadId, channels as channelsStore } from '$lib/stores/channels';
	import { currentGuild } from '$lib/stores/guilds';
	import { currentTypingUsers } from '$lib/stores/typing';
	import { ackChannel } from '$lib/stores/unreads';
	import { api } from '$lib/api/client';
	import { appendMessage } from '$lib/stores/messages';
	import { addToast } from '$lib/stores/toast';
	import TopBar from '$components/layout/TopBar.svelte';
	import MemberList from '$components/layout/MemberList.svelte';
	import ResizeHandle from '$components/common/ResizeHandle.svelte';
	import { memberListWidth } from '$lib/stores/layout';
	import MessageList from '$components/chat/MessageList.svelte';
	import MessageInput from '$components/chat/MessageInput.svelte';
	import TypingIndicator from '$components/chat/TypingIndicator.svelte';
	import PinnedMessages from '$components/chat/PinnedMessages.svelte';
	import ThreadPanel from '$components/chat/ThreadPanel.svelte';
	import VoiceChannelView from '$components/voice/VoiceChannelView.svelte';
	import ForumChannelView from '$components/channels/ForumChannelView.svelte';
	import GalleryChannelView from '$components/channels/GalleryChannelView.svelte';
	import GalleryPanel from '$lib/components/gallery/GalleryPanel.svelte';
	import { e2ee } from '$lib/encryption/e2eeManager';

	let showMembers = $state(true);
	let showPins = $state(false);
	let showFollowers = $state(false);
	let showGallery = $state(false);
	let activeThread = $state<{ channel: Channel; parentMessage: Message | null } | null>(null);
	let galleryViewRef: GalleryChannelView | undefined;
	let isDragging = $state(false);
	let dragCounter = 0;
	let isUploading = $state(false);
	let nsfwAccepted = $state(false);
	const isArchived = $derived($currentChannel?.archived ?? false);

	// --- Channel Followers (announcement channels) ---
	let followers = $state<ChannelFollower[]>([]);
	let loadingFollowers = $state(false);
	let followTargetChannelId = $state('');
	let following = $state(false);
	let guildChannelsForFollow = $state<Channel[]>([]);

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

		isUploading = true;
		try {
			const isEncrypted = !!$currentChannel?.encrypted;
			const opts: Record<string, any> = {};
			if (isEncrypted) opts.encrypted = true;
			for (let file of files) {
				if (isEncrypted) {
					try {
						const buf = await file.arrayBuffer();
						const encBuf = await e2ee.encryptFile(channelId, buf);
						file = new File([encBuf], file.name + '.enc', { type: 'application/octet-stream' });
					} catch {
						addToast('Failed to encrypt file. Do you have the channel key?', 'error');
						return;
					}
				}
				const uploaded = await api.uploadFile(file);
				const sent = await api.sendMessage(channelId, '', { ...opts, attachment_ids: [uploaded.id] });
				appendMessage(sent);
			}
			addToast(`Uploaded ${files.length} file${files.length > 1 ? 's' : ''}`, 'success');
		} catch (err) {
			addToast('Upload failed', 'error');
		} finally {
			isUploading = false;
		}
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
		if (!channelId) return;
		loadingFollowers = true;
		try {
			const [f, channels] = await Promise.all([
				api.getChannelFollowers(channelId),
				$currentGuild ? api.getGuildChannels($currentGuild.id) : Promise.resolve([])
			]);
			followers = f;
			guildChannelsForFollow = channels.filter(c => c.channel_type === 'text');
		} catch {}
		finally { loadingFollowers = false; }
	}

	async function handleFollowChannel() {
		const channelId = $currentChannelId;
		if (!channelId || !followTargetChannelId) return;
		following = true;
		try {
			const follower = await api.followChannel(channelId, { target_channel_id: followTargetChannelId });
			followers = [...followers, follower];
			followTargetChannelId = '';
			addToast('Channel followed! Announcements will be forwarded.', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to follow channel', 'error');
		} finally {
			following = false;
		}
	}

	async function handleUnfollowChannel(followerId: string) {
		const channelId = $currentChannelId;
		if (!channelId) return;
		try {
			await api.unfollowChannel(channelId, followerId);
			followers = followers.filter(f => f.id !== followerId);
			addToast('Unfollowed channel', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to unfollow', 'error');
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
				<span class="text-lg font-medium text-text-primary">Drop files to upload</span>
				<span class="text-sm text-text-muted">Files will be sent to #{$currentChannel?.name ?? 'channel'}</span>
			</div>
		</div>
	{/if}

	<!-- Upload progress overlay -->
	{#if isUploading}
		<div class="absolute inset-0 z-50 flex items-center justify-center bg-bg-primary/60">
			<div class="flex items-center gap-3 rounded-lg bg-bg-secondary px-6 py-4 shadow-lg">
				<svg class="h-5 w-5 animate-spin text-brand-400" fill="none" viewBox="0 0 24 24">
					<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
					<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
				</svg>
				<span class="text-sm text-text-primary">Uploading...</span>
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
					onclick={() => acceptNsfwForChannel($page.params.channelId)}
				>
					I understand, show content
				</button>
			</div>
		</div>
	{/if}

	<div class="flex min-w-0 flex-1 flex-col">
		<TopBar
			onToggleMembers={() => (showMembers = !showMembers)}
			onTogglePins={() => { showPins = !showPins; if (showPins) { activeThread = null; activeThreadId.set(null); showFollowers = false; showGallery = false; } }}
			onToggleFollowers={toggleFollowers}
			onToggleGallery={() => { showGallery = !showGallery; if (showGallery) { showPins = false; showFollowers = false; activeThread = null; activeThreadId.set(null); } }}
			{showPins}
			{showFollowers}
			{showGallery}
		/>
		{#if $currentChannel?.channel_type === 'voice' || $currentChannel?.channel_type === 'stage'}
			<VoiceChannelView
				channelId={$currentChannelId ?? ''}
				guildId={$page.params.guildId}
			/>
		{:else if $currentChannel?.channel_type === 'forum'}
			<ForumChannelView
				channelId={$currentChannelId ?? ''}
				onopenthread={openThread}
			/>
		{:else if $currentChannel?.channel_type === 'gallery'}
			<GalleryChannelView
				bind:this={galleryViewRef}
				channelId={$currentChannelId ?? ''}
				onopenthread={openThread}
			/>
		{:else}
			{#if isArchived}
				<div class="flex items-center gap-2 border-b border-bg-floating bg-yellow-500/10 px-4 py-2">
					<svg class="h-5 w-5 shrink-0 text-yellow-500" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4" />
					</svg>
					<span class="text-sm font-medium text-yellow-500">This channel is archived. It is read-only.</span>
				</div>
			{/if}
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
				<MessageInput />
			{/if}
		{/if}
	</div>

	{#if activeThread}
		<ThreadPanel
			threadChannel={activeThread.channel}
			parentMessage={activeThread.parentMessage}
			onclose={() => { activeThread = null; activeThreadId.set(null); }}
		/>
	{/if}

	{#if showPins && !activeThread}
		<PinnedMessages onclose={() => (showPins = false)} onscrollto={scrollToMessage} />
	{/if}

	{#if showFollowers && !activeThread && !showPins}
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
				{#if loadingFollowers}
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
							disabled={following || !followTargetChannelId}
						>
							{following ? 'Following...' : 'Follow'}
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

	{#if showGallery && !activeThread}
		<aside class="fixed inset-0 z-50 flex flex-col bg-bg-secondary md:relative md:inset-auto md:z-auto md:w-80 md:shrink-0 md:border-l md:border-bg-floating">
			<GalleryPanel channelId={$currentChannelId ?? undefined} guildId={$page.params.guildId} canManage={true} onclose={() => (showGallery = false)} />
		</aside>
	{/if}

	{#if showMembers && !showPins && !activeThread && !showFollowers && !showGallery}
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
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="fixed inset-0 z-40 bg-black/50 lg:hidden" onclick={() => (showMembers = false)}></div>
		<aside class="fixed inset-y-0 right-0 z-50 w-72 overflow-y-auto bg-bg-secondary lg:hidden">
			<MemberList />
		</aside>
	{/if}
</div>
