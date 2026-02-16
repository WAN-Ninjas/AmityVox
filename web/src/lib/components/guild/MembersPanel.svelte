<script lang="ts">
	import type { GuildMember, Role } from '$lib/types';
	import { api } from '$lib/api/client';
	import Avatar from '$components/common/Avatar.svelte';

	let {
		guildId,
		onError = (_msg: string) => {},
		onSuccess = (_msg: string) => {}
	}: {
		guildId: string;
		onError: (msg: string) => void;
		onSuccess: (msg: string) => void;
	} = $props();

	// --- State ---

	let members = $state<GuildMember[]>([]);
	let allRoles = $state<Role[]>([]);
	let memberRoles = $state<Map<string, string[]>>(new Map());
	let loading = $state(true);
	let searchQuery = $state('');
	let expandedMemberId = $state<string | null>(null);
	let loadingMemberRoles = $state(false);

	// Timeout state
	let timeoutDuration = $state('');
	let applyingTimeout = $state(false);

	// Kick state
	let kickConfirmId = $state<string | null>(null);

	// Role assignment
	let togglingRole = $state(false);

	// --- Filtering (exported for testing) ---

	export function filterMembers(members: GuildMember[], query: string): GuildMember[] {
		if (!query.trim()) return members;
		const q = query.toLowerCase();
		return members.filter((m) => {
			const username = m.user?.username?.toLowerCase() ?? '';
			const displayName = m.user?.display_name?.toLowerCase() ?? '';
			const nickname = m.nickname?.toLowerCase() ?? '';
			return username.includes(q) || displayName.includes(q) || nickname.includes(q);
		});
	}

	const filteredMembers = $derived(filterMembers(members, searchQuery));

	// --- Data loading ---

	async function loadData() {
		loading = true;
		try {
			const [m, r] = await Promise.all([
				api.getMembers(guildId),
				api.getRoles(guildId)
			]);
			members = m;
			allRoles = r;
		} catch (err: any) {
			onError(err.message || 'Failed to load members');
		} finally {
			loading = false;
		}
	}

	async function loadMemberRoles(memberId: string) {
		loadingMemberRoles = true;
		try {
			const roles = await api.getMemberRoles(guildId, memberId);
			memberRoles = new Map(memberRoles);
			memberRoles.set(memberId, roles.map((r: Role) => r.id));
		} catch {
			memberRoles = new Map(memberRoles);
			memberRoles.set(memberId, []);
		} finally {
			loadingMemberRoles = false;
		}
	}

	// --- Actions ---

	function toggleExpand(memberId: string) {
		if (expandedMemberId === memberId) {
			expandedMemberId = null;
			kickConfirmId = null;
			return;
		}
		expandedMemberId = memberId;
		kickConfirmId = null;
		timeoutDuration = '';
		if (!memberRoles.has(memberId)) {
			loadMemberRoles(memberId);
		}
	}

	async function handleToggleRole(memberId: string, roleId: string) {
		togglingRole = true;
		const currentRoles = memberRoles.get(memberId) ?? [];
		try {
			if (currentRoles.includes(roleId)) {
				await api.removeRole(guildId, memberId, roleId);
				memberRoles = new Map(memberRoles);
				memberRoles.set(memberId, currentRoles.filter((r) => r !== roleId));
			} else {
				await api.assignRole(guildId, memberId, roleId);
				memberRoles = new Map(memberRoles);
				memberRoles.set(memberId, [...currentRoles, roleId]);
			}
		} catch (err: any) {
			onError(err.message || 'Failed to update role');
		} finally {
			togglingRole = false;
		}
	}

	async function handleApplyTimeout(memberId: string) {
		if (!timeoutDuration) return;
		applyingTimeout = true;
		try {
			const until = new Date(Date.now() + parseInt(timeoutDuration) * 1000).toISOString();
			await api.updateMember(guildId, memberId, { timeout_until: until });
			members = members.map((m) =>
				m.user_id === memberId ? { ...m, timeout_until: until } : m
			);
			timeoutDuration = '';
			onSuccess('Timeout applied');
		} catch (err: any) {
			onError(err.message || 'Failed to apply timeout');
		} finally {
			applyingTimeout = false;
		}
	}

	async function handleClearTimeout(memberId: string) {
		try {
			await api.updateMember(guildId, memberId, { timeout_until: null });
			members = members.map((m) =>
				m.user_id === memberId ? { ...m, timeout_until: null } : m
			);
			onSuccess('Timeout cleared');
		} catch (err: any) {
			onError(err.message || 'Failed to clear timeout');
		}
	}

	async function handleKick(memberId: string) {
		try {
			await api.kickMember(guildId, memberId);
			members = members.filter((m) => m.user_id !== memberId);
			expandedMemberId = null;
			kickConfirmId = null;
			onSuccess('Member kicked');
		} catch (err: any) {
			onError(err.message || 'Failed to kick member');
		}
	}

	// --- Helpers ---

	function formatDate(iso: string): string {
		return new Date(iso).toLocaleDateString();
	}

	function isTimedOut(member: GuildMember): boolean {
		return !!member.timeout_until && new Date(member.timeout_until).getTime() > Date.now();
	}

	function getRoleById(roleId: string): Role | undefined {
		return allRoles.find((r) => r.id === roleId);
	}

	const timeoutOptions = [
		{ label: '60 seconds', value: '60' },
		{ label: '5 minutes', value: '300' },
		{ label: '10 minutes', value: '600' },
		{ label: '1 hour', value: '3600' },
		{ label: '1 day', value: '86400' },
		{ label: '1 week', value: '604800' },
	];

	// Load on mount
	$effect(() => {
		loadData();
	});
</script>

<div class="space-y-4">
	<!-- Search bar -->
	<div>
		<input
			type="text"
			class="input w-full"
			placeholder="Search members by username, display name, or nickname..."
			bind:value={searchQuery}
		/>
	</div>

	<!-- Member list -->
	{#if loading}
		<p class="text-sm text-text-muted">Loading members...</p>
	{:else if filteredMembers.length === 0}
		<p class="text-sm text-text-muted">
			{searchQuery ? 'No members match your search.' : 'No members found.'}
		</p>
	{:else}
		<p class="text-xs text-text-muted">{filteredMembers.length} member{filteredMembers.length !== 1 ? 's' : ''}</p>
		<div class="space-y-1">
			{#each filteredMembers as member (member.user_id)}
				{@const expanded = expandedMemberId === member.user_id}
				{@const mRoles = memberRoles.get(member.user_id) ?? []}
				<div class="rounded-lg bg-bg-secondary">
					<!-- Member row -->
					<button
						class="flex w-full items-center gap-3 p-3 text-left transition-colors hover:bg-bg-modifier {expanded ? 'rounded-t-lg' : 'rounded-lg'}"
						onclick={() => toggleExpand(member.user_id)}
					>
						<Avatar user={member.user} size="sm" />
						<div class="min-w-0 flex-1">
							<div class="flex items-center gap-2">
								<span class="truncate text-sm font-medium text-text-primary">
									{member.user?.display_name || member.user?.username || member.user_id.slice(0, 8)}
								</span>
								{#if member.nickname}
									<span class="truncate text-xs text-text-muted">({member.nickname})</span>
								{/if}
								{#if isTimedOut(member)}
									<span class="rounded bg-yellow-500/20 px-1.5 py-0.5 text-2xs text-yellow-400">Timed out</span>
								{/if}
							</div>
							<div class="flex items-center gap-2">
								<span class="text-xs text-text-muted">@{member.user?.username ?? '?'}</span>
								<span class="text-2xs text-text-muted">Joined {formatDate(member.joined_at)}</span>
							</div>
						</div>
						<!-- Role badges -->
						<div class="flex flex-wrap gap-1">
							{#each mRoles as roleId (roleId)}
								{@const role = getRoleById(roleId)}
								{#if role}
									<span
										class="rounded-full px-2 py-0.5 text-2xs font-medium"
										style="background-color: {role.color ?? '#99aab5'}20; color: {role.color ?? '#99aab5'}"
									>
										{role.name}
									</span>
								{/if}
							{/each}
						</div>
						<svg class="h-4 w-4 shrink-0 text-text-muted transition-transform {expanded ? 'rotate-180' : ''}" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M19 9l-7 7-7-7" />
						</svg>
					</button>

					<!-- Expanded management panel -->
					{#if expanded}
						<div class="space-y-4 border-t border-border-primary p-4">
							<!-- Role checkboxes -->
							<div>
								<h4 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Roles</h4>
								{#if loadingMemberRoles}
									<p class="text-xs text-text-muted">Loading roles...</p>
								{:else}
									<div class="grid grid-cols-2 gap-1.5 sm:grid-cols-3">
										{#each allRoles as role (role.id)}
											<label class="flex items-center gap-2 rounded px-2 py-1 text-sm text-text-secondary hover:bg-bg-modifier {togglingRole ? 'pointer-events-none opacity-50' : ''}">
												<input
													type="checkbox"
													checked={mRoles.includes(role.id)}
													onchange={() => handleToggleRole(member.user_id, role.id)}
													class="accent-brand-500"
													disabled={togglingRole}
												/>
												<div class="h-2 w-2 shrink-0 rounded-full" style="background-color: {role.color ?? '#99aab5'}"></div>
												<span class="truncate text-xs">{role.name}</span>
											</label>
										{/each}
									</div>
									{#if allRoles.length === 0}
										<p class="text-xs text-text-muted">No roles available.</p>
									{/if}
								{/if}
							</div>

							<!-- Timeout -->
							<div>
								<h4 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Timeout</h4>
								{#if isTimedOut(member)}
									<div class="flex items-center gap-2">
										<span class="text-xs text-yellow-400">
											Timed out until {new Date(member.timeout_until!).toLocaleString()}
										</span>
										<button class="text-xs text-brand-400 hover:text-brand-300" onclick={() => handleClearTimeout(member.user_id)}>
											Clear
										</button>
									</div>
								{:else}
									<div class="flex items-center gap-2">
										<select class="input text-sm" bind:value={timeoutDuration}>
											<option value="">Select duration...</option>
											{#each timeoutOptions as opt (opt.value)}
												<option value={opt.value}>{opt.label}</option>
											{/each}
										</select>
										<button
											class="btn-primary text-xs"
											onclick={() => handleApplyTimeout(member.user_id)}
											disabled={applyingTimeout || !timeoutDuration}
										>
											{applyingTimeout ? 'Applying...' : 'Apply'}
										</button>
									</div>
								{/if}
							</div>

							<!-- Kick -->
							<div>
								<h4 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Actions</h4>
								{#if kickConfirmId === member.user_id}
									<div class="flex items-center gap-2">
										<span class="text-xs text-red-400">Are you sure?</span>
										<button class="rounded bg-red-500 px-3 py-1 text-xs text-white hover:bg-red-600" onclick={() => handleKick(member.user_id)}>
											Confirm Kick
										</button>
										<button class="text-xs text-text-muted hover:text-text-secondary" onclick={() => (kickConfirmId = null)}>
											Cancel
										</button>
									</div>
								{:else}
									<button class="text-xs text-red-400 hover:text-red-300" onclick={() => (kickConfirmId = member.user_id)}>
										Kick Member
									</button>
								{/if}
							</div>
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>
