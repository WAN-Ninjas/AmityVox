<script lang="ts">
	import { api } from '$lib/api/client';
	import { createAsyncOp } from '$lib/utils/asyncOp';
	import type { AuditLogEntry } from '$lib/types';

	interface Props {
		guildId: string;
	}

	let { guildId }: Props = $props();

	let auditLog = $state<AuditLogEntry[]>([]);
	let loadOp = $state(createAsyncOp());
	let loadedGuildId = $state<string | null>(null);

	const actionTypeLabels: Record<string, string> = {
		guild_update: 'Server Updated',
		channel_create: 'Channel Created',
		channel_update: 'Channel Updated',
		channel_delete: 'Channel Deleted',
		role_create: 'Role Created',
		role_update: 'Role Updated',
		role_delete: 'Role Deleted',
		member_kick: 'Member Kicked',
		member_ban: 'Member Banned',
		member_unban: 'Member Unbanned',
		invite_create: 'Invite Created',
		invite_delete: 'Invite Deleted',
		webhook_create: 'Webhook Created',
		webhook_update: 'Webhook Updated',
		webhook_delete: 'Webhook Deleted'
	};

	$effect(() => {
		if (guildId && loadedGuildId !== guildId && !loadOp.loading) {
			loadAudit();
		}
	});

	async function loadAudit() {
		const result = await loadOp.run(() => api.getAuditLog(guildId, { limit: 50 }));
		if (result) {
			auditLog = result;
			loadedGuildId = guildId;
		} else {
			auditLog = [];
		}
	}

	function formatDate(iso: string): string {
		return new Date(iso).toLocaleString();
	}
</script>

<h1 class="mb-6 text-xl font-bold text-text-primary">Audit Log</h1>

{#if loadOp.loading}
	<p class="text-sm text-text-muted">Loading audit log...</p>
{:else if auditLog.length === 0}
	<p class="text-sm text-text-muted">No audit log entries.</p>
{:else}
	<div class="space-y-2">
		{#each auditLog as entry (entry.id)}
			<div class="rounded-lg bg-bg-secondary p-3">
				<div class="flex items-center justify-between">
					<div class="flex items-center gap-2">
						<span class="text-sm font-medium text-text-primary">
							{entry.actor?.display_name ?? entry.actor?.username ?? entry.actor_id.slice(0, 8)}
						</span>
						<span class="rounded bg-bg-modifier px-1.5 py-0.5 text-xs text-text-muted">
							{actionTypeLabels[entry.action_type] ?? entry.action_type}
						</span>
					</div>
					<span class="text-xs text-text-muted">{formatDate(entry.created_at)}</span>
				</div>
				{#if entry.reason}
					<p class="mt-1 text-xs text-text-muted">Reason: {entry.reason}</p>
				{/if}
			</div>
		{/each}
	</div>
{/if}
