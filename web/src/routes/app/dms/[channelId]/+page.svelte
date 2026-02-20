<script lang="ts">
	import { page } from '$app/stores';
	import { setChannel, currentChannelId, currentChannel } from '$lib/stores/channels';
	import { currentGuildId, setGuild } from '$lib/stores/guilds';
	import { currentTypingUsers } from '$lib/stores/typing';
	import { ackChannel } from '$lib/stores/unreads';
	import { api } from '$lib/api/client';
	import { appendMessage } from '$lib/stores/messages';
	import { addToast } from '$lib/stores/toast';
	import { dmList } from '$lib/stores/dms';
	import { currentUser } from '$lib/stores/auth';
	import { presenceMap } from '$lib/stores/presence';
	import { voiceChannelId, voiceState, joinVoice, leaveVoice, toggleCamera } from '$lib/stores/voice';
	import { dismissIncomingCall } from '$lib/stores/callRing';
	import { e2ee } from '$lib/encryption/e2eeManager';
	import { getDMDisplayName, getDMRecipient } from '$lib/utils/dm';
	import Avatar from '$components/common/Avatar.svelte';
	import ProfileModal from '$components/common/ProfileModal.svelte';
	import MessageList from '$components/chat/MessageList.svelte';
	import MessageInput from '$components/chat/MessageInput.svelte';
	import TypingIndicator from '$components/chat/TypingIndicator.svelte';
	import GroupDMSettingsPanel from '$components/common/GroupDMSettingsPanel.svelte';
	import VoiceChannelView from '$components/voice/VoiceChannelView.svelte';
	import EncryptionPanel from '$components/encryption/EncryptionPanel.svelte';
	import Modal from '$components/common/Modal.svelte';

	let showGroupSettings = $state(false);
	let showEncryption = $state(false);
	let profileUserId = $state<string | null>(null);
	let callLoading = $state(false);

	let isDragging = $state(false);
	let dragCounter = 0;
	let isUploading = $state(false);

	const dmChannel = $derived($dmList.find(c => c.id === $page.params.channelId));
	const recipientName = $derived(dmChannel ? getDMDisplayName(dmChannel, $currentUser?.id) : 'Direct Message');
	const recipient = $derived(dmChannel ? getDMRecipient(dmChannel, $currentUser?.id) : undefined);
	const recipientStatus = $derived(recipient ? ($presenceMap.get(recipient.id) ?? 'offline') : undefined);
	const isGroupDM = $derived(dmChannel?.channel_type === 'group');
	const inCall = $derived($voiceChannelId === $page.params.channelId && $voiceState !== 'disconnected');

	async function startCall(withVideo: boolean = false) {
		callLoading = true;
		try {
			dismissIncomingCall($page.params.channelId);
			await joinVoice($page.params.channelId, '', recipientName);
			if (withVideo) {
				try { await toggleCamera(); } catch { /* camera failure is non-fatal */ }
			}
		} catch {
			addToast('Failed to start call', 'error');
		} finally {
			callLoading = false;
		}
	}

	// Ensure we're not in a guild context for DMs.
	$effect(() => {
		if ($currentGuildId) {
			setGuild(null);
		}
	});

	// Set current channel when route params change.
	$effect(() => {
		const channelId = $page.params.channelId;
		if (channelId) {
			setChannel(channelId);
			ackChannel(channelId);
		}
	});

	function handleDragEnter(e: DragEvent) {
		e.preventDefault();
		dragCounter++;
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

		const files = e.dataTransfer?.files;
		const channelId = $currentChannelId;
		if (!files?.length || !channelId) return;

		isUploading = true;
		try {
			const isEncrypted = !!$currentChannel?.encrypted;
			const opts: { attachment_ids?: string[]; encrypted?: boolean } = {};
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
</script>

<svelte:head>
	<title>{recipientName} — AmityVox</title>
</svelte:head>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
	class="relative flex h-full flex-col"
	ondragenter={handleDragEnter}
	ondragleave={handleDragLeave}
	ondragover={handleDragOver}
	ondrop={handleDrop}
>
	{#if isDragging}
		<div class="absolute inset-0 z-50 flex items-center justify-center bg-bg-primary/80 backdrop-blur-sm">
			<div class="flex flex-col items-center gap-3 rounded-xl border-2 border-dashed border-brand-500 bg-bg-secondary/90 px-12 py-10">
				<svg class="h-12 w-12 text-brand-400" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
					<path d="M3 16.5v2.25A2.25 2.25 0 005.25 21h13.5A2.25 2.25 0 0021 18.75V16.5m-13.5-9L12 3m0 0l4.5 4.5M12 3v13.5" />
				</svg>
				<span class="text-lg font-medium text-text-primary">Drop files to upload</span>
				<span class="text-sm text-text-muted">Files will be sent to this conversation</span>
			</div>
		</div>
	{/if}

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

	<header class="flex h-12 items-center gap-3 border-b border-bg-floating bg-bg-tertiary pl-12 pr-4 md:pl-4">
		{#if recipient && !isGroupDM}
			<button
				class="flex min-w-0 flex-1 items-center gap-3 rounded-md transition-colors hover:bg-bg-modifier"
				onclick={() => (profileUserId = recipient.id)}
				title="View profile"
			>
				<Avatar
					name={recipientName}
					src={recipient.avatar_id ? `/api/v1/files/${recipient.avatar_id}` : null}
					size="sm"
					status={recipientStatus}
				/>
				<h1 class="truncate font-semibold text-text-primary">{recipientName}</h1>
			</button>
			{#if inCall}
				<button
					class="rounded p-1.5 text-red-400 transition-colors hover:bg-bg-modifier hover:text-red-300"
					onclick={() => leaveVoice()}
					title="End Call"
				>
					<svg class="h-5 w-5" fill="currentColor" viewBox="0 0 24 24">
						<path d="M12 9c-1.6 0-3.15.25-4.6.72v3.1c0 .39-.23.74-.56.9-.98.49-1.87 1.12-2.66 1.85-.18.18-.43.28-.7.28-.28 0-.53-.11-.71-.29L.29 13.08a.956.956 0 01-.29-.7c0-.28.11-.53.29-.71C3.34 8.78 7.46 7 12 7s8.66 1.78 11.71 4.67c.18.18.29.43.29.71 0 .28-.11.53-.29.71l-2.48 2.48c-.18.18-.43.29-.71.29-.27 0-.52-.11-.7-.28a11.27 11.27 0 00-2.67-1.85.996.996 0 01-.56-.9v-3.1C15.15 9.25 13.6 9 12 9z" />
					</svg>
				</button>
			{:else}
				<button
					class="rounded p-1.5 text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-secondary disabled:opacity-50"
					onclick={() => startCall(false)}
					disabled={callLoading}
					title="Start Voice Call"
				>
					<svg class="h-5 w-5" fill="currentColor" viewBox="0 0 24 24">
						<path d="M20.01 15.38c-1.23 0-2.42-.2-3.53-.56a.977.977 0 00-1.01.24l-1.57 1.97c-2.83-1.35-5.48-3.9-6.89-6.83l1.95-1.66c.27-.28.35-.67.24-1.02-.37-1.11-.56-2.3-.56-3.53 0-.54-.45-.99-.99-.99H4.19C3.65 3 3 3.24 3 3.99 3 13.28 10.73 21 20.01 21c.71 0 .99-.63.99-1.18v-3.45c0-.54-.45-.99-.99-.99z" />
					</svg>
				</button>
				<button
					class="hidden rounded p-1.5 text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-secondary disabled:opacity-50 md:block"
					onclick={() => startCall(true)}
					disabled={callLoading}
					title="Start Video Call"
				>
					<svg class="h-5 w-5" fill="currentColor" viewBox="0 0 24 24">
						<path d="M17 10.5V7c0-.55-.45-1-1-1H4c-.55 0-1 .45-1 1v10c0 .55.45 1 1 1h12c.55 0 1-.45 1-1v-3.5l4 4v-11l-4 4z" />
					</svg>
				</button>
			{/if}
			<button
				class="hidden rounded p-1.5 transition-colors hover:bg-bg-modifier md:block {dmChannel?.encrypted ? 'text-green-400 hover:text-green-300' : 'text-text-muted hover:text-text-secondary'}"
				onclick={() => (showEncryption = true)}
				title={dmChannel?.encrypted ? 'Encryption enabled' : 'Set up encryption'}
			>
				<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					{#if dmChannel?.encrypted}
						<path stroke-linecap="round" stroke-linejoin="round" d="M16.5 10.5V6.75a4.5 4.5 0 10-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H6.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z" />
					{:else}
						<path stroke-linecap="round" stroke-linejoin="round" d="M13.5 10.5V6.75a4.5 4.5 0 119 0v3.75M3.75 21.75h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H3.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z" />
					{/if}
				</svg>
			</button>
		{:else if isGroupDM}
			<div class="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-brand-600 text-xs font-bold text-white">
				<svg class="h-4 w-4" fill="currentColor" viewBox="0 0 24 24">
					<path d="M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z" />
				</svg>
			</div>
			<div class="min-w-0 flex-1">
				<h1 class="truncate font-semibold text-text-primary">{recipientName}</h1>
				<p class="text-xs text-text-muted">{dmChannel?.recipients?.length ?? 0} members</p>
			</div>
			{#if inCall}
				<button
					class="rounded p-1.5 text-red-400 transition-colors hover:bg-bg-modifier hover:text-red-300"
					onclick={() => leaveVoice()}
					title="End Call"
				>
					<svg class="h-5 w-5" fill="currentColor" viewBox="0 0 24 24">
						<path d="M12 9c-1.6 0-3.15.25-4.6.72v3.1c0 .39-.23.74-.56.9-.98.49-1.87 1.12-2.66 1.85-.18.18-.43.28-.7.28-.28 0-.53-.11-.71-.29L.29 13.08a.956.956 0 01-.29-.7c0-.28.11-.53.29-.71C3.34 8.78 7.46 7 12 7s8.66 1.78 11.71 4.67c.18.18.29.43.29.71 0 .28-.11.53-.29.71l-2.48 2.48c-.18.18-.43.29-.71.29-.27 0-.52-.11-.7-.28a11.27 11.27 0 00-2.67-1.85.996.996 0 01-.56-.9v-3.1C15.15 9.25 13.6 9 12 9z" />
					</svg>
				</button>
			{:else}
				<button
					class="rounded p-1.5 text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-secondary disabled:opacity-50"
					onclick={() => startCall(false)}
					disabled={callLoading}
					title="Start Voice Call"
				>
					<svg class="h-5 w-5" fill="currentColor" viewBox="0 0 24 24">
						<path d="M20.01 15.38c-1.23 0-2.42-.2-3.53-.56a.977.977 0 00-1.01.24l-1.57 1.97c-2.83-1.35-5.48-3.9-6.89-6.83l1.95-1.66c.27-.28.35-.67.24-1.02-.37-1.11-.56-2.3-.56-3.53 0-.54-.45-.99-.99-.99H4.19C3.65 3 3 3.24 3 3.99 3 13.28 10.73 21 20.01 21c.71 0 .99-.63.99-1.18v-3.45c0-.54-.45-.99-.99-.99z" />
					</svg>
				</button>
				<button
					class="hidden rounded p-1.5 text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-secondary disabled:opacity-50 md:block"
					onclick={() => startCall(true)}
					disabled={callLoading}
					title="Start Video Call"
				>
					<svg class="h-5 w-5" fill="currentColor" viewBox="0 0 24 24">
						<path d="M17 10.5V7c0-.55-.45-1-1-1H4c-.55 0-1 .45-1 1v10c0 .55.45 1 1 1h12c.55 0 1-.45 1-1v-3.5l4 4v-11l-4 4z" />
					</svg>
				</button>
			{/if}
			<button
				class="rounded p-1.5 text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-secondary"
				onclick={() => (showGroupSettings = true)}
				title="Group Settings"
			>
				<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
					<path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
				</svg>
			</button>
		{:else}
			<svg class="h-5 w-5 text-text-muted" fill="currentColor" viewBox="0 0 24 24">
				<path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 3c1.66 0 3 1.34 3 3s-1.34 3-3 3-3-1.34-3-3 1.34-3 3-3zm0 14.2c-2.5 0-4.71-1.28-6-3.22.03-1.99 4-3.08 6-3.08 1.99 0 5.97 1.09 6 3.08-1.29 1.94-3.5 3.22-6 3.22z" />
			</svg>
			<h1 class="font-semibold text-text-primary">Direct Message</h1>
		{/if}
	</header>
	{#if inCall}
		<VoiceChannelView channelId={$page.params.channelId} guildId="" />
	{:else}
		<MessageList />
		<TypingIndicator typingUsers={$currentTypingUsers} />
		<MessageInput />
	{/if}
</div>

{#if isGroupDM && dmChannel}
	<GroupDMSettingsPanel channel={dmChannel} bind:open={showGroupSettings} onclose={() => (showGroupSettings = false)} />
{/if}

<ProfileModal userId={profileUserId} open={!!profileUserId} onclose={() => (profileUserId = null)} />

{#if dmChannel}
	<Modal bind:open={showEncryption} title="Encryption" onclose={() => (showEncryption = false)}>
		<EncryptionPanel
			channelId={dmChannel.id}
			encrypted={dmChannel.encrypted ?? false}
			onchange={() => { showEncryption = false; }}
		/>
	</Modal>
{/if}
