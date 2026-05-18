<script lang="ts">
	import { goto } from '$app/navigation';
	import { guildMutePrefs, isGuildMuted, muteGuild, unmuteGuild } from '$lib/stores/muting';
	import type { Guild } from '$lib/types';

	interface Props {
		x: number;
		y: number;
		guild: Guild;
		canManageGuild: boolean;
		onclose: () => void;
		oninvite: () => void;
	}

	let { x, y, guild, canManageGuild, onclose, oninvite }: Props = $props();
	let showMuteSubmenu = $state(false);

	const muteDurations = [
		{ label: '15 Minutes', ms: 15 * 60 * 1000 },
		{ label: '1 Hour', ms: 60 * 60 * 1000 },
		{ label: '8 Hours', ms: 8 * 60 * 60 * 1000 },
		{ label: '24 Hours', ms: 24 * 60 * 60 * 1000 },
		{ label: 'Until I turn it back on', ms: 0 }
	];

	const isMuted = $derived(Boolean($guildMutePrefs) && isGuildMuted(guild.id));
</script>

<div
	class="fixed z-50 min-w-[180px] rounded-md bg-bg-floating p-1 shadow-lg"
	style="left: {x}px; top: {y}px;"
	onclick={(e) => e.stopPropagation()}
	onkeydown={(e) => e.stopPropagation()}
	role="menu"
	tabindex="-1"
>
	{#if isMuted}
		<button
			class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
			onclick={() => { unmuteGuild(guild.id); onclose(); }}
		>
			<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M15.536 8.464a5 5 0 010 7.072M18.364 5.636a9 9 0 010 12.728M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
			</svg>
			Unmute Server
		</button>
	{:else}
		<div class="relative">
			<button
				class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
				onclick={() => (showMuteSubmenu = !showMuteSubmenu)}
			>
				<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
					<path d="M17 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2" />
				</svg>
				Mute Server
				<svg class="ml-auto h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M9 5l7 7-7 7" />
				</svg>
			</button>
			{#if showMuteSubmenu}
				{@const submenuLeft = x + 300 < (typeof window === 'undefined' ? 1024 : window.innerWidth)}
				<div class="absolute top-0 min-w-[180px] rounded-md bg-bg-floating p-1 shadow-lg {submenuLeft ? 'left-full ml-1' : 'right-full mr-1'}">
					{#each muteDurations as opt}
						<button
							class="flex w-full items-center rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
							onclick={() => { muteGuild(guild.id, opt.ms || undefined); onclose(); }}
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
		onclick={() => { oninvite(); onclose(); }}
	>
		<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path d="M18 9v3m0 0v3m0-3h3m-3 0h-3m-2-5a4 4 0 11-8 0 4 4 0 018 0zM3 20a6 6 0 0112 0v1H3v-1z" />
		</svg>
		Invite People
	</button>
	{#if canManageGuild}
		<button
			class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
			onclick={() => { goto(`/app/guilds/${guild.id}/settings`); onclose(); }}
		>
			<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M12 6V4m0 2a2 2 0 100 4m0-4a2 2 0 110 4m-6 8a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4m6 6v10m6-2a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4" />
			</svg>
			Server Settings
		</button>
	{/if}
</div>
