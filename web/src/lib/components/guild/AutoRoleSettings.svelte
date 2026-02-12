<script lang="ts">
	import { api } from '$lib/api/client';
	import type { Role } from '$lib/types';

	let { guildId }: { guildId: string } = $props();

	interface AutoRole {
		id: string;
		guild_id: string;
		role_id: string;
		rule_type: 'on_join' | 'after_delay' | 'on_verify';
		delay_seconds: number;
		enabled: boolean;
		created_at: string;
		role_name: string;
	}

	let loading = $state(false);
	let error = $state('');
	let success = $state('');

	let autoRoles = $state<AutoRole[]>([]);
	let roles = $state<Role[]>([]);

	let newRoleId = $state('');
	let newRuleType = $state<'on_join' | 'after_delay' | 'on_verify'>('on_join');
	let newDelaySeconds = $state(600);
	let creating = $state(false);

	async function loadAutoRoles() {
		loading = true;
		error = '';
		try {
			autoRoles = await api.request<AutoRole[]>('GET', `/guilds/${guildId}/auto-roles`);
		} catch (err: any) {
			error = err.message || 'Failed to load auto roles';
		} finally {
			loading = false;
		}
	}

	async function loadRoles() {
		try {
			roles = await api.getRoles(guildId);
		} catch { /* ignore */ }
	}

	async function createAutoRole() {
		if (!newRoleId) return;
		creating = true;
		error = '';
		success = '';
		try {
			const ar = await api.request<AutoRole>(
				'POST', `/guilds/${guildId}/auto-roles`, {
					role_id: newRoleId,
					rule_type: newRuleType,
					delay_seconds: newRuleType === 'after_delay' ? newDelaySeconds : 0
				}
			);
			autoRoles = [...autoRoles, ar];
			newRoleId = '';
			newRuleType = 'on_join';
			newDelaySeconds = 600;
			success = 'Auto-role created';
			setTimeout(() => (success = ''), 3000);
		} catch (err: any) {
			error = err.message || 'Failed to create auto role';
		} finally {
			creating = false;
		}
	}

	async function toggleAutoRole(rule: AutoRole) {
		try {
			const updated = await api.request<AutoRole>(
				'PATCH', `/guilds/${guildId}/auto-roles/${rule.id}`, {
					enabled: !rule.enabled
				}
			);
			autoRoles = autoRoles.map(ar => ar.id === updated.id ? updated : ar);
		} catch (err: any) {
			error = err.message || 'Failed to update auto role';
		}
	}

	async function deleteAutoRole(id: string) {
		try {
			await api.request('DELETE', `/guilds/${guildId}/auto-roles/${id}`);
			autoRoles = autoRoles.filter(ar => ar.id !== id);
		} catch (err: any) {
			error = err.message || 'Failed to delete auto role';
		}
	}

	function getRoleName(roleId: string): string {
		return roles.find(r => r.id === roleId)?.name ?? roleId;
	}

	function ruleTypeLabel(type: string): string {
		switch (type) {
			case 'on_join': return 'On Join';
			case 'after_delay': return 'After Delay';
			case 'on_verify': return 'On Verify';
			default: return type;
		}
	}

	function formatDelay(seconds: number): string {
		if (seconds < 60) return `${seconds}s`;
		if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
		return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`;
	}

	$effect(() => {
		if (guildId) {
			loadAutoRoles();
			loadRoles();
		}
	});
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<h2 class="text-lg font-semibold text-text-primary">Auto-Role Assignment</h2>
	</div>

	<p class="text-sm text-text-muted">
		Automatically assign roles to members when they join the server, after a delay, or when they pass verification.
	</p>

	{#if error}
		<div class="rounded bg-red-500/10 px-4 py-3 text-sm text-red-400">{error}</div>
	{/if}
	{#if success}
		<div class="rounded bg-green-500/10 px-4 py-3 text-sm text-green-400">{success}</div>
	{/if}

	{#if loading}
		<div class="flex items-center justify-center py-12">
			<div class="h-8 w-8 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
		</div>
	{:else}
		<!-- Existing Rules -->
		{#if autoRoles.length > 0}
			<div class="space-y-2">
				{#each autoRoles as rule}
					<div class="flex items-center justify-between rounded-lg bg-bg-secondary px-4 py-3">
						<div class="flex items-center gap-3">
							<input
								type="checkbox"
								checked={rule.enabled}
								onchange={() => toggleAutoRole(rule)}
								class="h-4 w-4 rounded"
							/>
							<div>
								<div class="text-sm font-medium text-text-primary">
									{rule.role_name || getRoleName(rule.role_id)}
								</div>
								<div class="text-xs text-text-muted">
									{ruleTypeLabel(rule.rule_type)}
									{#if rule.rule_type === 'after_delay'}
										({formatDelay(rule.delay_seconds)})
									{/if}
								</div>
							</div>
						</div>
						<button
							class="text-xs text-red-400 hover:text-red-300"
							onclick={() => deleteAutoRole(rule.id)}
						>Delete</button>
					</div>
				{/each}
			</div>
		{:else}
			<div class="rounded-lg bg-bg-secondary px-4 py-6 text-center text-sm text-text-muted">
				No auto-roles configured. Add one below.
			</div>
		{/if}

		<!-- Add New Rule -->
		<div class="rounded-lg bg-bg-secondary p-4">
			<h3 class="mb-3 text-sm font-semibold text-text-primary">Add Auto-Role</h3>

			<div class="space-y-3">
				<div>
					<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
						Role
					</label>
					<select class="input w-full" bind:value={newRoleId}>
						<option value="">Select role...</option>
						{#each roles as role}
							<option value={role.id}>{role.name}</option>
						{/each}
					</select>
				</div>

				<div>
					<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
						Rule Type
					</label>
					<select class="input w-full" bind:value={newRuleType}>
						<option value="on_join">On Join - assign immediately when member joins</option>
						<option value="after_delay">After Delay - assign after a time period</option>
						<option value="on_verify">On Verify - assign after passing verification</option>
					</select>
				</div>

				{#if newRuleType === 'after_delay'}
					<div>
						<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
							Delay
						</label>
						<select class="input w-full" bind:value={newDelaySeconds}>
							<option value={60}>1 minute</option>
							<option value={300}>5 minutes</option>
							<option value={600}>10 minutes</option>
							<option value={1800}>30 minutes</option>
							<option value={3600}>1 hour</option>
							<option value={21600}>6 hours</option>
							<option value={43200}>12 hours</option>
							<option value={86400}>24 hours</option>
						</select>
					</div>
				{/if}

				<button
					class="btn-primary"
					onclick={createAutoRole}
					disabled={creating || !newRoleId}
				>
					{creating ? 'Creating...' : 'Add Auto-Role'}
				</button>
			</div>
		</div>
	{/if}
</div>
