<script lang="ts">
	import { onMount } from 'svelte';
	import type { GuildMember } from '$lib/types';
	import { currentGuildId } from '$lib/stores/guilds';
	import { api } from '$lib/api/client';
	import Avatar from '$components/common/Avatar.svelte';
	import { getPresence } from '$lib/stores/presence';
	import { get } from 'svelte/store';
	import { presenceMap } from '$lib/stores/presence';

	let members = $state<GuildMember[]>([]);
	let visible = $state(true);

	$effect(() => {
		const guildId = $currentGuildId;
		if (guildId) {
			api.getMembers(guildId).then((m) => (members = m)).catch(() => {});
		} else {
			members = [];
		}
	});

	const onlineMembers = $derived(
		members.filter((m) => {
			const status = get(presenceMap).get(m.user_id);
			return status && status !== 'offline';
		})
	);

	const offlineMembers = $derived(
		members.filter((m) => {
			const status = get(presenceMap).get(m.user_id);
			return !status || status === 'offline';
		})
	);
</script>

{#if visible && $currentGuildId}
	<aside class="hidden w-60 shrink-0 overflow-y-auto bg-bg-secondary lg:block">
		<div class="p-3">
			{#if onlineMembers.length > 0}
				<h3 class="mb-1 px-1 text-2xs font-bold uppercase tracking-wide text-text-muted">
					Online — {onlineMembers.length}
				</h3>
				{#each onlineMembers as member (member.user_id)}
					<div class="flex items-center gap-2 rounded px-2 py-1.5 hover:bg-bg-modifier">
						<Avatar
							name={member.nickname ?? member.user?.display_name ?? member.user?.username ?? '?'}
							size="sm"
							status="online"
						/>
						<span class="truncate text-sm text-text-secondary">
							{member.nickname ?? member.user?.display_name ?? member.user?.username ?? 'Unknown'}
						</span>
					</div>
				{/each}
			{/if}

			{#if offlineMembers.length > 0}
				<h3 class="mb-1 mt-4 px-1 text-2xs font-bold uppercase tracking-wide text-text-muted">
					Offline — {offlineMembers.length}
				</h3>
				{#each offlineMembers as member (member.user_id)}
					<div class="flex items-center gap-2 rounded px-2 py-1.5 opacity-50 hover:bg-bg-modifier hover:opacity-75">
						<Avatar
							name={member.nickname ?? member.user?.display_name ?? member.user?.username ?? '?'}
							size="sm"
						/>
						<span class="truncate text-sm text-text-secondary">
							{member.nickname ?? member.user?.display_name ?? member.user?.username ?? 'Unknown'}
						</span>
					</div>
				{/each}
			{/if}
		</div>
	</aside>
{/if}
