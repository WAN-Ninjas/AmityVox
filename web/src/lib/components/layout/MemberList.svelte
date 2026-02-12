<script lang="ts">
	import type { GuildMember } from '$lib/types';
	import { currentGuildId, currentGuild } from '$lib/stores/guilds';
	import { currentUser } from '$lib/stores/auth';
	import { api } from '$lib/api/client';
	import Avatar from '$components/common/Avatar.svelte';
	import { presenceMap } from '$lib/stores/presence';

	let members = $state<GuildMember[]>([]);
	let visible = $state(true);
	let loadingGuildId = $state<string | null>(null);

	// Context menu state
	let contextMenu = $state<{ x: number; y: number; member: GuildMember } | null>(null);

	$effect(() => {
		const guildId = $currentGuildId;
		if (guildId) {
			// Track which guild we're loading to discard stale responses.
			loadingGuildId = guildId;
			api.getMembers(guildId).then((m) => {
				if (loadingGuildId === guildId) members = m;
			}).catch(() => {});
		} else {
			loadingGuildId = null;
			members = [];
		}
	});

	const onlineMembers = $derived(
		members.filter((m) => {
			const status = $presenceMap.get(m.user_id);
			return status && status !== 'offline';
		})
	);

	const offlineMembers = $derived(
		members.filter((m) => {
			const status = $presenceMap.get(m.user_id);
			return !status || status === 'offline';
		})
	);

	const isOwner = $derived($currentGuild?.owner_id === $currentUser?.id);

	function openContextMenu(e: MouseEvent, member: GuildMember) {
		e.preventDefault();
		// Don't show actions for self
		if (member.user_id === $currentUser?.id) return;
		contextMenu = { x: e.clientX, y: e.clientY, member };
	}

	function closeContextMenu() {
		contextMenu = null;
	}

	async function kickMember(member: GuildMember) {
		const guildId = $currentGuildId;
		if (!guildId || !confirm(`Kick ${member.nickname ?? member.user?.username ?? 'this user'}?`)) return;
		try {
			await api.kickMember(guildId, member.user_id);
			members = members.filter((m) => m.user_id !== member.user_id);
		} catch (err: any) {
			alert(err.message || 'Failed to kick member');
		}
		closeContextMenu();
	}

	async function banMember(member: GuildMember) {
		const guildId = $currentGuildId;
		if (!guildId || !confirm(`Ban ${member.nickname ?? member.user?.username ?? 'this user'}? They will not be able to rejoin.`)) return;
		try {
			await api.banUser(guildId, member.user_id);
			members = members.filter((m) => m.user_id !== member.user_id);
		} catch (err: any) {
			alert(err.message || 'Failed to ban member');
		}
		closeContextMenu();
	}

	async function startDM(member: GuildMember) {
		try {
			const channel = await api.createDM(member.user_id);
			// Navigate to DM - for now just close menu
			closeContextMenu();
		} catch (err: any) {
			alert(err.message || 'Failed to create DM');
			closeContextMenu();
		}
	}

	function getMemberName(member: GuildMember): string {
		return member.nickname ?? member.user?.display_name ?? member.user?.username ?? '?';
	}
</script>

<svelte:window onclick={closeContextMenu} />

{#if visible && $currentGuildId}
	<aside class="hidden w-60 shrink-0 overflow-y-auto bg-bg-secondary lg:block">
		<div class="p-3">
			{#if onlineMembers.length > 0}
				<h3 class="mb-1 px-1 text-2xs font-bold uppercase tracking-wide text-text-muted">
					Online — {onlineMembers.length}
				</h3>
				{#each onlineMembers as member (member.user_id)}
					<button
						class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left hover:bg-bg-modifier"
						oncontextmenu={(e) => openContextMenu(e, member)}
					>
						<Avatar
							name={getMemberName(member)}
							size="sm"
							status="online"
						/>
						<span class="truncate text-sm text-text-secondary">
							{getMemberName(member)}
						</span>
					</button>
				{/each}
			{/if}

			{#if offlineMembers.length > 0}
				<h3 class="mb-1 mt-4 px-1 text-2xs font-bold uppercase tracking-wide text-text-muted">
					Offline — {offlineMembers.length}
				</h3>
				{#each offlineMembers as member (member.user_id)}
					<button
						class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left opacity-50 hover:bg-bg-modifier hover:opacity-75"
						oncontextmenu={(e) => openContextMenu(e, member)}
					>
						<Avatar
							name={getMemberName(member)}
							size="sm"
						/>
						<span class="truncate text-sm text-text-secondary">
							{getMemberName(member)}
						</span>
					</button>
				{/each}
			{/if}
		</div>
	</aside>
{/if}

<!-- Member context menu -->
{#if contextMenu}
	<div
		class="fixed z-50 min-w-[160px] rounded-md bg-bg-floating p-1 shadow-lg"
		style="left: {contextMenu.x}px; top: {contextMenu.y}px;"
	>
		<button
			class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
			onclick={() => startDM(contextMenu!.member)}
		>
			Message
		</button>
		{#if isOwner}
			<div class="my-1 border-t border-bg-modifier"></div>
			<button
				class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-red-400 hover:bg-red-500 hover:text-white"
				onclick={() => kickMember(contextMenu!.member)}
			>
				Kick
			</button>
			<button
				class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-red-400 hover:bg-red-500 hover:text-white"
				onclick={() => banMember(contextMenu!.member)}
			>
				Ban
			</button>
		{/if}
	</div>
{/if}
