<script lang="ts">
	import type { Embed, Reaction } from '$lib/types';

	interface Props {
		embeds?: Embed[];
		reactions?: Reaction[];
		ontogglereaction: (emoji: string) => void;
	}

	let { embeds = [], reactions = [], ontogglereaction }: Props = $props();
</script>

{#if embeds.length > 0}
	{#each embeds as embed}
		<div class="mt-1 max-w-md overflow-hidden rounded border-l-4 border-brand-500 bg-bg-secondary p-3">
			{#if embed.provider_name}
				<p class="text-xs text-text-muted">{embed.provider_name}</p>
			{/if}
			{#if embed.title}
				<p class="font-semibold text-text-link">
					{#if embed.url}
						<a href={embed.url} target="_blank" rel="noopener" class="hover:underline">{embed.title}</a>
					{:else}
						{embed.title}
					{/if}
				</p>
			{/if}
			{#if embed.description}
				<p class="mt-1 text-sm text-text-secondary">{embed.description}</p>
			{/if}
			{#if embed.thumbnail_url}
				<img src={embed.thumbnail_url} alt="" class="mt-2 max-h-60 rounded" loading="lazy" />
			{/if}
		</div>
	{/each}
{/if}

{#if reactions.length > 0}
	<div class="mt-1 flex flex-wrap gap-1">
		{#each reactions as reaction (reaction.emoji)}
			<button
				class="flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs transition-colors {reaction.me ? 'border-brand-500 bg-brand-500/10' : 'border-bg-modifier hover:border-brand-500'}"
				onclick={() => ontogglereaction(reaction.emoji)}
				title="{reaction.count} reaction{reaction.count !== 1 ? 's' : ''}"
			>
				<span>{reaction.emoji}</span>
				<span class="text-text-muted">{reaction.count}</span>
			</button>
		{/each}
	</div>
{/if}
