<!-- VoiceConnectionBar.svelte — Persistent bar shown above user panel when connected to voice. -->
<script lang="ts">
	import {
		isVoiceConnected,
		voiceChannelName,
		voiceGuildId,
		selfMute,
		selfDeaf,
		toggleMute,
		toggleDeafen,
		leaveVoice
	} from '$lib/stores/voice';
</script>

{#if $isVoiceConnected}
	<div class="flex items-center gap-2 border-t border-bg-floating bg-green-600/10 px-3 py-2">
		<div class="min-w-0 flex-1">
			<div class="flex items-center gap-1.5">
				<span class="h-2 w-2 shrink-0 rounded-full bg-green-500"></span>
				<span class="truncate text-xs font-semibold text-green-400">Voice Connected</span>
			</div>
			<p class="truncate text-2xs text-text-muted">{$voiceChannelName ?? 'Voice Channel'}</p>
		</div>

		<!-- Mute toggle -->
		<button
			class="flex h-7 w-7 shrink-0 items-center justify-center rounded text-text-muted transition-colors {$selfMute ? 'bg-red-500/20 text-red-400' : 'hover:bg-bg-modifier hover:text-text-primary'}"
			onclick={toggleMute}
			title={$selfMute ? 'Unmute' : 'Mute'}
		>
			{#if $selfMute}
				<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2.5" viewBox="0 0 24 24">
					<path d="M19 19L5 5m14 0v8a3 3 0 01-5.12 2.12M12 19v2m-4-4h8" />
				</svg>
			{:else}
				<svg class="h-3.5 w-3.5" fill="currentColor" viewBox="0 0 24 24">
					<path d="M12 3a1 1 0 0 0-1 1v8a3 3 0 1 0 6 0V4a1 1 0 1 0-2 0v8a1 1 0 1 1-2 0V4a1 1 0 0 0-1-1zM7 12a5 5 0 0 0 10 0h2a7 7 0 0 1-6 6.92V21h-2v-2.08A7 7 0 0 1 5 12h2z" />
				</svg>
			{/if}
		</button>

		<!-- Deafen toggle -->
		<button
			class="flex h-7 w-7 shrink-0 items-center justify-center rounded text-text-muted transition-colors {$selfDeaf ? 'bg-red-500/20 text-red-400' : 'hover:bg-bg-modifier hover:text-text-primary'}"
			onclick={toggleDeafen}
			title={$selfDeaf ? 'Undeafen' : 'Deafen'}
		>
			{#if $selfDeaf}
				<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2.5" viewBox="0 0 24 24">
					<path d="M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
					<path d="M17 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2" />
				</svg>
			{:else}
				<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2.5" viewBox="0 0 24 24">
					<path d="M15.536 8.464a5 5 0 010 7.072M18.364 5.636a9 9 0 010 12.728M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
				</svg>
			{/if}
		</button>

		<!-- Disconnect -->
		<button
			class="flex h-7 w-7 shrink-0 items-center justify-center rounded text-red-400 transition-colors hover:bg-red-500/20"
			onclick={leaveVoice}
			title="Disconnect"
		>
			<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2.5" viewBox="0 0 24 24">
				<path d="M16 8l-8 8m0-8l8 8" />
			</svg>
		</button>
	</div>
{/if}
