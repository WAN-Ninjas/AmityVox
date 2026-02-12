<script lang="ts">
	import { page } from '$app/stores';
	import { setChannel, currentChannelId } from '$lib/stores/channels';
	import { currentGuildId, setGuild } from '$lib/stores/guilds';
	import { currentTypingUsers } from '$lib/stores/typing';
	import { ackChannel } from '$lib/stores/unreads';
	import { api } from '$lib/api/client';
	import { appendMessage } from '$lib/stores/messages';
	import { addToast } from '$lib/stores/toast';
	import MessageList from '$components/chat/MessageList.svelte';
	import MessageInput from '$components/chat/MessageInput.svelte';
	import TypingIndicator from '$components/chat/TypingIndicator.svelte';

	let isDragging = $state(false);
	let dragCounter = 0;
	let isUploading = $state(false);

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
			for (const file of files) {
				const uploaded = await api.uploadFile(file);
				const sent = await api.sendMessage(channelId, '', { attachment_ids: [uploaded.id] });
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
	<title>DM — AmityVox</title>
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

	<header class="flex h-12 items-center border-b border-bg-floating bg-bg-tertiary px-4">
		<svg class="mr-2 h-5 w-5 text-text-muted" fill="currentColor" viewBox="0 0 24 24">
			<path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 3c1.66 0 3 1.34 3 3s-1.34 3-3 3-3-1.34-3-3 1.34-3 3-3zm0 14.2c-2.5 0-4.71-1.28-6-3.22.03-1.99 4-3.08 6-3.08 1.99 0 5.97 1.09 6 3.08-1.29 1.94-3.5 3.22-6 3.22z" />
		</svg>
		<h1 class="font-semibold text-text-primary">Direct Message</h1>
	</header>
	<MessageList />
	<TypingIndicator typingUsers={$currentTypingUsers} />
	<MessageInput />
</div>
