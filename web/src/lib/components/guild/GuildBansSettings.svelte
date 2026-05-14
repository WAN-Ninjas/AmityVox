<script lang="ts">
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import Avatar from '$components/common/Avatar.svelte';
	import type { Ban } from '$lib/types';

	interface Props {
		guildId: string;
	}

	let { guildId }: Props = $props();

	let bans = $state<Ban[]>([]);
	let loadingBans = $state(false);
	let loadedGuildId = $state<string | null>(null);

	$effect(() => {
		if (guildId && loadedGuildId !== guildId && !loadingBans) {
			loadBans();
		}
	});

	async function loadBans() {
		loadingBans = true;
		try {
			bans = await api.getGuildBans(guildId);
			loadedGuildId = guildId;
		} catch {
			bans = [];
		} finally {
			loadingBans = false;
		}
	}

	async function handleUnban(userId: string) {
		try {
			await api.unbanUser(guildId, userId);
			bans = bans.filter((ban) => ban.user_id !== userId);
			addToast('User unbanned', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to unban', 'error');
		}
	}
</script>

<h1 class="mb-6 text-xl font-bold text-text-primary">Bans</h1>

{#if loadingBans}
	<p class="text-sm text-text-muted">Loading bans...</p>
{:else if bans.length === 0}
	<p class="text-sm text-text-muted">No banned users.</p>
{:else}
	<div class="space-y-2">
		{#each bans as ban (ban.user_id)}
			<div class="flex items-center justify-between rounded-lg bg-bg-secondary p-3">
				<div class="flex items-center gap-3">
					<Avatar name={ban.user?.display_name ?? ban.user?.username ?? '?'} size="sm" />
					<div>
						<span class="text-sm font-medium text-text-primary">
							{ban.user?.display_name ?? ban.user?.username ?? ban.user_id}
						</span>
						{#if ban.reason}
							<p class="text-xs text-text-muted">Reason: {ban.reason}</p>
						{/if}
					</div>
				</div>
				<button
					class="text-xs text-red-400 hover:text-red-300"
					onclick={() => handleUnban(ban.user_id)}
				>
					Unban
				</button>
			</div>
		{/each}
	</div>
{/if}
