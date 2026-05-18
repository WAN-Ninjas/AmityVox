<script lang="ts">
	import FederationBadge from '$components/common/FederationBadge.svelte';
	import type { Guild, User } from '$lib/types';

	interface Props {
		guild: Guild;
		currentUser: User | null;
		totalUnreads: number;
		canManageGuild: boolean;
		onmarkallread: () => void;
		oninvite: () => void;
		onsettings: () => void;
		oncontextmenu: (event: MouseEvent) => void;
	}

	let {
		guild,
		currentUser,
		totalUnreads,
		canManageGuild,
		onmarkallread,
		oninvite,
		onsettings,
		oncontextmenu
	}: Props = $props();
</script>

<div
	class="flex h-12 items-center justify-between border-b border-bg-floating px-4"
	oncontextmenu={oncontextmenu}
	role="button"
	tabindex="0"
>
	<div class="flex min-w-0 items-center gap-1.5">
		<h2 class="truncate text-sm font-semibold text-text-primary">{guild.name}</h2>
		{#if guild.instance_id && currentUser && guild.instance_id !== currentUser.instance_id}
			<FederationBadge domain={guild.instance_domain || guild.instance_id} compact />
		{/if}
	</div>
	<div class="flex items-center gap-1">
		{#if totalUnreads > 0}
			<button
				class="rounded p-1 text-text-muted hover:bg-bg-modifier hover:text-text-primary"
				onclick={onmarkallread}
				title="Mark All as Read"
			>
				<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
				</svg>
			</button>
		{/if}
		<button
			class="rounded p-1 text-text-muted hover:bg-bg-modifier hover:text-text-primary"
			onclick={oninvite}
			title="Create Invite"
		>
			<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M18 9v3m0 0v3m0-3h3m-3 0h-3m-2-5a4 4 0 11-8 0 4 4 0 018 0zM3 20a6 6 0 0112 0v1H3v-1z" />
			</svg>
		</button>
		{#if canManageGuild}
			<button
				class="rounded p-1 text-text-muted hover:bg-bg-modifier hover:text-text-primary"
				onclick={onsettings}
				title="Server Settings"
			>
				<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M12 6V4m0 2a2 2 0 100 4m0-4a2 2 0 110 4m-6 8a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4m6 6v10m6-2a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4" />
				</svg>
			</button>
		{/if}
	</div>
</div>
