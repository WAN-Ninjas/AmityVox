<script lang="ts">
	import { channelVoiceUsers } from '$lib/stores/voice';
	import { guilds } from '$lib/stores/guilds';
	import { channels } from '$lib/stores/channels';
	import { goto } from '$app/navigation';
	import Avatar from '$lib/components/common/Avatar.svelte';

	// Build a grouped view: guild -> channel -> participants.
	const activeVoice = $derived.by(() => {
		const groups: Array<{
			guildId: string;
			guildName: string;
			guildIconId: string | null;
			channels: Array<{
				channelId: string;
				channelName: string;
				participants: Array<{ userId: string; username: string; displayName: string | null; avatarId: string | null; speaking: boolean }>;
			}>;
		}> = [];

		for (const [channelId, participants] of $channelVoiceUsers) {
			if (participants.size === 0) continue;

			const channel = $channels.get(channelId);
			if (!channel) continue;

			const guild = channel.guild_id ? $guilds.get(channel.guild_id) : null;
			const guildId = guild?.id ?? 'unknown';

			let group = groups.find((g) => g.guildId === guildId);
			if (!group) {
				group = {
					guildId,
					guildName: guild?.name ?? 'Unknown Server',
					guildIconId: guild?.icon_id ?? null,
					channels: []
				};
				groups.push(group);
			}

			group.channels.push({
				channelId,
				channelName: channel.name,
				participants: Array.from(participants.values()).map((p) => ({
					userId: p.userId,
					username: p.username,
					displayName: p.displayName,
					avatarId: p.avatarId,
					speaking: p.speaking
				}))
			});
		}

		return groups;
	});

	const totalParticipants = $derived(
		activeVoice.reduce((sum, g) => sum + g.channels.reduce((s, c) => s + c.participants.length, 0), 0)
	);
</script>

<div class="rounded-lg bg-bg-secondary p-4">
	<div class="mb-3 flex items-center gap-2">
		<svg class="h-5 w-5 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path stroke-linecap="round" stroke-linejoin="round" d="M15.536 8.464a5 5 0 010 7.072m2.828-9.9a9 9 0 010 12.728M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
		</svg>
		<h3 class="text-sm font-semibold text-text-primary">Active Voice</h3>
		{#if totalParticipants > 0}
			<span class="rounded-full bg-green-500/20 px-1.5 py-0.5 text-2xs font-medium text-green-400">{totalParticipants}</span>
		{/if}
	</div>

	{#if activeVoice.length === 0}
		<p class="py-2 text-sm text-text-muted">No active voice channels.</p>
	{:else}
		<div class="space-y-3">
			{#each activeVoice as group (group.guildId)}
				<div>
					<div class="mb-1.5 flex items-center gap-1.5">
						{#if group.guildIconId}
							<img class="h-4 w-4 rounded object-cover" src="/api/v1/files/{group.guildIconId}" alt="" />
						{:else}
							<span class="flex h-4 w-4 items-center justify-center rounded bg-brand-600 text-2xs font-bold text-white">
								{group.guildName[0]?.toUpperCase() ?? '?'}
							</span>
						{/if}
						<span class="text-xs font-medium text-text-muted">{group.guildName}</span>
					</div>
					{#each group.channels as vc (vc.channelId)}
						<button
							class="mb-1 w-full rounded-md bg-bg-primary p-2 text-left transition-colors hover:bg-bg-modifier"
							onclick={() => goto(group.guildId === 'unknown' ? `/app/dms/${vc.channelId}` : `/app/guilds/${group.guildId}/channels/${vc.channelId}`)}
						>
							<div class="mb-1 flex items-center gap-1.5">
								<svg class="h-3.5 w-3.5 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" d="M15.536 8.464a5 5 0 010 7.072m2.828-9.9a9 9 0 010 12.728M5.586 15H4a1 1 0 01-1-1v-4a1 1 0 011-1h1.586l4.707-4.707C10.923 3.663 12 4.109 12 5v14c0 .891-1.077 1.337-1.707.707L5.586 15z" />
								</svg>
								<span class="text-xs font-medium text-text-secondary">{vc.channelName}</span>
								<span class="text-2xs text-text-muted">({vc.participants.length})</span>
							</div>
							<div class="flex flex-wrap gap-1">
								{#each vc.participants.slice(0, 8) as p (p.userId)}
									<div class="flex items-center gap-1 rounded bg-bg-secondary px-1.5 py-0.5" title={p.displayName ?? p.username}>
										<Avatar
											name={p.displayName ?? p.username}
											src={p.avatarId ? `/api/v1/files/${p.avatarId}` : null}
											size="sm"
										/>
										<span class="max-w-16 truncate text-2xs text-text-muted">{p.displayName ?? p.username}</span>
										{#if p.speaking}
											<span class="h-1.5 w-1.5 rounded-full bg-green-400 animate-pulse"></span>
										{/if}
									</div>
								{/each}
								{#if vc.participants.length > 8}
									<span class="flex items-center px-1 text-2xs text-text-muted">+{vc.participants.length - 8}</span>
								{/if}
							</div>
						</button>
					{/each}
				</div>
			{/each}
		</div>
	{/if}
</div>
