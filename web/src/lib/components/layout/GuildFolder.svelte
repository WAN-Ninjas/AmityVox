<script lang="ts">
	import type { Guild, GuildFolder as GuildFolderType } from '$lib/types';
	import { removeGuildFolder, loadGuildFolders } from '$lib/stores/guilds';
	import { addToast } from '$lib/stores/toast';
	import { api } from '$lib/api/client';

	interface Props {
		folder: GuildFolderType;
		guilds: Guild[];
		currentGuildId: string | null;
		guildHasUnreads: (id: string) => boolean;
		onselectguild: (id: string) => void;
		ondropguild?: (guildId: string, folderId: string) => void;
	}

	let { folder, guilds, currentGuildId, guildHasUnreads, onselectguild, ondropguild }: Props = $props();

	let expanded = $state(false);
	let isPointerOver = $state(false);

	async function deleteFolder() {
		try {
			await api.deleteGuildFolder(folder.id);
			removeGuildFolder(folder.id);
			addToast('Folder deleted', 'info');
		} catch (err: any) {
			addToast(err.message || 'Failed to delete folder', 'error');
		}
	}

	// Show first 4 guild icons in a 2x2 grid when collapsed.
	const previewGuilds = $derived(guilds.slice(0, 4));
	const hasAnyUnread = $derived(guilds.some(g => guildHasUnreads(g.id)));
	const containsCurrent = $derived(guilds.some(g => g.id === currentGuildId));
</script>

<div
	class="flex flex-col items-center"
	onpointerenter={() => { isPointerOver = true; }}
	onpointerleave={() => { isPointerOver = false; }}
	data-folder-id={folder.id}
	role="group"
>
	{#if expanded}
		<!-- Expanded: show folder name bar + guild list -->
		<button
			class="mb-1 flex h-5 w-9 items-center justify-center rounded text-2xs font-bold uppercase"
			style="color: {folder.color || 'var(--text-muted)'};"
			onclick={() => (expanded = false)}
			title="Collapse {folder.name}"
		>
			<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M19 9l-7 7-7-7" />
			</svg>
		</button>
		<div class="flex flex-col items-center gap-2 rounded-lg py-1 {isPointerOver ? 'ring-2 ring-brand-500' : ''}" style="border-left: 2px solid {folder.color || '#888'};">
			{#each guilds as guild (guild.id)}
				<button
					class="group relative flex h-9 w-9 items-center justify-center rounded-md border border-bg-modifier bg-bg-tertiary transition-colors hover:bg-brand-500"
					class:!bg-brand-500={currentGuildId === guild.id}
					onclick={() => onselectguild(guild.id)}
					title={guild.name}
				>
					{#if guild.icon_id}
						<img
							src="/api/v1/files/{guild.icon_id}"
							alt={guild.name}
							class="h-full w-full rounded-[inherit] object-cover"
						/>
					{:else}
						<span class="text-sm font-semibold text-text-primary">
							{guild.name.split(' ').map((w) => w[0]).join('').slice(0, 3)}
						</span>
					{/if}
					{#if currentGuildId === guild.id}
						<div class="absolute -left-2 h-5 w-0.5 bg-text-primary"></div>
					{:else if guildHasUnreads(guild.id)}
						<div class="absolute -left-2 h-2 w-0.5 bg-text-primary"></div>
					{/if}
				</button>
			{/each}
		</div>
	{:else}
		<!-- Collapsed: 2x2 mini-icon grid -->
		<button
			class="group relative flex h-9 w-9 items-center justify-center rounded-md border transition-colors {isPointerOver ? 'ring-2 ring-brand-500 border-brand-500 bg-brand-500/20' : containsCurrent ? 'border-brand-500 bg-brand-500/20' : 'border-bg-modifier bg-bg-tertiary hover:bg-bg-modifier'}"
			onclick={() => (expanded = true)}
			oncontextmenu={(e) => { e.preventDefault(); deleteFolder(); }}
			title="{folder.name} ({guilds.length} servers) — Right-click to delete"
		>
			{#if previewGuilds.length > 0}
				<div class="grid h-7 w-7 grid-cols-2 gap-0.5 overflow-hidden rounded-sm">
					{#each previewGuilds as guild (guild.id)}
						{#if guild.icon_id}
							<img
								src="/api/v1/files/{guild.icon_id}"
								alt=""
								class="h-full w-full object-cover"
							/>
						{:else}
							<div class="flex h-full w-full items-center justify-center bg-bg-modifier text-[0.5rem] font-bold text-text-muted">
								{guild.name[0]}
							</div>
						{/if}
					{/each}
					{#each Array(Math.max(0, 4 - previewGuilds.length)) as _}
						<div class="bg-bg-modifier"></div>
					{/each}
				</div>
			{:else}
				<svg class="h-4 w-4 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" />
				</svg>
			{/if}
			{#if hasAnyUnread && !containsCurrent}
				<div class="absolute -left-1 h-2 w-0.5 bg-text-primary"></div>
			{/if}
		</button>
	{/if}
	<span class="mt-0.5 max-w-[3rem] truncate text-center text-[0.6rem] leading-tight" style="color: {folder.color || 'var(--text-muted)'};">
		{folder.name}
	</span>
</div>
