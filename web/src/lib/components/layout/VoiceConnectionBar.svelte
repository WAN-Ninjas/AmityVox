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
	<div class="flex items-center gap-2.5 border-t border-bg-floating bg-green-600/10 px-3 py-2.5">
		<div class="min-w-0 flex-1">
			<div class="flex items-center gap-1.5">
				<span class="h-2.5 w-2.5 shrink-0 rounded-full bg-green-500"></span>
				<span class="truncate text-sm font-semibold text-green-400">Voice Connected</span>
			</div>
			<p class="truncate text-xs text-text-muted">{$voiceChannelName ?? 'Voice Channel'}</p>
		</div>

		<!-- Mute toggle -->
		<button
			class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-text-muted transition-colors {$selfMute ? 'bg-red-500/20 text-red-400' : 'hover:bg-bg-modifier hover:text-text-primary'}"
			onclick={toggleMute}
			title={$selfMute ? 'Unmute' : 'Mute'}
		>
			{#if $selfMute}
				<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M19 19L5 5m14 0v8a3 3 0 01-5.12 2.12M12 19v2m-4-4h8" />
				</svg>
			{:else}
				<svg class="h-5 w-5" fill="currentColor" viewBox="0 0 24 24">
					<path d="M12 2c-1.66 0-3 1.34-3 3v6c0 1.66 1.34 3 3 3s3-1.34 3-3V5c0-1.66-1.34-3-3-3zm5 9c0 2.76-2.24 5-5 5s-5-2.24-5-5H5c0 3.53 2.61 6.43 6 6.92V21h2v-3.08c3.39-.49 6-3.39 6-6.92h-2z" />
				</svg>
			{/if}
		</button>

		<!-- Deafen toggle -->
		<button
			class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-text-muted transition-colors {$selfDeaf ? 'bg-red-500/20 text-red-400' : 'hover:bg-bg-modifier hover:text-text-primary'}"
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

		<!-- Disconnect -->
		<button
			class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-red-400 transition-colors hover:bg-red-500/20"
			onclick={leaveVoice}
			title="Disconnect"
		>
			<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M16 8l-8 8m0-8l8 8" />
			</svg>
		</button>
	</div>
{/if}
