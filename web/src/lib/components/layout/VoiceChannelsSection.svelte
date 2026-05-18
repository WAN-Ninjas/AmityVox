<script lang="ts">
	import Avatar from '$components/common/Avatar.svelte';
	import { channelVoiceUsers, joinVoice, voiceChannelId } from '$lib/stores/voice';
	import type { Channel } from '$lib/types';
	import { avatarUrl } from '$lib/utils/avatar';

	interface Props {
		channels: Channel[];
		currentChannelId: string | null;
		guildId: string | null;
		onchannelclick: (channelId: string) => void;
		oncontextmenu: (event: MouseEvent, channel: Channel) => void;
	}

	let { channels, currentChannelId, guildId, onchannelclick, oncontextmenu }: Props = $props();

	function handleVoiceJoin(channel: Channel) {
		if (!guildId) return;
		joinVoice(channel.id, guildId, channel.name ?? '');
	}
</script>

{#each channels as channel (channel.id)}
	{@const voiceUsers = $channelVoiceUsers.get(channel.id)}
	<button
		class="mb-0.5 flex w-full items-center gap-1.5 rounded px-2 py-1.5 text-left text-sm transition-colors {currentChannelId === channel.id ? 'bg-bg-modifier text-text-primary' : 'text-text-muted hover:bg-bg-modifier hover:text-text-secondary'}"
		onclick={() => onchannelclick(channel.id)}
		ondblclick={() => handleVoiceJoin(channel)}
		oncontextmenu={(e) => oncontextmenu(e, channel)}
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
