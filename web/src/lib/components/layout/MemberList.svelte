<script lang="ts">
	import type { GuildMember, Role } from '$lib/types';
	import { memberListWidth } from '$lib/stores/layout';
	import { currentGuildId, currentGuild } from '$lib/stores/guilds';
	import { currentUser } from '$lib/stores/auth';
	import { api } from '$lib/api/client';
	import Avatar from '$components/common/Avatar.svelte';
	import ContextMenu from '$components/common/ContextMenu.svelte';
	import ContextMenuItem from '$components/common/ContextMenuItem.svelte';
	import ContextMenuDivider from '$components/common/ContextMenuDivider.svelte';
	import UserPopover from '$components/common/UserPopover.svelte';
	import ProfileModal from '$components/common/ProfileModal.svelte';
	import FederationBadge from '$components/common/FederationBadge.svelte';
	import { presenceMap } from '$lib/stores/presence';
	import { addDMChannel } from '$lib/stores/dms';
	import { relationships, addOrUpdateRelationship } from '$lib/stores/relationships';
	import { addToast } from '$lib/stores/toast';
	import { guildMembers, setGuildMembers, setGuildRoles, memberTimeouts } from '$lib/stores/members';
	import { getMemberRoleColor } from '$lib/utils/roleColor';
	import { canKickMembers, canBanMembers, canTimeoutMembers, canAssignRoles } from '$lib/stores/permissions';
	import { kickModalTarget, banModalTarget } from '$lib/stores/moderation';
	import { clientNicknames } from '$lib/stores/nicknames';
	import { blockedUsers } from '$lib/stores/blocked';
	import { goto } from '$app/navigation';
	import { createAsyncOp } from '$lib/utils/asyncOp';
	import { avatarUrl } from '$lib/utils/avatar';

	// Members are derived from the guildMembers store so real-time updates
	// (e.g. avatar changes via USER_UPDATE) are reflected immediately.
	// Filter out users with 'block' level — 'ignore' level users still appear.
	const members = $derived(
		Array.from($guildMembers.values()).filter(m => $blockedUsers.get(m.user_id) !== 'block')
	);
	let visible = $state(true);
	let loadingGuildId = $state<string | null>(null);

	// Guild roles for hoist grouping
	let allGuildRoles = $state<Role[]>([]);

	// Context menu state
	let contextMenu = $state<{ x: number; y: number; member: GuildMember } | null>(null);
	// User popover state
	let popover = $state<{ userId: string; x: number; y: number } | null>(null);
	let profileUserId = $state<string | null>(null);
	// Submenu state
	let showRolesSubmenu = $state(false);
	let showTimeoutSubmenu = $state(false);
	let customTimeoutMinutes = $state('');
	// Role data for the submenu
	let guildRoles = $state<Role[]>([]);
	let memberRoleIds = $state<Set<string>>(new Set());
	let rolesOp = $state(createAsyncOp());

	$effect(() => {
		const guildId = $currentGuildId;
		if (guildId) {
			loadingGuildId = guildId;
			Promise.all([
				api.getMembers(guildId),
				api.getRoles(guildId),
			]).then(([m, r]) => {
				if (loadingGuildId === guildId) {
					setGuildMembers(m);
					allGuildRoles = r;
					setGuildRoles(r);
				}
			}).catch(() => {});
		} else {
			loadingGuildId = null;
			setGuildMembers([]);
			allGuildRoles = [];
		}
	});

	// Build a map from role ID → Role for quick lookups
	const roleMap = $derived(new Map(allGuildRoles.map((r) => [r.id, r])));

	// Hoisted roles sorted by position descending (highest first)
	const hoistedRoles = $derived(
		allGuildRoles
			.filter((r) => r.hoist && r.name !== '@everyone')
			.sort((a, b) => b.position - a.position)
	);

	// Compute grouped member lists: hoisted groups, then generic online, then offline
	const memberGroups = $derived.by(() => {
		const placed = new Set<string>();
		const groups: { role: Role | null; label: string; color: string | null; members: GuildMember[] }[] = [];

		// Online member IDs
		const onlineIds = new Set(
			members
				.filter((m) => {
					const status = $presenceMap.get(m.user_id);
					return status && status !== 'offline';
				})
				.map((m) => m.user_id)
		);

		// For each hoisted role (highest position first), collect online members
		for (const role of hoistedRoles) {
			const groupMembers: GuildMember[] = [];
			for (const m of members) {
				if (placed.has(m.user_id)) continue;
				if (!onlineIds.has(m.user_id)) continue;
				if (m.roles && m.roles.includes(role.id)) {
					groupMembers.push(m);
					placed.add(m.user_id);
				}
			}
			if (groupMembers.length > 0) {
				groups.push({ role, label: role.name, color: role.color, members: groupMembers });
			}
		}

		// Remaining online members (not placed in any hoisted group)
		const remainingOnline = members.filter(
			(m) => onlineIds.has(m.user_id) && !placed.has(m.user_id)
		);
		if (remainingOnline.length > 0) {
			groups.push({ role: null, label: 'Online', color: null, members: remainingOnline });
		}

		// All offline members (regardless of hoist)
		const offlineList = members.filter((m) => !onlineIds.has(m.user_id));
		if (offlineList.length > 0) {
			groups.push({ role: null, label: 'Offline', color: null, members: offlineList });
		}

		return groups;
	});

	// Current user's highest role position (for hierarchy filtering in roles submenu)
	const myHighestPos = $derived.by(() => {
		const me = members.find((m) => m.user_id === $currentUser?.id);
		if (!me?.roles) return 0;
		let max = 0;
		for (const rid of me.roles) {
			const r = roleMap.get(rid);
			if (r && r.position > max) max = r.position;
		}
		return max;
	});

	const isOwner = $derived($currentUser?.id === $currentGuild?.owner_id);

	const contextMemberRel = $derived(contextMenu ? $relationships.get(contextMenu.member.user_id) : undefined);
	const isContextSelf = $derived(contextMenu?.member.user_id === $currentUser?.id);
	const isContextOwner = $derived(contextMenu?.member.user_id === $currentGuild?.owner_id);
	const canModerate = $derived(!isContextSelf && !isContextOwner);
	const hasAnyModPerm = $derived($canKickMembers || $canBanMembers || $canTimeoutMembers || $canAssignRoles);

	const timeoutPresets = [
		{ label: '1 minute', seconds: 60 },
		{ label: '5 minutes', seconds: 300 },
		{ label: '10 minutes', seconds: 600 },
		{ label: '15 minutes', seconds: 900 },
		{ label: '30 minutes', seconds: 1800 },
		{ label: '1 hour', seconds: 3600 },
	];

	function openContextMenu(e: MouseEvent, member: GuildMember) {
		e.preventDefault();
		showRolesSubmenu = false;
		showTimeoutSubmenu = false;
		customTimeoutMinutes = '';
		contextMenu = { x: e.clientX, y: e.clientY, member };
	}

	function closeContextMenu() {
		contextMenu = null;
		showRolesSubmenu = false;
		showTimeoutSubmenu = false;
	}

	function openProfile(member: GuildMember) {
		if (!contextMenu) return;
		popover = { userId: member.user_id, x: contextMenu.x, y: contextMenu.y };
		closeContextMenu();
	}

	async function startDM(member: GuildMember) {
		try {
			// For federated users, ensure a local user stub exists first so createDM works.
			if (member.user?.instance_domain) {
				await api.ensureFederatedUser({
					user_id: member.user_id,
					instance_domain: member.user.instance_domain,
					username: member.user.username,
					display_name: member.user.display_name,
					avatar_id: member.user.avatar_id,
				});
			}
			const channel = await api.createDM(member.user_id);
			addDMChannel(channel);
			closeContextMenu();
			goto(`/app/dms/${channel.id}`);
		} catch (err: any) {
			addToast(err.message || 'Failed to create DM', 'error');
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

	function openKickModal(member: GuildMember) {
		const guildId = $currentGuildId;
		if (!guildId) return;
		$kickModalTarget = { userId: member.user_id, guildId, displayName: getMemberName(member) };
		closeContextMenu();
	}

	function openBanModal(member: GuildMember) {
		const guildId = $currentGuildId;
		if (!guildId) return;
		$banModalTarget = { userId: member.user_id, guildId, displayName: getMemberName(member) };
		closeContextMenu();
	}

	async function toggleRolesSubmenu() {
		showTimeoutSubmenu = false;
		showRolesSubmenu = !showRolesSubmenu;
		if (showRolesSubmenu && contextMenu) {
			const guildId = $currentGuildId;
			if (!guildId) return;
			const member = contextMenu.member;
			const result = await rolesOp.run(
				() => Promise.all([
					api.getRoles(guildId),
					api.getMemberRoles(guildId, member.user_id),
				]),
				msg => addToast(msg, 'error')
			);
			if (result) {
				const [roles, mRoles] = result;
				// Filter out @everyone and roles at or above the actor's highest position (unless owner)
				guildRoles = roles.filter((r) => {
					if (r.name === '@everyone') return false;
					if (!isOwner && r.position >= myHighestPos) return false;
					return true;
				});
				memberRoleIds = new Set(mRoles.map((r) => r.id));
			}
		}
	}

	async function toggleRole(roleId: string) {
		const guildId = $currentGuildId;
		const member = contextMenu?.member;
		if (!guildId || !member) return;
		try {
			if (memberRoleIds.has(roleId)) {
				await api.removeRole(guildId, member.user_id, roleId);
				memberRoleIds.delete(roleId);
				memberRoleIds = new Set(memberRoleIds);
			} else {
				await api.assignRole(guildId, member.user_id, roleId);
				memberRoleIds.add(roleId);
				memberRoleIds = new Set(memberRoleIds);
			}
		} catch (err: any) {
			addToast(err.message || 'Failed to update role', 'error');
		}
	}

	function toggleTimeoutSubmenu() {
		showRolesSubmenu = false;
		showTimeoutSubmenu = !showTimeoutSubmenu;
		customTimeoutMinutes = '';
	}

	async function applyTimeout(seconds: number) {
		const guildId = $currentGuildId;
		const member = contextMenu?.member;
		if (!guildId || !member) return;
		const until = new Date(Date.now() + seconds * 1000).toISOString();
		try {
			await api.updateMember(guildId, member.user_id, { timeout_until: until });
			addToast(`Timed out ${getMemberName(member)} for ${formatDuration(seconds)}`, 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to timeout member', 'error');
		}
		closeContextMenu();
	}

	function applyCustomTimeout() {
		const mins = parseInt(customTimeoutMinutes, 10);
		if (mins > 0) applyTimeout(mins * 60);
	}

	function formatDuration(seconds: number): string {
		if (seconds < 60) return `${seconds}s`;
		if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
		return `${Math.floor(seconds / 3600)}h`;
	}

	// Report modal
	let showReportUserModal = $state(false);
	let reportUserTarget = $state<GuildMember | null>(null);
	let reportUserReason = $state('');
	let reportUserOp = $state(createAsyncOp());

	function openReportUser(member: GuildMember) {
		reportUserTarget = member;
		reportUserReason = '';
		showReportUserModal = true;
		closeContextMenu();
	}

	async function submitReportUser() {
		if (!reportUserTarget || !reportUserReason.trim()) return;
		await reportUserOp.run(
			() => api.reportUser(reportUserTarget!.user_id, reportUserReason.trim(), $currentGuildId ?? undefined),
			msg => addToast(msg, 'error')
		);
		if (!reportUserOp.error) {
			addToast('User reported to moderators', 'success');
			showReportUserModal = false;
		}
	}

	function getMemberName(member: GuildMember): string {
		return $clientNicknames.get(member.user_id) ?? member.nickname ?? member.user?.display_name ?? member.user?.username ?? '?';
	}

	function hasClientNickname(member: GuildMember): boolean {
		return $clientNicknames.has(member.user_id);
	}

	function isMemberTimedOut(member: GuildMember): boolean {
		if (!member.timeout_until) return false;
		return new Date(member.timeout_until).getTime() > Date.now();
	}
</script>

{#if visible && $currentGuildId}
	<aside class="shrink-0 overflow-y-auto bg-bg-secondary" style="width: {$memberListWidth}px;">
		<div class="p-3">
			{#each memberGroups as group}
				{@const isOffline = group.label === 'Offline'}
				<h3 class="mb-1 {group !== memberGroups[0] ? 'mt-4' : ''} px-1 text-2xs font-bold uppercase tracking-wide text-text-muted">
					{#if group.color}
						<span style="color: {group.color}">{group.label}</span>
					{:else}
						{group.label}
					{/if}
					 — {group.members.length}
				</h3>
				{#each group.members as member (member.user_id)}
					{@const memberColor = getMemberRoleColor(member.roles, roleMap)}
					<button
						class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left {isOffline ? 'opacity-50 hover:bg-bg-modifier hover:opacity-75' : 'hover:bg-bg-modifier'}"
						onclick={(e) => { popover = { userId: member.user_id, x: e.clientX, y: e.clientY }; }}
						oncontextmenu={(e) => openContextMenu(e, member)}
					>
						<Avatar
							name={getMemberName(member)}
							src={avatarUrl(member.user?.avatar_id, member.user?.instance_id || undefined)}
							size="sm"
							status={isOffline ? 'offline' : ($presenceMap.get(member.user_id) ?? 'online')}
						/>
						<span class="flex items-center gap-1 truncate text-sm text-text-secondary {hasClientNickname(member) ? 'italic' : ''}" style={memberColor ? `color: ${memberColor}` : ''} title={hasClientNickname(member) ? `Nickname for ${member.user?.display_name ?? member.user?.username ?? member.user_id}` : ''}>
							{getMemberName(member)}
							{#if member.user?.instance_domain || (member.user?.instance_id && $currentUser && member.user.instance_id !== $currentUser.instance_id)}
								<FederationBadge domain={member.user.instance_domain ?? member.user.instance_id} compact />
							{/if}
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
			{/each}
		</div>
	</aside>
{/if}

<!-- Member context menu -->
{#if contextMenu}
	<ContextMenu x={contextMenu.x} y={contextMenu.y} onclose={closeContextMenu}>
		<ContextMenuItem label="View Profile" onclick={() => openProfile(contextMenu!.member)} />
		<ContextMenuItem label="Copy ID" onclick={() => { navigator.clipboard.writeText(contextMenu!.member.user_id); closeContextMenu(); }} />
		{#if !isContextSelf}
			<ContextMenuItem label="Message" onclick={() => startDM(contextMenu!.member)} />
			{#if !contextMemberRel || contextMemberRel.type === 'pending_incoming'}
				<ContextMenuItem
					label={contextMemberRel?.type === 'pending_incoming' ? 'Accept Request' : 'Add Friend'}
					onclick={() => addFriend(contextMenu!.member)}
				/>
			{:else if contextMemberRel.type === 'pending_outgoing'}
				<ContextMenuItem label="Request Sent" disabled />
			{/if}
		{/if}

		<!-- Moderation actions (permission-gated) -->
		{#if canModerate && hasAnyModPerm}
			<ContextMenuDivider />

			<!-- Roles submenu -->
			{#if $canAssignRoles}
				<div class="relative">
					<button
						class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-primary hover:bg-brand-500 hover:text-white"
						onclick={(e) => { e.stopPropagation(); toggleRolesSubmenu(); }}
					>
						Roles
						<svg class="ml-auto h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M9 5l7 7-7 7" />
						</svg>
					</button>
					{#if showRolesSubmenu}
						{@const submenuLeft = contextMenu.x + 360 < window.innerWidth}
						<!-- svelte-ignore a11y_no_static_element_interactions -->
						<div
							class="absolute top-0 max-h-[50vh] min-w-[160px] overflow-y-auto rounded-md bg-bg-floating p-1 shadow-lg {submenuLeft ? 'left-full ml-1' : 'right-full mr-1'}"
							onclick={(e) => e.stopPropagation()}
							onkeydown={() => {}}
						>
							{#if rolesOp.loading}
								<div class="px-2 py-1.5 text-sm text-text-muted">Loading...</div>
							{:else if guildRoles.length === 0}
								<div class="px-2 py-1.5 text-sm text-text-muted">No roles</div>
							{:else}
								{#each guildRoles as role (role.id)}
									<button
										class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-primary hover:bg-brand-500 hover:text-white"
										onclick={() => toggleRole(role.id)}
									>
										<span class="flex h-4 w-4 shrink-0 items-center justify-center rounded border {memberRoleIds.has(role.id) ? 'border-brand-500 bg-brand-500' : 'border-text-muted'}">
											{#if memberRoleIds.has(role.id)}
												<svg class="h-3 w-3 text-white" fill="none" stroke="currentColor" stroke-width="3" viewBox="0 0 24 24">
													<path d="M5 13l4 4L19 7" />
												</svg>
											{/if}
										</span>
										{#if role.color}
											<span class="h-3 w-3 shrink-0 rounded-full" style="background-color: {role.color}"></span>
										{/if}
										<span class="truncate">{role.name}</span>
									</button>
								{/each}
							{/if}
						</div>
					{/if}
				</div>
			{/if}

			<!-- Timeout submenu -->
			{#if $canTimeoutMembers}
				<div class="relative">
					<button
						class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm text-text-primary hover:bg-brand-500 hover:text-white"
						onclick={(e) => { e.stopPropagation(); toggleTimeoutSubmenu(); }}
					>
						Timeout
						<svg class="ml-auto h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M9 5l7 7-7 7" />
						</svg>
					</button>
					{#if showTimeoutSubmenu}
						{@const submenuLeft = contextMenu.x + 360 < window.innerWidth}
						<!-- svelte-ignore a11y_no_static_element_interactions -->
						<div
							class="absolute top-0 min-w-[160px] rounded-md bg-bg-floating p-1 shadow-lg {submenuLeft ? 'left-full ml-1' : 'right-full mr-1'}"
							onclick={(e) => e.stopPropagation()}
							onkeydown={() => {}}
						>
							{#each timeoutPresets as preset}
								<button
									class="flex w-full items-center rounded px-2 py-1.5 text-sm text-text-primary hover:bg-brand-500 hover:text-white"
									onclick={() => applyTimeout(preset.seconds)}
								>{preset.label}</button>
							{/each}
							<div class="my-1 border-t border-bg-modifier"></div>
							<div class="flex items-center gap-1 px-2 py-1">
								<input
									type="number"
									min="1"
									class="w-16 rounded border border-bg-modifier bg-bg-primary px-1.5 py-1 text-sm text-text-primary focus:border-brand-500 focus:outline-none"
									placeholder="min"
									bind:value={customTimeoutMinutes}
									onclick={(e) => e.stopPropagation()}
									onkeydown={(e) => { if (e.key === 'Enter') applyCustomTimeout(); e.stopPropagation(); }}
								/>
								<button
									class="rounded bg-brand-500 px-2 py-1 text-xs font-medium text-white hover:bg-brand-600 disabled:opacity-50"
									disabled={!customTimeoutMinutes || parseInt(customTimeoutMinutes, 10) <= 0}
									onclick={applyCustomTimeout}
								>Apply</button>
							</div>
						</div>
					{/if}
				</div>
			{/if}

			<!-- Destructive moderation actions -->
			{#if $canKickMembers || $canBanMembers}
				<ContextMenuDivider />
				{#if $canKickMembers}
					<ContextMenuItem label="Kick" danger onclick={() => openKickModal(contextMenu!.member)} />
				{/if}
				{#if $canBanMembers}
					<ContextMenuItem label="Ban" danger onclick={() => openBanModal(contextMenu!.member)} />
				{/if}
			{/if}
		{/if}

		<!-- Report (always visible for non-self) -->
		{#if !isContextSelf}
			<ContextMenuDivider />
			<ContextMenuItem label="Report User" danger onclick={() => openReportUser(contextMenu!.member)} />
		{/if}
	</ContextMenu>
{/if}

<!-- Report user modal -->
{#if showReportUserModal && reportUserTarget}
	<div class="fixed inset-0 z-[100] flex items-center justify-center bg-black/50" onclick={() => showReportUserModal = false} onkeydown={(e) => e.key === 'Escape' && (showReportUserModal = false)} role="dialog" tabindex="-1">
		<div class="mx-4 w-full max-w-sm rounded-lg bg-bg-secondary p-4 shadow-xl md:mx-0" onclick={(e) => e.stopPropagation()} onkeydown={() => {}} role="document" tabindex="-1">
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
					disabled={!reportUserReason.trim() || reportUserOp.loading}
					onclick={submitReportUser}
				>{reportUserOp.loading ? 'Submitting...' : 'Report'}</button>
			</div>
		</div>
	</div>
{/if}

{#if popover}
	<UserPopover
		userId={popover.userId}
		x={popover.x}
		y={popover.y}
		onclose={() => (popover = null)}
		onviewprofile={(uid) => { popover = null; profileUserId = uid; }}
	/>
{/if}

{#if profileUserId}
	<ProfileModal userId={profileUserId} open={!!profileUserId} onclose={() => (profileUserId = null)} />
{/if}
