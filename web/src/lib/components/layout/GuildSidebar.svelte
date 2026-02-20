<script lang="ts">
	import { guildList, currentGuildId, setGuild, guilds, federatedGuilds, removeFederatedGuild } from '$lib/stores/guilds';
	import { unreadCounts, unreadState, guildUnreadSet, guildMentionCounts } from '$lib/stores/unreads';
	import { unreadNotificationCount } from '$lib/stores/notifications';
	import { pendingIncomingCount } from '$lib/stores/relationships';
	import { dmChannels } from '$lib/stores/dms';
	import { currentUser } from '$lib/stores/auth';
	import { isDndActive } from '$lib/stores/settings';
	import { incomingCallCount } from '$lib/stores/callRing';
	import Avatar from '$components/common/Avatar.svelte';
	import CreateGuildModal from '$components/guild/CreateGuildModal.svelte';
	import NotificationPopover from '$components/common/NotificationPopover.svelte';
	import { DragController } from '$lib/utils/dragDrop';
	import { addToast } from '$lib/stores/toast';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { onDestroy, untrack } from 'svelte';
	import ContextMenu from '$components/common/ContextMenu.svelte';
	import ContextMenuItem from '$components/common/ContextMenuItem.svelte';
	import ContextMenuDivider from '$components/common/ContextMenuDivider.svelte';
	import { isGuildMuted, muteGuild, unmuteGuild } from '$lib/stores/muting';
	import type { FederatedGuild } from '$lib/types';

	const federatedGuildList = $derived(Array.from($federatedGuilds.values()));
	import { channelGuildMap } from '$lib/stores/unreads';
	import InviteModal from '$components/guild/InviteModal.svelte';

	let showNotificationPopover = $state(false);
	let showInviteForGuild = $state<string | null>(null);

	const isAdmin = $derived(($currentUser?.flags ?? 0) & 4);
	const isGlobalMod = $derived(($currentUser?.flags ?? 0) & 32);

	// Badge count for the Home button: pending friend requests + unread DMs + incoming calls.
	const homeBadgeCount = $derived.by(() => {
		let dmUnreads = 0;
		for (const [channelId, n] of $unreadCounts) {
			if (n > 0 && $dmChannels.has(channelId)) dmUnreads += n;
		}
		return dmUnreads + $pendingIncomingCount + $incomingCallCount;
	});
	const hasIncomingCall = $derived($incomingCallCount > 0);

	let showCreateModal = $state(false);

	// --- Guild drag-reorder ---
	let guildListEl = $state<HTMLElement | null>(null);
	let guildDragController = $state<DragController | null>(null);

	$effect(() => {
		const el = guildListEl;
		if (!el) return;
		untrack(() => {
			guildDragController?.destroy();
			guildDragController = new DragController({
				container: el,
				items: () => $guildList.map(g => g.id),
				getElement: (id) => el.querySelector(`[data-guild-id="${id}"]`) as HTMLElement | null,
				canDrag: true,
				onDrop: handleGuildReorder,
			});
		});
	});

	onDestroy(() => { guildDragController?.destroy(); });

	let reorderingGuilds = false;

	async function handleGuildReorder(sourceId: string, targetIndex: number) {
		if (reorderingGuilds) return;
		reorderingGuilds = true;
		try {
			const list = $guildList;
			const sourceIdx = list.findIndex(g => g.id === sourceId);
			if (sourceIdx === -1) return;

			const prevOrder = [...list];
			const reordered = [...list];
			const [moved] = reordered.splice(sourceIdx, 1);
			reordered.splice(targetIndex, 0, moved);

			// Optimistic update — re-insert into Map in new order
			guilds.setAll(reordered.map(g => [g.id, g]));

			const positions = reordered.map((g, i) => ({ guild_id: g.id, position: i }));
			try {
				await api.reorderGuilds(positions);
			} catch (err: any) {
				guilds.setAll(prevOrder.map(g => [g.id, g]));
				addToast(err.message || 'Failed to reorder servers', 'error');
			}
		} finally {
			reorderingGuilds = false;
		}
	}

	function selectGuild(id: string) {
		goto(`/app/guilds/${id}`);
	}

	// --- Guild icon context menu ---
	let guildCtxMenu = $state<{ x: number; y: number; guildId: string; guildName: string; ownerId: string } | null>(null);
	let showGuildMuteSubmenu = $state(false);

	const muteDurations = [
		{ label: 'Mute for 15 Minutes', ms: 15 * 60 * 1000 },
		{ label: 'Mute for 1 Hour', ms: 60 * 60 * 1000 },
		{ label: 'Mute for 8 Hours', ms: 8 * 60 * 60 * 1000 },
		{ label: 'Mute for 24 Hours', ms: 24 * 60 * 60 * 1000 },
		{ label: 'Mute Until I Turn It Back On', ms: 0 }
	];

	function openGuildContextMenu(e: MouseEvent, guild: { id: string; name: string; owner_id: string }) {
		e.preventDefault();
		e.stopPropagation();
		guildCtxMenu = { x: e.clientX, y: e.clientY, guildId: guild.id, guildName: guild.name, ownerId: guild.owner_id };
		showGuildMuteSubmenu = false;
	}

	function closeGuildContextMenu() {
		guildCtxMenu = null;
		showGuildMuteSubmenu = false;
	}

	async function markGuildAsRead(guildId: string) {
		const guildChannelIds: string[] = [];
		for (const [channelId, gId] of $channelGuildMap) {
			if (gId === guildId) {
				const unread = $unreadCounts.get(channelId) ?? 0;
				const mentions = $unreadState.get(channelId)?.mentionCount ?? 0;
				if (unread > 0 || mentions > 0) {
					guildChannelIds.push(channelId);
				}
			}
		}
		// Clear unread counts and mention counts locally first
		for (const cid of guildChannelIds) {
			unreadCounts.removeEntry(cid);
			unreadState.updateEntry(cid, (entry) => ({ ...entry, mentionCount: 0 }));
		}
		// Ack on server (best-effort)
		for (const cid of guildChannelIds) {
			try { await api.ackChannel(cid); } catch {}
		}
	}

	async function handleLeaveGuild(guildId: string) {
		if (!confirm('Are you sure you want to leave this server?')) return;
		try {
			await api.leaveGuild(guildId);
			guilds.removeEntry(guildId);
			if ($currentGuildId === guildId) {
				goto('/app');
			}
			addToast('Left server', 'info');
		} catch (err: any) {
			addToast(err.message || 'Failed to leave server', 'error');
		}
		closeGuildContextMenu();
	}

	async function handleLeaveFederatedGuild(guildId: string) {
		if (!confirm('Are you sure you want to leave this federated server?')) return;
		try {
			await api.leaveFederatedGuild(guildId);
			removeFederatedGuild(guildId);
			if ($currentGuildId === guildId) {
				goto('/app');
			}
			addToast('Left federated server', 'info');
		} catch (err: any) {
			addToast(err.message || 'Failed to leave server', 'error');
		}
	}

	// Context menu for federated guilds
	let fedCtxMenu = $state<{ x: number; y: number; guildId: string; guildName: string; domain: string } | null>(null);

	function openFedContextMenu(e: MouseEvent, fg: FederatedGuild) {
		e.preventDefault();
		e.stopPropagation();
		fedCtxMenu = { x: e.clientX, y: e.clientY, guildId: fg.guild_id, guildName: fg.name, domain: fg.instance_domain };
	}

	function closeFedContextMenu() {
		fedCtxMenu = null;
	}
</script>

<nav class="flex h-full w-14 shrink-0 flex-col items-center gap-2 overflow-y-auto border-r border-[--border-primary] bg-bg-floating py-3" aria-label="Server list">
	<!-- Home / DMs button -->
	<button
		class="relative flex h-9 w-9 items-center justify-center rounded-md border border-bg-modifier bg-bg-tertiary text-text-primary transition-colors hover:bg-brand-500"
		class:!bg-brand-500={$currentGuildId === null}
		onclick={() => { setGuild(null); goto('/app'); }}
		title="Home"
	>
		<svg class="h-5 w-5" fill="currentColor" viewBox="0 0 24 24">
			<path d="M10 20v-6h4v6h5v-8h3L12 3 2 12h3v8z" />
		</svg>
		{#if homeBadgeCount > 0}
			<span class="absolute -right-0.5 -top-0.5 flex h-5 min-w-5 items-center justify-center rounded-full px-1 text-2xs font-bold text-white {hasIncomingCall ? 'animate-pulse bg-green-500' : 'bg-red-500'}">
				{hasIncomingCall ? '!' : homeBadgeCount > 99 ? '99+' : homeBadgeCount}
			</span>
		{/if}
	</button>

	<div class="mx-auto w-8 border-t border-bg-modifier"></div>

	<!-- Guild list -->
	<div bind:this={guildListEl} class="relative flex flex-col items-center gap-2">
		{#each $guildList as guild (guild.id)}
			<div
				class="group/drag"
				data-guild-id={guild.id}
				onpointerdown={(e) => guildDragController?.handlePointerDown(e, guild.id)}
			>
				<button
					class="group relative flex h-9 w-9 items-center justify-center rounded-md border border-bg-modifier bg-bg-tertiary transition-colors hover:bg-brand-500"
					class:!bg-brand-500={$currentGuildId === guild.id}
					onclick={() => selectGuild(guild.id)}
					oncontextmenu={(e) => openGuildContextMenu(e, guild)}
					title={guild.name}
				>
					{#if guild.icon_id}
						<img
							src="/api/v1/files/{guild.icon_id}"
							alt={guild.name}
							class="h-full w-full rounded-[inherit] object-cover"
						/>
					{:else}
						<span class="text-sm font-semibold text-text-primary">
							{guild.name.split(' ').map((w) => w[0]).join('').slice(0, 3)}
						</span>
					{/if}

					<!-- Unread pill indicator (Discord-like) -->
					{#if $currentGuildId === guild.id}
						<div class="absolute -left-[3px] top-1/2 h-10 w-1 -translate-y-1/2 rounded-r-full bg-text-primary transition-all"></div>
					{:else if $guildUnreadSet.has(guild.id)}
						<div class="absolute -left-[3px] top-1/2 h-2 w-1 -translate-y-1/2 rounded-r-full bg-text-primary transition-all"></div>
					{/if}
					<!-- Mention count badge (red) -->
					{#if ($guildMentionCounts.get(guild.id) ?? 0) > 0}
						{@const mc = $guildMentionCounts.get(guild.id) ?? 0}
						<span class="absolute -bottom-0.5 -right-0.5 flex h-5 min-w-5 items-center justify-center rounded-full bg-red-500 px-1 text-2xs font-bold text-white">
							{mc > 99 ? '99+' : mc}
						</span>
					{/if}
				</button>
			</div>
		{/each}
	</div>

	<!-- Federated guilds -->
	{#if federatedGuildList.length > 0}
		<div class="mx-auto w-8 border-t border-bg-modifier"></div>
		<div class="flex flex-col items-center gap-2">
			{#each federatedGuildList as fg (fg.guild_id)}
				<button
					class="group relative flex h-9 w-9 items-center justify-center rounded-md border border-blue-500/30 bg-bg-tertiary transition-colors hover:bg-blue-500"
					class:!bg-blue-500={$currentGuildId === fg.guild_id}
					onclick={() => selectGuild(fg.guild_id)}
					oncontextmenu={(e) => openFedContextMenu(e, fg)}
					title="{fg.name} ({fg.instance_domain})"
				>
					{#if fg.icon_id}
						<img
							src="https://{fg.instance_domain}/api/v1/files/{fg.icon_id}"
							alt={fg.name}
							class="h-full w-full rounded-[inherit] object-cover"
						/>
					{:else}
						<span class="text-sm font-semibold text-text-primary">
							{fg.name.split(' ').map((w) => w[0]).join('').slice(0, 3)}
						</span>
					{/if}
					<!-- Federation indicator dot -->
					<span class="absolute -bottom-0.5 -right-0.5 flex h-3 w-3 items-center justify-center rounded-full bg-bg-floating">
						<svg class="h-2 w-2 text-blue-400" viewBox="0 0 16 16" fill="currentColor">
							<path d="M8 0a8 8 0 100 16A8 8 0 008 0z"/>
						</svg>
					</span>
				</button>
			{/each}
		</div>
	{/if}

	<!-- Add guild button -->
	<button
		class="flex h-9 w-9 items-center justify-center rounded-md border border-bg-modifier bg-bg-tertiary text-green-500 transition-colors hover:bg-green-500 hover:text-white"
		onclick={() => (showCreateModal = true)}
		title="Create or Join a Server"
	>
		<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path d="M12 5v14m-7-7h14" />
		</svg>
	</button>

	<!-- Discover servers button -->
	<button
		class="flex h-9 w-9 items-center justify-center rounded-md border border-bg-modifier bg-bg-tertiary text-text-muted transition-colors hover:bg-green-600 hover:text-white"
		onclick={() => goto('/app/discover')}
		title="Discover Servers"
	>
		<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
		</svg>
	</button>

	<!-- Spacer to push bottom buttons down -->
	<div class="flex-1"></div>

	<!-- Saved messages button -->
	<button
		class="flex h-9 w-9 items-center justify-center rounded-md border border-bg-modifier bg-bg-tertiary text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-primary"
		class:!bg-bg-modifier={$page.url.pathname === '/app/bookmarks'}
		onclick={() => goto('/app/bookmarks')}
		title="Saved Messages"
	>
		<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z" />
		</svg>
	</button>

	<!-- Notifications bell button -->
	<button
		class="relative flex h-9 w-9 items-center justify-center rounded-md border border-bg-modifier bg-bg-tertiary text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-primary"
		onclick={() => (showNotificationPopover = !showNotificationPopover)}
		title={$isDndActive ? 'Notifications (Do Not Disturb active)' : 'Notifications'}
	>
		{#if $isDndActive}
			<!-- DND: show a crossed-out bell -->
			<svg class="h-5 w-5 text-status-dnd" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M14.857 17.082a23.848 23.848 0 005.454-1.31A8.967 8.967 0 0118 9.75v-.7V9A6 6 0 006 9v.75a8.967 8.967 0 01-2.312 6.022c1.733.64 3.56 1.085 5.455 1.31m5.714 0a24.255 24.255 0 01-5.714 0m5.714 0a3 3 0 11-5.714 0" />
				<line x1="3" y1="3" x2="21" y2="21" stroke-width="2" />
			</svg>
			<!-- DND indicator dot -->
			<span class="absolute -right-0.5 -top-0.5 flex h-4 w-4 items-center justify-center rounded-full bg-status-dnd">
				<span class="h-1.5 w-1.5 rounded-sm bg-white"></span>
			</span>
		{:else}
			<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M14.857 17.082a23.848 23.848 0 005.454-1.31A8.967 8.967 0 0118 9.75v-.7V9A6 6 0 006 9v.75a8.967 8.967 0 01-2.312 6.022c1.733.64 3.56 1.085 5.455 1.31m5.714 0a24.255 24.255 0 01-5.714 0m5.714 0a3 3 0 11-5.714 0" />
			</svg>
			{#if $unreadNotificationCount > 0}
				<span class="absolute -right-0.5 -top-0.5 flex h-5 min-w-5 items-center justify-center rounded-full bg-red-500 px-1 text-2xs font-bold text-white">
					{$unreadNotificationCount > 99 ? '99+' : $unreadNotificationCount}
				</span>
			{/if}
		{/if}
	</button>

	<NotificationPopover bind:open={showNotificationPopover} />

	<!-- Admin button (only visible to admins) -->
	{#if isAdmin}
		<button
			class="flex h-9 w-9 items-center justify-center rounded-md border border-bg-modifier bg-bg-tertiary text-yellow-500 transition-colors hover:bg-yellow-500 hover:text-white"
			class:!bg-yellow-500={$page.url.pathname.startsWith('/app/admin')}
			class:!text-white={$page.url.pathname.startsWith('/app/admin')}
			onclick={() => goto('/app/admin')}
			title="Admin Panel"
		>
			<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
			</svg>
		</button>
	{/if}

	<!-- Moderation button (visible to global mods and admins) -->
	{#if isGlobalMod || isAdmin}
		<button
			class="flex h-9 w-9 items-center justify-center rounded-md border border-bg-modifier bg-bg-tertiary text-orange-500 transition-colors hover:bg-orange-500 hover:text-white"
			class:!bg-orange-500={$page.url.pathname.startsWith('/app/moderation')}
			class:!text-white={$page.url.pathname.startsWith('/app/moderation')}
			onclick={() => goto('/app/moderation')}
			title="Moderation Panel"
		>
			<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
				<path d="M12 8v4m0 4h.01" />
			</svg>
		</button>
	{/if}

	<!-- Settings button -->
	<button
		class="flex h-9 w-9 items-center justify-center rounded-md border border-bg-modifier bg-bg-tertiary text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-primary"
		class:!bg-bg-modifier={$page.url.pathname.startsWith('/app/settings') || $page.url.pathname === '/settings'}
		onclick={() => goto('/app/settings')}
		title="User Settings"
	>
		<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
			<circle cx="12" cy="12" r="3" />
		</svg>
	</button>
</nav>

<svelte:window
	onclick={() => { closeGuildContextMenu(); closeFedContextMenu(); }}
	onpointermove={(e) => guildDragController?.handlePointerMove(e)}
	onpointerup={(e) => guildDragController?.handlePointerUp(e)}
	onpointercancel={(e) => guildDragController?.handlePointerCancel(e)}
	onkeydown={(e) => guildDragController?.handleKeyDown(e)}
/>

<!-- Guild icon context menu -->
{#if guildCtxMenu}
	<ContextMenu x={guildCtxMenu.x} y={guildCtxMenu.y} onclose={closeGuildContextMenu}>
		<!-- Mark as Read -->
		{#if $guildUnreadSet.has(guildCtxMenu.guildId) || ($guildMentionCounts.get(guildCtxMenu.guildId) ?? 0) > 0}
			<ContextMenuItem label="Mark as Read" onclick={() => { markGuildAsRead(guildCtxMenu!.guildId); closeGuildContextMenu(); }} />
		{/if}
		<!-- Mute / Unmute -->
		{#if isGuildMuted(guildCtxMenu.guildId)}
			<ContextMenuItem label="Unmute Server" onclick={() => { unmuteGuild(guildCtxMenu!.guildId); closeGuildContextMenu(); }} />
		{:else}
			{#each muteDurations as dur}
				<ContextMenuItem label={dur.label} onclick={() => { muteGuild(guildCtxMenu!.guildId, dur.ms || undefined); closeGuildContextMenu(); }} />
			{/each}
		{/if}
		<ContextMenuDivider />
		<!-- Invite People (navigate to guild first so InviteModal has the right context) -->
		<ContextMenuItem label="Invite People" onclick={() => { const gid = guildCtxMenu!.guildId; closeGuildContextMenu(); selectGuild(gid); showInviteForGuild = gid; }} />
		<!-- Guild Settings (owner only from sidebar context) -->
		{#if guildCtxMenu.ownerId === $currentUser?.id}
			<ContextMenuItem label="Server Settings" onclick={() => { goto(`/app/guilds/${guildCtxMenu!.guildId}/settings`); closeGuildContextMenu(); }} />
		{/if}
		<ContextMenuDivider />
		<!-- Leave Guild -->
		{#if guildCtxMenu.ownerId !== $currentUser?.id}
			<ContextMenuItem label="Leave Server" danger onclick={() => handleLeaveGuild(guildCtxMenu!.guildId)} />
		{/if}
	</ContextMenu>
{/if}

<!-- Invite Modal (triggered from guild context menu) -->
{#if showInviteForGuild}
	<InviteModal open={true} guildId={showInviteForGuild} onclose={() => (showInviteForGuild = null)} />
{/if}

<!-- Federated guild context menu -->
{#if fedCtxMenu}
	<ContextMenu x={fedCtxMenu.x} y={fedCtxMenu.y} onclose={closeFedContextMenu}>
		<ContextMenuItem label="{fedCtxMenu.domain}" disabled />
		<ContextMenuDivider />
		<ContextMenuItem label="Leave Server" danger onclick={() => { handleLeaveFederatedGuild(fedCtxMenu!.guildId); closeFedContextMenu(); }} />
	</ContextMenu>
{/if}

<CreateGuildModal bind:open={showCreateModal} onclose={() => (showCreateModal = false)} />
