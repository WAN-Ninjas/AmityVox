<!-- VoiceChannelView.svelte — Main voice channel experience with participant grid, controls, and text-in-voice. -->
<script lang="ts">
	import { currentUser } from '$lib/stores/auth';
	import { currentChannel } from '$lib/stores/channels';
	import {
		voiceState,
		selfMute,
		selfDeaf,
		participantList,
		joinVoice,
		leaveVoice,
		toggleMute,
		toggleDeafen,
		screenShareElement,
		screenShareUserId,
		type VoiceParticipant
	} from '$lib/stores/voice';
	import { addToast } from '$lib/stores/toast';
	import Avatar from '$components/common/Avatar.svelte';
	import MessageList from '$components/chat/MessageList.svelte';
	import MessageInput from '$components/chat/MessageInput.svelte';
	import VoiceControls from './VoiceControls.svelte';
	import Soundboard from './Soundboard.svelte';
	import ScreenShareControls from './ScreenShareControls.svelte';

	interface Props {
		channelId: string;
		guildId: string;
	}

	let { channelId, guildId }: Props = $props();

	import { onMount } from 'svelte';

	let joining = $state(false);
	let showSettings = $state(false);
	let showSoundboard = $state(false);
	let showScreenShare = $state(false);
	let textCollapsed = $state(false);
	let screenShareContainer: HTMLDivElement;

	const connected = $derived($voiceState === 'connected');
	const connecting = $derived($voiceState === 'connecting');

	// Attach/detach the screen share video element when it changes.
	$effect(() => {
		const videoEl = $screenShareElement;
		const container = screenShareContainer;
		if (!container) return;
		// Clear previous content
		container.innerHTML = '';
		if (videoEl) {
			container.appendChild(videoEl);
		}
	});

	async function handleJoin() {
		joining = true;
		try {
			await joinVoice(channelId, guildId, $currentChannel?.name ?? 'Voice');
		} catch (err: any) {
			addToast(err.message || 'Failed to join voice channel', 'error');
		} finally {
			joining = false;
		}
	}

	async function handleLeave() {
		try {
			await leaveVoice();
		} catch (err: any) {
			addToast(err.message || 'Failed to leave voice channel', 'error');
		}
	}
</script>

<div class="flex h-full flex-col">
	<!-- Voice area -->
	<div class="flex min-h-0 flex-1 flex-col">
		{#if !connected && !connecting}
			<!-- Disconnected state: Join button -->
			<div class="flex flex-1 flex-col items-center justify-center gap-6 p-8">
				<div class="flex flex-col items-center gap-3 text-center">
					<div class="flex h-20 w-20 items-center justify-center rounded-full bg-bg-modifier">
						<svg class="h-10 w-10 text-text-muted" fill="currentColor" viewBox="0 0 24 24">
							<path d="M12 3a1 1 0 0 0-1 1v8a3 3 0 1 0 6 0V4a1 1 0 1 0-2 0v8a1 1 0 1 1-2 0V4a1 1 0 0 0-1-1zM7 12a5 5 0 0 0 10 0h2a7 7 0 0 1-6 6.92V21h-2v-2.08A7 7 0 0 1 5 12h2z" />
						</svg>
					</div>
					<h2 class="text-xl font-bold text-text-primary">{$currentChannel?.name ?? 'Voice Channel'}</h2>
					{#if $currentChannel?.topic}
						<p class="max-w-md text-sm text-text-muted">{$currentChannel.topic}</p>
					{/if}
					<div class="flex items-center gap-3 text-xs text-text-muted">
						{#if $currentChannel && $currentChannel.user_limit > 0}
							<span>{$currentChannel.user_limit} user limit</span>
							<span>&middot;</span>
						{/if}
						{#if $currentChannel}
							<span>{Math.floor($currentChannel.bitrate / 1000)}kbps</span>
						{/if}
					</div>
				</div>

				{#if $participantList.length > 0}
					<div class="flex flex-col items-center gap-2">
						<p class="text-xs font-medium text-text-muted">{$participantList.length} connected</p>
						<div class="flex -space-x-2">
							{#each $participantList.slice(0, 8) as p (p.userId)}
								<div class="relative" title={p.displayName ?? p.username}>
									<Avatar name={p.displayName ?? p.username} size="sm" />
								</div>
							{/each}
							{#if $participantList.length > 8}
								<div class="flex h-8 w-8 items-center justify-center rounded-full bg-bg-modifier text-xs font-medium text-text-muted">
									+{$participantList.length - 8}
								</div>
							{/if}
						</div>
					</div>
				{/if}

				<button
					class="rounded-lg bg-green-600 px-8 py-3 text-sm font-semibold text-white transition-colors hover:bg-green-700 disabled:opacity-50"
					onclick={handleJoin}
					disabled={joining}
				>
					{#if joining}
						<span class="flex items-center gap-2">
							<svg class="h-4 w-4 animate-spin" fill="none" viewBox="0 0 24 24">
								<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
								<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
							</svg>
							Connecting...
						</span>
					{:else}
						Join Voice
					{/if}
				</button>
			</div>
		{:else if connecting}
			<!-- Connecting state -->
			<div class="flex flex-1 items-center justify-center">
				<div class="flex flex-col items-center gap-3">
					<svg class="h-8 w-8 animate-spin text-brand-400" fill="none" viewBox="0 0 24 24">
						<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
						<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
					</svg>
					<p class="text-sm text-text-muted">Connecting to voice...</p>
				</div>
			</div>
		{:else}
			<!-- Connected state: Screen share + Participant grid -->

			<!-- Screen share video (always in DOM for bind:this, hidden when inactive) -->
			<div class="flex flex-col border-b border-bg-floating bg-black" class:hidden={!$screenShareElement}>
				<div class="flex items-center gap-2 bg-bg-secondary px-3 py-1.5">
					<span class="h-2 w-2 animate-pulse rounded-full bg-brand-500"></span>
					<span class="text-xs font-medium text-text-secondary">
						{#each $participantList.filter(p => p.userId === $screenShareUserId) as sharer}
							{sharer.displayName ?? sharer.username} is sharing their screen
						{:else}
							Screen share active
						{/each}
					</span>
				</div>
				<div
					bind:this={screenShareContainer}
					class="flex max-h-[60vh] min-h-[200px] items-center justify-center bg-black"
				></div>
			</div>

			<div class="flex-1 overflow-y-auto p-4">
				<div class="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5">
					{#each $participantList as participant (participant.userId)}
						{@const isSelf = participant.userId === $currentUser?.id}
						<div
							class="flex flex-col items-center gap-2 rounded-xl bg-bg-secondary p-4 transition-all {participant.speaking ? 'ring-2 ring-green-500' : ''}"
						>
							<div class="relative">
								<div class="h-16 w-16 overflow-hidden rounded-full {participant.speaking ? 'ring-2 ring-green-400 ring-offset-2 ring-offset-bg-secondary' : ''}">
									<Avatar
										name={participant.displayName ?? participant.username}
										size="lg"
									/>
								</div>
								<!-- Mute/deafen indicators -->
								{#if participant.deafened}
									<div class="absolute -bottom-1 -right-1 flex h-5 w-5 items-center justify-center rounded-full bg-red-500">
										<svg class="h-3 w-3 text-white" fill="none" stroke="currentColor" stroke-width="2.5" viewBox="0 0 24 24">
											<path d="M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
											<path d="M17 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2" />
										</svg>
									</div>
								{:else if participant.muted}
									<div class="absolute -bottom-1 -right-1 flex h-5 w-5 items-center justify-center rounded-full bg-red-500">
										<svg class="h-3 w-3 text-white" fill="none" stroke="currentColor" stroke-width="2.5" viewBox="0 0 24 24">
											<path d="M19 19L5 5m14 0v8a3 3 0 01-5.12 2.12M12 19v2m-4-4h8" />
										</svg>
									</div>
								{/if}
							</div>
							<span class="max-w-full truncate text-xs font-medium {isSelf ? 'text-brand-400' : 'text-text-primary'}">
								{participant.displayName ?? participant.username}
							</span>
						</div>
					{/each}
				</div>
			</div>

			<!-- Voice control bar -->
			<div class="flex items-center justify-center gap-2 border-t border-bg-floating bg-bg-secondary px-4 py-3">
				<!-- Mute -->
				<button
					class="flex h-10 w-10 items-center justify-center rounded-full transition-colors {$selfMute ? 'bg-red-500/20 text-red-400 hover:bg-red-500/30' : 'bg-bg-modifier text-text-secondary hover:bg-bg-floating hover:text-text-primary'}"
					onclick={toggleMute}
					title={$selfMute ? 'Unmute' : 'Mute'}
				>
					{#if $selfMute}
						<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M19 19L5 5m14 0v8a3 3 0 01-5.12 2.12M12 19v2m-4-4h8" />
						</svg>
					{:else}
						<svg class="h-5 w-5" fill="currentColor" viewBox="0 0 24 24">
							<path d="M12 3a1 1 0 0 0-1 1v8a3 3 0 1 0 6 0V4a1 1 0 1 0-2 0v8a1 1 0 1 1-2 0V4a1 1 0 0 0-1-1zM7 12a5 5 0 0 0 10 0h2a7 7 0 0 1-6 6.92V21h-2v-2.08A7 7 0 0 1 5 12h2z" />
						</svg>
					{/if}
				</button>

				<!-- Deafen -->
				<button
					class="flex h-10 w-10 items-center justify-center rounded-full transition-colors {$selfDeaf ? 'bg-red-500/20 text-red-400 hover:bg-red-500/30' : 'bg-bg-modifier text-text-secondary hover:bg-bg-floating hover:text-text-primary'}"
					onclick={toggleDeafen}
					title={$selfDeaf ? 'Undeafen' : 'Deafen'}
				>
					{#if $selfDeaf}
						<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
							<path d="M17 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2" />
						</svg>
					{:else}
						<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M15.536 8.464a5 5 0 010 7.072M18.364 5.636a9 9 0 010 12.728M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
						</svg>
					{/if}
				</button>

				<!-- Screen Share -->
				<button
					class="flex h-10 w-10 items-center justify-center rounded-full bg-bg-modifier text-text-secondary transition-colors hover:bg-bg-floating hover:text-text-primary"
					onclick={() => (showScreenShare = !showScreenShare)}
					title="Screen Share"
				>
					<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
					</svg>
				</button>

				<!-- Soundboard -->
				<button
					class="flex h-10 w-10 items-center justify-center rounded-full bg-bg-modifier text-text-secondary transition-colors hover:bg-bg-floating hover:text-text-primary"
					onclick={() => (showSoundboard = !showSoundboard)}
					title="Soundboard"
				>
					<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M9 19V6l12-3v13M9 19c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zm12-3c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zM9 10l12-3" />
					</svg>
				</button>

				<!-- Settings -->
				<button
					class="flex h-10 w-10 items-center justify-center rounded-full bg-bg-modifier text-text-secondary transition-colors hover:bg-bg-floating hover:text-text-primary"
					onclick={() => (showSettings = !showSettings)}
					title="Voice Settings"
				>
					<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
						<circle cx="12" cy="12" r="3" />
					</svg>
				</button>

				<!-- Disconnect -->
				<button
					class="flex h-10 w-10 items-center justify-center rounded-full bg-red-500/20 text-red-400 transition-colors hover:bg-red-500/30"
					onclick={handleLeave}
					title="Disconnect"
				>
					<svg class="h-5 w-5" fill="currentColor" viewBox="0 0 24 24">
						<path d="M18.36 5.64a1 1 0 00-1.41 0l-1.42 1.42A7 7 0 005 12a6.93 6.93 0 001.07 3.69l-1.43 1.43a1 1 0 101.42 1.42l1.42-1.43A6.93 6.93 0 0012 19a7 7 0 004.95-2.05l1.41 1.41a1 1 0 101.41-1.41l-1.41-1.42A6.93 6.93 0 0019 12a6.93 6.93 0 00-1.07-3.69l1.43-1.43a1 1 0 000-1.24zM12 17a5 5 0 01-5-5 4.93 4.93 0 01.68-2.49l6.81 6.81A4.93 4.93 0 0112 17zm4.32-2.51l-6.81-6.81A4.93 4.93 0 0112 7a5 5 0 015 5 4.93 4.93 0 01-.68 2.49z" />
					</svg>
				</button>
			</div>
		{/if}
	</div>

	<!-- Settings / Soundboard / Screen Share panels -->
	{#if connected && showSettings}
		<div class="border-t border-bg-floating">
			<div class="flex items-center justify-between bg-bg-secondary px-4 py-2">
				<h3 class="text-sm font-semibold text-text-primary">Voice Settings</h3>
				<button class="text-text-muted hover:text-text-primary" onclick={() => (showSettings = false)}>
					<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			</div>
			<div class="max-h-60 overflow-y-auto bg-bg-primary p-4">
				<VoiceControls
					{channelId}
					{guildId}
					{connected}
					selfMute={$selfMute}
					selfDeaf={$selfDeaf}
				/>
			</div>
		</div>
	{/if}

	{#if connected && showSoundboard}
		<div class="border-t border-bg-floating">
			<div class="flex items-center justify-between bg-bg-secondary px-4 py-2">
				<h3 class="text-sm font-semibold text-text-primary">Soundboard</h3>
				<button class="text-text-muted hover:text-text-primary" onclick={() => (showSoundboard = false)}>
					<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			</div>
			<div class="max-h-60 overflow-y-auto bg-bg-primary p-4">
				<Soundboard {guildId} {channelId} />
			</div>
		</div>
	{/if}

	{#if connected && showScreenShare}
		<div class="border-t border-bg-floating">
			<div class="flex items-center justify-between bg-bg-secondary px-4 py-2">
				<h3 class="text-sm font-semibold text-text-primary">Screen Share</h3>
				<button class="text-text-muted hover:text-text-primary" onclick={() => (showScreenShare = false)}>
					<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			</div>
			<div class="max-h-60 overflow-y-auto bg-bg-primary p-4">
				<ScreenShareControls
					{channelId}
					{guildId}
					{connected}
					currentUserId={$currentUser?.id ?? ''}
				/>
			</div>
		</div>
	{/if}

	<!-- Text-in-voice section -->
	<div class="flex flex-col border-t border-bg-floating {textCollapsed ? '' : 'min-h-0 flex-1'}">
		<button
			class="flex items-center gap-2 bg-bg-secondary px-4 py-1.5 text-xs font-medium text-text-muted hover:text-text-secondary"
			onclick={() => (textCollapsed = !textCollapsed)}
		>
			<svg
				class="h-3 w-3 transition-transform duration-200 {textCollapsed ? '-rotate-90' : ''}"
				fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"
			>
				<path d="M19 9l-7 7-7-7" />
			</svg>
			Text Chat
		</button>
		{#if !textCollapsed}
			<div class="flex min-h-0 flex-1 flex-col">
				<MessageList />
				<MessageInput />
			</div>
		{/if}
	</div>
</div>
