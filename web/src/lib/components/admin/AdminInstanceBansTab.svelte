<script lang="ts">
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import type { InstanceBan } from '$lib/types';

	let instanceBans = $state<InstanceBan[]>([]);
	let bansLoaded = $state(false);
	let loadingBans = $state(false);

	$effect(() => {
		if (!bansLoaded && !loadingBans) {
			loadInstanceBans();
		}
	});

	async function loadInstanceBans() {
		loadingBans = true;
		try {
			instanceBans = await api.getInstanceBans();
			bansLoaded = true;
		} catch {
			instanceBans = [];
		} finally {
			loadingBans = false;
		}
	}

	async function handleInstanceUnban(userId: string) {
		try {
			await api.instanceUnbanUser(userId);
			instanceBans = instanceBans.filter((ban) => ban.user_id !== userId);
			addToast('User unbanned', 'success');
		} catch {
			addToast('Failed to unban user', 'error');
		}
	}
</script>

<div class="mb-6">
	<h1 class="text-2xl font-bold text-text-primary">Instance Bans</h1>
	<p class="mt-1 text-sm text-text-muted">Users banned from this instance.</p>
</div>

{#if loadingBans}
	<p class="text-sm text-text-muted">Loading bans...</p>
{:else if instanceBans.length === 0}
	<p class="text-sm text-text-muted">No banned users.</p>
{:else}
	<div class="overflow-hidden rounded-lg border border-bg-modifier">
		<table class="w-full text-left text-sm">
			<thead class="bg-bg-secondary">
				<tr>
					<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">User</th>
					<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Reason</th>
					<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Banned By</th>
					<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted">Date</th>
					<th class="px-4 py-3 text-xs font-bold uppercase tracking-wide text-text-muted"></th>
				</tr>
			</thead>
			<tbody class="divide-y divide-bg-modifier">
				{#each instanceBans as ban (ban.user_id)}
					<tr class="hover:bg-bg-secondary/50">
						<td class="px-4 py-3">
							<div class="font-medium text-text-primary">{ban.display_name ?? `@${ban.username}`}</div>
							<div class="text-xs text-text-muted">{ban.user_id}</div>
						</td>
						<td class="px-4 py-3 text-text-secondary">{ban.reason || 'No reason provided'}</td>
						<td class="px-4 py-3 text-text-muted">{ban.admin_id}</td>
						<td class="px-4 py-3 text-text-muted">{new Date(ban.created_at).toLocaleString()}</td>
						<td class="px-4 py-3 text-right">
							<button class="text-xs text-green-400 hover:text-green-300" onclick={() => handleInstanceUnban(ban.user_id)}>Unban</button>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}
