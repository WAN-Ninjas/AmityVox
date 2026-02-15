<script lang="ts">
	import type { GuildMember } from '$lib/types';
	import { currentGuildId, currentGuild } from '$lib/stores/guilds';
	import { currentUser } from '$lib/stores/auth';
	import { api } from '$lib/api/client';
	import Avatar from '$components/common/Avatar.svelte';
	import { presenceMap } from '$lib/stores/presence';
	import { addDMChannel } from '$lib/stores/dms';
	import { relationships, addOrUpdateRelationship } from '$lib/stores/relationships';
	import { addToast } from '$lib/stores/toast';
	import { setGuildMembers, memberTimeouts } from '$lib/stores/members';
	import { goto } from '$app/navigation';

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
				if (loadingGuildId === guildId) {
					members = m;
					setGuildMembers(m);
				}
			}).catch(() => {});
		} else {
			loadingGuildId = null;
			members = [];
			setGuildMembers([]);
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
	const contextMemberRel = $derived(contextMenu ? $relationships.get(contextMenu.member.user_id) : undefined);

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

	let showReportUserModal = $state(false);
	let reportUserTarget = $state<GuildMember | null>(null);
	let reportUserReason = $state('');
	let reportUserSubmitting = $state(false);

	function openReportUser(member: GuildMember) {
		reportUserTarget = member;
		reportUserReason = '';
		showReportUserModal = true;
		closeContextMenu();
	}

	async function submitReportUser() {
		if (!reportUserTarget || !reportUserReason.trim()) return;
		reportUserSubmitting = true;
		try {
			await api.reportUser(reportUserTarget.user_id, reportUserReason.trim(), $currentGuildId ?? undefined);
			addToast('User reported to moderators', 'success');
			showReportUserModal = false;
		} catch {
			addToast('Failed to report user', 'error');
		} finally {
			reportUserSubmitting = false;
		}
	}

	async function startDM(member: GuildMember) {
		try {
			const channel = await api.createDM(member.user_id);
			addDMChannel(channel);
			closeContextMenu();
			goto(`/app/dms/${channel.id}`);
		} catch (err: any) {
			alert(err.message || 'Failed to create DM');
			closeContextMenu();
		}
	}

	async function addFriend(member: GuildMember) {
		closeContextMenu();
		try {
			const rel = await api.addFriend(member.user_id);
			addOrUpdateRelationship(rel);
			addToast(rel.type === 'friend' ? 'Friend request accepted!' : 'Friend request sent!', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to send friend request', 'error');
		}
	}

	function getMemberName(member: GuildMember): string {
		return member.nickname ?? member.user?.display_name ?? member.user?.username ?? '?';
	}

	function isMemberTimedOut(member: GuildMember): boolean {
		if (!member.timeout_until) return false;
		return new Date(member.timeout_until).getTime() > Date.now();
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
							status={$presenceMap.get(member.user_id) ?? 'online'}
						/>
						<span class="flex items-center gap-1 truncate text-sm text-text-secondary">
							{getMemberName(member)}
							{#if isMemberTimedOut(member)}
								<svg class="h-3.5 w-3.5 shrink-0 text-yellow-500" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
									<title>Timed out</title>
									<circle cx="12" cy="12" r="10" />
									<path d="M12 6v6l4 2" />
								</svg>
								<span class="text-2xs text-yellow-500">(Timed out)</span>
							{/if}
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
							status="offline"
						/>
						<span class="flex items-center gap-1 truncate text-sm text-text-secondary">
							{getMemberName(member)}
							{#if isMemberTimedOut(member)}
								<svg class="h-3.5 w-3.5 shrink-0 text-yellow-500" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
									<title>Timed out</title>
									<circle cx="12" cy="12" r="10" />
									<path d="M12 6v6l4 2" />
								</svg>
								<span class="text-2xs text-yellow-500">(Timed out)</span>
							{/if}
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
		{#if !contextMemberRel || contextMemberRel.type === 'pending_incoming'}
			<button
				class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-secondary hover:bg-brand-500 hover:text-white"
				onclick={() => addFriend(contextMenu!.member)}
			>
				{contextMemberRel?.type === 'pending_incoming' ? 'Accept Request' : 'Add Friend'}
			</button>
		{:else if contextMemberRel.type === 'pending_outgoing'}
			<button
				class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-muted cursor-default"
				disabled
			>
				Request Sent
			</button>
		{/if}
		{#if contextMenu.member.user_id !== $currentUser?.id}
			<div class="my-1 border-t border-bg-modifier"></div>
			<button
				class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-red-400 hover:bg-red-500 hover:text-white"
				onclick={() => openReportUser(contextMenu!.member)}
			>
				Report User
			</button>
		{/if}
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

<!-- Report user modal -->
{#if showReportUserModal && reportUserTarget}
	<div class="fixed inset-0 z-[100] flex items-center justify-center bg-black/50" onclick={() => showReportUserModal = false} onkeydown={(e) => e.key === 'Escape' && (showReportUserModal = false)} role="dialog" tabindex="-1">
		<div class="w-96 rounded-lg bg-bg-secondary p-4 shadow-xl" onclick={(e) => e.stopPropagation()} onkeydown={() => {}} role="document" tabindex="-1">
			<h3 class="mb-3 text-lg font-semibold text-text-primary">Report User</h3>
			<p class="mb-2 text-sm text-text-muted">
				Report <strong class="text-text-primary">{reportUserTarget.nickname ?? reportUserTarget.user?.username ?? 'this user'}</strong> to instance moderators.
			</p>
			<textarea
				class="mb-3 w-full rounded-md border border-bg-modifier bg-bg-primary p-2 text-sm text-text-primary placeholder:text-text-muted focus:border-brand-500 focus:outline-none"
				placeholder="Why are you reporting this user?"
				rows="3"
				bind:value={reportUserReason}
			></textarea>
			<div class="flex justify-end gap-2">
				<button
					class="rounded-md px-3 py-1.5 text-sm text-text-muted hover:text-text-primary"
					onclick={() => showReportUserModal = false}
				>Cancel</button>
				<button
					class="rounded-md bg-red-500 px-3 py-1.5 text-sm font-medium text-white hover:bg-red-600 disabled:opacity-50"
					disabled={!reportUserReason.trim() || reportUserSubmitting}
					onclick={submitReportUser}
				>{reportUserSubmitting ? 'Submitting...' : 'Report'}</button>
			</div>
		</div>
	</div>
{/if}
