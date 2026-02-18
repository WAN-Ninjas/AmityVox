<script lang="ts">
	import { api } from '$lib/api/client';
	import type { Role, Channel } from '$lib/types';
	import { createAsyncOp } from '$lib/utils/asyncOp';

	let { guildId }: { guildId: string } = $props();

	interface LevelingConfig {
		guild_id: string;
		enabled: boolean;
		xp_per_message: number;
		xp_cooldown_seconds: number;
		level_up_channel_id: string | null;
		level_up_message: string;
		stack_roles: boolean;
	}

	interface LevelRole {
		id: string;
		guild_id: string;
		level: number;
		role_id: string;
	}

	interface MemberXP {
		guild_id: string;
		user_id: string;
		xp: number;
		level: number;
		messages_counted: number;
		username: string;
		display_name: string | null;
		avatar_id: string | null;
	}

	let loadOp = $state(createAsyncOp());
	let saveOp = $state(createAsyncOp());
	let error = $state('');
	let success = $state('');

	let config = $state<LevelingConfig>({
		guild_id: '',
		enabled: false,
		xp_per_message: 15,
		xp_cooldown_seconds: 60,
		level_up_channel_id: null,
		level_up_message: 'Congratulations {user}, you reached level {level}!',
		stack_roles: true
	});
	let levelRoles = $state<LevelRole[]>([]);
	let leaderboard = $state<MemberXP[]>([]);
	let roles = $state<Role[]>([]);
	let channels = $state<Channel[]>([]);

	let newLevel = $state(5);
	let newRoleId = $state('');
	let addRoleOp = $state(createAsyncOp());
	let loadLeaderboardOp = $state(createAsyncOp());
	let activeTab = $state<'settings' | 'roles' | 'leaderboard'>('settings');

	async function loadConfig() {
		error = '';
		const data = await loadOp.run(
			() => api.request<{ config: LevelingConfig; level_roles: LevelRole[] }>(
				'GET', `/guilds/${guildId}/leveling`
			)
		);
		if (loadOp.error) {
			error = loadOp.error;
		} else if (data) {
			config = data.config;
			levelRoles = data.level_roles;
		}
	}

	async function loadGuildData() {
		try {
			const [r, c] = await Promise.all([
				api.getRoles(guildId),
				api.getGuildChannels(guildId)
			]);
			roles = r;
			channels = c.filter((ch: Channel) => ch.channel_type === 'text');
		} catch { /* ignore */ }
	}

	async function saveConfig() {
		error = '';
		success = '';
		const updated = await saveOp.run(
			() => api.request<LevelingConfig>(
				'PATCH', `/guilds/${guildId}/leveling`, {
					enabled: config.enabled,
					xp_per_message: config.xp_per_message,
					xp_cooldown_seconds: config.xp_cooldown_seconds,
					level_up_channel_id: config.level_up_channel_id,
					level_up_message: config.level_up_message,
					stack_roles: config.stack_roles
				}
			)
		);
		if (saveOp.error) {
			error = saveOp.error;
		} else if (updated) {
			config = updated;
			success = 'Settings saved';
			setTimeout(() => (success = ''), 3000);
		}
	}

	async function addLevelRole() {
		if (!newRoleId) return;
		error = '';
		const lr = await addRoleOp.run(
			() => api.request<LevelRole>(
				'POST', `/guilds/${guildId}/leveling/roles`, {
					level: newLevel,
					role_id: newRoleId
				}
			)
		);
		if (addRoleOp.error) {
			error = addRoleOp.error;
		} else if (lr) {
			levelRoles = [...levelRoles, lr].sort((a, b) => a.level - b.level);
			newLevel = 5;
			newRoleId = '';
		}
	}

	async function removeLevelRole(id: string) {
		try {
			await api.request('DELETE', `/guilds/${guildId}/leveling/roles/${id}`);
			levelRoles = levelRoles.filter(lr => lr.id !== id);
		} catch (err: any) {
			error = err.message || 'Failed to remove level role';
		}
	}

	async function loadLeaderboard() {
		const result = await loadLeaderboardOp.run(
			() => api.request<MemberXP[]>(
				'GET', `/guilds/${guildId}/leveling/leaderboard?limit=50`
			)
		);
		if (loadLeaderboardOp.error) {
			error = loadLeaderboardOp.error;
		} else if (result) {
			leaderboard = result;
		}
	}

	function getRoleName(roleId: string): string {
		return roles.find(r => r.id === roleId)?.name ?? roleId;
	}

	function getChannelName(chId: string): string {
		return channels.find(c => c.id === chId)?.name ?? chId;
	}

	$effect(() => {
		if (guildId) {
			loadConfig();
			loadGuildData();
		}
	});
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<h2 class="text-lg font-semibold text-text-primary">Leveling / XP System</h2>
	</div>

	{#if error}
		<div class="rounded bg-red-500/10 px-4 py-3 text-sm text-red-400">{error}</div>
	{/if}
	{#if success}
		<div class="rounded bg-green-500/10 px-4 py-3 text-sm text-green-400">{success}</div>
	{/if}

	{#if loadOp.loading}
		<div class="flex items-center justify-center py-12">
			<div class="h-8 w-8 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
		</div>
	{:else}
		<!-- Tabs -->
		<div class="flex gap-2 border-b border-bg-tertiary pb-2">
			<button
				class="rounded px-3 py-1.5 text-sm font-medium transition-colors"
				class:bg-brand-500={activeTab === 'settings'}
				class:text-white={activeTab === 'settings'}
				class:text-text-muted={activeTab !== 'settings'}
				onclick={() => (activeTab = 'settings')}
			>Settings</button>
			<button
				class="rounded px-3 py-1.5 text-sm font-medium transition-colors"
				class:bg-brand-500={activeTab === 'roles'}
				class:text-white={activeTab === 'roles'}
				class:text-text-muted={activeTab !== 'roles'}
				onclick={() => (activeTab = 'roles')}
			>Role Rewards</button>
			<button
				class="rounded px-3 py-1.5 text-sm font-medium transition-colors"
				class:bg-brand-500={activeTab === 'leaderboard'}
				class:text-white={activeTab === 'leaderboard'}
				class:text-text-muted={activeTab !== 'leaderboard'}
				onclick={() => { activeTab = 'leaderboard'; loadLeaderboard(); }}
			>Leaderboard</button>
		</div>

		{#if activeTab === 'settings'}
			<div class="space-y-4">
				<label class="flex items-center gap-3">
					<input type="checkbox" bind:checked={config.enabled} class="h-4 w-4 rounded" />
					<span class="text-sm text-text-primary">Enable leveling system</span>
				</label>

				<div>
					<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
						XP Per Message
					</label>
					<input type="number" class="input w-32" bind:value={config.xp_per_message}
						min="1" max="100" />
				</div>

				<div>
					<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
						XP Cooldown (seconds)
					</label>
					<input type="number" class="input w-32" bind:value={config.xp_cooldown_seconds}
						min="0" max="600" />
					<p class="mt-1 text-xs text-text-muted">Time before a user can earn XP again</p>
				</div>

				<div>
					<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
						Level Up Channel
					</label>
					<select class="input w-full" bind:value={config.level_up_channel_id}>
						<option value={null}>None (no notification)</option>
						{#each channels as ch}
							<option value={ch.id}>#{ch.name}</option>
						{/each}
					</select>
				</div>

				<div>
					<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
						Level Up Message
					</label>
					<textarea class="input w-full" rows="2" bind:value={config.level_up_message}></textarea>
					<p class="mt-1 text-xs text-text-muted">
						Variables: {'{user}'}, {'{level}'}, {'{username}'}
					</p>
				</div>

				<label class="flex items-center gap-3">
					<input type="checkbox" bind:checked={config.stack_roles} class="h-4 w-4 rounded" />
					<span class="text-sm text-text-primary">Stack role rewards (keep lower level roles)</span>
				</label>

				<button class="btn-primary" onclick={saveConfig} disabled={saveOp.loading}>
					{saveOp.loading ? 'Saving...' : 'Save Settings'}
				</button>
			</div>

		{:else if activeTab === 'roles'}
			<div class="space-y-4">
				<p class="text-sm text-text-muted">
					Assign roles automatically when members reach a level.
				</p>

				{#if levelRoles.length > 0}
					<div class="space-y-2">
						{#each levelRoles as lr}
							<div class="flex items-center justify-between rounded bg-bg-secondary px-3 py-2">
								<div class="text-sm text-text-primary">
									Level <span class="font-bold text-brand-400">{lr.level}</span>
									&rarr;
									<span class="font-medium">{getRoleName(lr.role_id)}</span>
								</div>
								<button class="text-xs text-red-400 hover:text-red-300"
									onclick={() => removeLevelRole(lr.id)}>Remove</button>
							</div>
						{/each}
					</div>
				{:else}
					<div class="rounded bg-bg-secondary px-4 py-6 text-center text-sm text-text-muted">
						No level role rewards configured yet
					</div>
				{/if}

				<div class="flex gap-2">
					<div>
						<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Level</label>
						<input type="number" class="input w-20" bind:value={newLevel} min="1" max="100" />
					</div>
					<div class="flex-1">
						<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">Role</label>
						<select class="input w-full" bind:value={newRoleId}>
							<option value="">Select role...</option>
							{#each roles as role}
								<option value={role.id}>{role.name}</option>
							{/each}
						</select>
					</div>
					<div class="flex items-end">
						<button class="btn-primary" onclick={addLevelRole} disabled={addRoleOp.loading || !newRoleId}>
							{addRoleOp.loading ? 'Adding...' : 'Add'}
						</button>
					</div>
				</div>
			</div>

		{:else if activeTab === 'leaderboard'}
			{#if loadLeaderboardOp.loading}
				<div class="flex items-center justify-center py-8">
					<div class="h-6 w-6 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
				</div>
			{:else if leaderboard.length === 0}
				<div class="rounded bg-bg-secondary px-4 py-6 text-center text-sm text-text-muted">
					No XP data yet. Members earn XP by chatting.
				</div>
			{:else}
				<div class="space-y-1">
					{#each leaderboard as member, i}
						<div class="flex items-center gap-3 rounded bg-bg-secondary px-3 py-2">
							<span class="w-8 text-right text-sm font-bold"
								class:text-yellow-400={i === 0}
								class:text-gray-300={i === 1}
								class:text-orange-400={i === 2}
								class:text-text-muted={i > 2}
							>#{i + 1}</span>
							<div class="flex-1">
								<span class="text-sm font-medium text-text-primary">
									{member.display_name ?? member.username}
								</span>
								{#if member.display_name}
									<span class="text-xs text-text-muted">@{member.username}</span>
								{/if}
							</div>
							<div class="text-right">
								<div class="text-sm font-bold text-brand-400">Lv. {member.level}</div>
								<div class="text-xs text-text-muted">{member.xp.toLocaleString()} XP</div>
							</div>
						</div>
					{/each}
				</div>
			{/if}
		{/if}
	{/if}
</div>
