<script lang="ts">
	import { goto } from '$app/navigation';
	import Avatar from '$components/common/Avatar.svelte';
	import { currentUser } from '$lib/stores/auth';
	import { currentChannelId } from '$lib/stores/channels';
	import { dmList } from '$lib/stores/dms';
	import { presenceMap } from '$lib/stores/presence';
	import { pendingIncomingCount } from '$lib/stores/relationships';
	import { mentionCounts, unreadCounts } from '$lib/stores/unreads';
	import { unlockedChannels } from '$lib/encryption/e2eeManager';
	import { isChannelMuted } from '$lib/stores/muting';
	import { avatarUrl } from '$lib/utils/avatar';
	import { getDMDisplayName, getDMRecipient } from '$lib/utils/dm';
	import type { Channel } from '$lib/types';

	interface Props {
		collapsed: boolean;
		ontoggle: () => void;
		oncreategroup: () => void;
		onprofile: (userId: string) => void;
		oncontextmenu: (event: MouseEvent, channel: Channel) => void;
	}

	let { collapsed, ontoggle, oncreategroup, onprofile, oncontextmenu }: Props = $props();
</script>

<div class="mb-1 flex items-center justify-between px-1">
	<button class="mb-0.5 flex w-full items-center gap-1.5 rounded px-2 py-1.5 text-left text-sm text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-secondary" onclick={() => goto('/app/friends')}>
		<svg class="h-4 w-4 shrink-0" fill="currentColor" viewBox="0 0 24 24">
			<path d="M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z" />
		</svg>
		<span class="flex-1">Friends</span>
		{#if $pendingIncomingCount > 0}
			<span class="flex h-4 min-w-4 items-center justify-center rounded-full bg-red-500 px-1 text-2xs font-bold text-white">{$pendingIncomingCount > 99 ? '99+' : $pendingIncomingCount}</span>
		{/if}
	</button>
</div>

<div class="mb-1 flex items-center justify-between px-1 pt-2">
	<button class="flex items-center gap-1 text-2xs font-bold uppercase tracking-wide text-text-muted hover:text-text-secondary" onclick={ontoggle} title={collapsed ? 'Expand Direct Messages' : 'Collapse Direct Messages'}>
		<svg class="h-3 w-3 shrink-0 transition-transform duration-200 {collapsed ? '-rotate-90' : ''}" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path d="M19 9l-7 7-7-7" />
		</svg>
		Direct Messages
	</button>
	<button class="rounded p-0.5 text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-secondary" onclick={oncreategroup} title="Create Group DM">
		<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" />
		</svg>
	</button>
</div>

{#if !collapsed}
	{#if $dmList.length === 0}
		<p class="px-2 py-2 text-xs text-text-muted">No conversations yet.</p>
	{:else}
		{#each $dmList as dm (dm.id)}
			{@const dmUnread = $unreadCounts.get(dm.id) ?? 0}
			{@const dmMentions = $mentionCounts.get(dm.id) ?? 0}
			{@const dmName = getDMDisplayName(dm, $currentUser?.id)}
			{@const dmRecipient = getDMRecipient(dm, $currentUser?.id)}
			{@const dmMuted = isChannelMuted(dm.id)}
			<button
				class="mb-0.5 flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm transition-colors {dmMuted ? 'opacity-60' : ''} {$currentChannelId === dm.id ? 'bg-bg-modifier text-text-primary' : dmUnread > 0 && !dmMuted ? 'text-text-primary font-semibold hover:bg-bg-modifier' : 'text-text-muted hover:bg-bg-modifier hover:text-text-secondary'}"
				onclick={() => goto(`/app/dms/${dm.id}`)}
				oncontextmenu={(e) => oncontextmenu(e, dm)}
			>
				<span
					class="cursor-pointer"
					onclick={(e) => { if (dmRecipient) { e.stopPropagation(); onprofile(dmRecipient.id); } }}
					onkeydown={(e) => { if ((e.key === 'Enter' || e.key === ' ') && dmRecipient) { e.preventDefault(); e.stopPropagation(); onprofile(dmRecipient.id); } }}
					role="button"
					tabindex="0"
				>
					<Avatar name={dmName} src={dmRecipient?.avatar_id ? avatarUrl(dmRecipient.avatar_id, dmRecipient.instance_id || undefined) : null} size="sm" status={dmRecipient ? ($presenceMap.get(dmRecipient.id) ?? undefined) : undefined} />
				</span>
				{#if dm.encrypted}
					{@const unlocked = $unlockedChannels.has(dm.id)}
					<svg class="h-3.5 w-3.5 shrink-0 {unlocked ? 'text-green-400' : 'text-red-400'}" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<title>{unlocked ? 'Encrypted (unlocked)' : 'Encrypted (locked)'}</title>
						{#if unlocked}
							<path stroke-linecap="round" stroke-linejoin="round" d="M13.5 10.5V6.75a4.5 4.5 0 119 0v3.75M3.75 21.75h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H3.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z" />
						{:else}
							<path stroke-linecap="round" stroke-linejoin="round" d="M16.5 10.5V6.75a4.5 4.5 0 10-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H6.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z" />
						{/if}
					</svg>
				{/if}
				<span class="flex-1 truncate">{dmName}</span>
				{#if dmMuted}
					<svg class="h-3.5 w-3.5 shrink-0 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<title>Muted</title>
						<path d="M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
						<path d="M17 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2" />
					</svg>
				{/if}
				{#if dmMentions > 0}
					<span class="ml-auto flex h-4 min-w-4 items-center justify-center rounded-full {dmMuted ? 'bg-text-muted/50' : 'bg-red-500'} px-1 text-2xs font-bold text-white" title="{dmMentions} mention{dmMentions !== 1 ? 's' : ''}">@{dmMentions > 99 ? '99+' : dmMentions}</span>
				{:else if dmUnread > 0}
					<span class="ml-auto flex h-4 min-w-4 items-center justify-center rounded-full {dmMuted ? 'bg-text-muted/30' : 'bg-text-muted'} px-1 text-2xs font-bold text-white">{dmUnread > 99 ? '99+' : dmUnread}</span>
				{/if}
			</button>
		{/each}
	{/if}
{/if}
