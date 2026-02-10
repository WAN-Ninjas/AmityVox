<script lang="ts">
	import { guildList, currentGuildId, setGuild } from '$lib/stores/guilds';
	import Avatar from '$components/common/Avatar.svelte';

	let showCreateModal = $state(false);

	function selectGuild(id: string) {
		setGuild(id);
	}
</script>

<nav class="flex h-full w-[72px] shrink-0 flex-col items-center gap-2 overflow-y-auto bg-bg-floating py-3">
	<!-- Home / DMs button -->
	<button
		class="flex h-12 w-12 items-center justify-center rounded-2xl bg-bg-tertiary text-text-primary transition-all hover:rounded-xl hover:bg-brand-500"
		class:!rounded-xl={$currentGuildId === null}
		class:!bg-brand-500={$currentGuildId === null}
		onclick={() => setGuild(null)}
		title="Home"
	>
		<svg class="h-6 w-6" fill="currentColor" viewBox="0 0 24 24">
			<path d="M10 20v-6h4v6h5v-8h3L12 3 2 12h3v8z" />
		</svg>
	</button>

	<div class="mx-auto w-8 border-t border-bg-modifier"></div>

	<!-- Guild list -->
	{#each $guildList as guild (guild.id)}
		<button
			class="group relative flex h-12 w-12 items-center justify-center rounded-2xl bg-bg-tertiary transition-all hover:rounded-xl hover:bg-brand-500"
			class:!rounded-xl={$currentGuildId === guild.id}
			class:!bg-brand-500={$currentGuildId === guild.id}
			onclick={() => selectGuild(guild.id)}
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

			<!-- Active indicator -->
			{#if $currentGuildId === guild.id}
				<div class="absolute -left-1 h-10 w-1 rounded-r-full bg-text-primary"></div>
			{/if}
		</button>
	{/each}

	<!-- Add guild button -->
	<button
		class="flex h-12 w-12 items-center justify-center rounded-2xl bg-bg-tertiary text-green-500 transition-all hover:rounded-xl hover:bg-green-500 hover:text-white"
		onclick={() => (showCreateModal = true)}
		title="Create or Join a Guild"
	>
		<svg class="h-6 w-6" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path d="M12 5v14m-7-7h14" />
		</svg>
	</button>
</nav>
