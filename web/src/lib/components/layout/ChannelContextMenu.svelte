<script lang="ts">
	import { channelMutePrefs, isChannelMuted, muteChannel, unmuteChannel } from '$lib/stores/muting';

	interface ChannelGroupSummary {
		id: string;
		name: string;
		color: string;
		channels: string[];
	}
	interface ChannelContextMenuState {
		x: number;
		y: number;
		channelId: string;
		channelName: string;
		archived: boolean;
	}
	interface Props {
		menu: ChannelContextMenuState;
		canManageChannels: boolean;
		channelGroups: ChannelGroupSummary[];
		getthreadfilter: (channelId: string) => number | null;
		onthreadfilter: (channelId: string, minutes: number | null) => void;
		onedit: (channelId: string, channelName: string) => void;
		onremovefromgroup: (channelId: string) => void;
		onaddtogroup: (groupId: string, channelId: string) => void;
		ondelete: (channelId: string) => void;
		onclose: () => void;
	}
	let { menu, canManageChannels, channelGroups, getthreadfilter, onthreadfilter, onedit, onremovefromgroup, onaddtogroup, ondelete, onclose }: Props = $props();

	let showThreadFilterSubmenu = $state(false);
	let showMuteSubmenu = $state(false);
	let showMoveToGroupSubmenu = $state(false);

	const muteDurations = [{ label: '15 Minutes', ms: 15 * 60 * 1000 }, { label: '1 Hour', ms: 60 * 60 * 1000 }, { label: '8 Hours', ms: 8 * 60 * 60 * 1000 }, { label: '24 Hours', ms: 24 * 60 * 60 * 1000 }, { label: 'Until I turn it back on', ms: 0 }];
	const threadFilterOptions = [{ label: 'All', value: null }, { label: 'Last Hour', value: 60 }, { label: 'Last 6 Hours', value: 360 }, { label: 'Last 12 Hours', value: 720 }, { label: 'Last Day', value: 1440 }];

	const currentGroup = $derived(channelGroups.find(group => group.channels.includes(menu.channelId)) ?? null);
	const channelMuted = $derived(Boolean($channelMutePrefs) && isChannelMuted(menu.channelId));
	const submenuLeft = $derived(menu.x + 300 < (typeof window === 'undefined' ? 1024 : window.innerWidth));
</script>

<div
	class="fixed z-50 min-w-[160px] rounded-md bg-bg-floating p-1 shadow-lg"
	style="left: {menu.x}px; top: {menu.y}px;"
	onclick={(e) => e.stopPropagation()}
	onkeydown={(e) => e.stopPropagation()}
	role="menu"
	tabindex="-1"
>
	{#if canManageChannels}
		<button
			class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
			onclick={() => onedit(menu.channelId, menu.channelName)}
		>
			<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
			</svg>
			Edit Channel
		</button>
	{/if}
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
			{@const currentFilter = getthreadfilter(menu.channelId)}
			<div class="absolute top-0 max-h-[50vh] min-w-[140px] overflow-y-auto rounded-md bg-bg-floating p-1 shadow-lg {submenuLeft ? 'left-full ml-1' : 'right-full mr-1'}">
				{#each threadFilterOptions as option}
					<button
						class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm transition-colors {currentFilter === option.value ? 'text-brand-400' : 'text-text-secondary'} hover:bg-brand-500 hover:text-white"
						onclick={() => { onthreadfilter(menu.channelId, option.value); showThreadFilterSubmenu = false; }}
					>
						{option.label}
					</button>
				{/each}
			</div>
		{/if}
	</div>
	{#if channelMuted}
		<button
			class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
			onclick={() => { unmuteChannel(menu.channelId); onclose(); }}
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
				onclick={() => (showMuteSubmenu = !showMuteSubmenu)}
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
			{#if showMuteSubmenu}
				<div class="absolute top-0 min-w-[180px] rounded-md bg-bg-floating p-1 shadow-lg {submenuLeft ? 'left-full ml-1' : 'right-full mr-1'}">
					{#each muteDurations as opt}
						<button
							class="flex w-full items-center rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
							onclick={() => { muteChannel(menu.channelId, opt.ms || undefined); onclose(); }}
						>
							{opt.label}
						</button>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
	{#if channelGroups.length > 0}
		{#if currentGroup}
			<button
				class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
				onclick={() => onremovefromgroup(menu.channelId)}
			>
				<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
				</svg>
				Remove from Group
			</button>
		{:else}
			<div class="relative">
				<button
					class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
					onclick={() => (showMoveToGroupSubmenu = !showMoveToGroupSubmenu)}
				>
					<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" />
					</svg>
					Move to Group
					<svg class="ml-auto h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M9 5l7 7-7 7" />
					</svg>
				</button>
				{#if showMoveToGroupSubmenu}
					<div class="absolute top-0 min-w-[140px] rounded-md bg-bg-floating p-1 shadow-lg {submenuLeft ? 'left-full ml-1' : 'right-full mr-1'}">
						{#each channelGroups as group}
							<button
								class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
								onclick={() => onaddtogroup(group.id, menu.channelId)}
							>
								<span class="h-2 w-2 rounded-full shrink-0" style="background-color: {group.color}"></span>
								{group.name}
							</button>
						{/each}
					</div>
				{/if}
			</div>
		{/if}
	{/if}
	{#if canManageChannels}
		<button
			class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-red-400 hover:bg-red-500 hover:text-white"
			onclick={() => { ondelete(menu.channelId); onclose(); }}
		>
			<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
			</svg>
			Delete Channel
		</button>
	{/if}
</div>
