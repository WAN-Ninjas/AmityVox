<script lang="ts">
	import type { MediaTag } from '$lib/types';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';

	interface Props {
		guildId: string;
	}

	let { guildId }: Props = $props();

	let tags = $state<MediaTag[]>([]);
	let loading = $state(true);
	let newTagName = $state('');
	let adding = $state(false);

	$effect(() => {
		api.getMediaTags(guildId)
			.then((t) => { tags = t; })
			.catch(() => {})
			.finally(() => { loading = false; });
	});

	async function addTag() {
		if (!newTagName.trim() || adding) return;
		adding = true;
		try {
			const tag = await api.createMediaTag(guildId, newTagName.trim());
			tags = [...tags, tag];
			newTagName = '';
			addToast('Tag created', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to create tag', 'error');
		} finally {
			adding = false;
		}
	}

	async function removeTag(tagId: string) {
		try {
			await api.deleteMediaTag(guildId, tagId);
			tags = tags.filter((t) => t.id !== tagId);
			addToast('Tag deleted', 'success');
		} catch {
			addToast('Failed to delete tag', 'error');
		}
	}
</script>

<div class="space-y-3">
	<h3 class="text-sm font-semibold text-text-primary">Media Tags</h3>
	<p class="text-xs text-text-muted">Tags help organize media in your server's gallery.</p>

	{#if loading}
		<div class="flex items-center gap-2 py-2 text-sm text-text-muted">
			<div class="h-4 w-4 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
			Loading...
		</div>
	{:else}
		<!-- Existing tags -->
		{#if tags.length > 0}
			<div class="flex flex-wrap gap-2">
				{#each tags as tag (tag.id)}
					<div class="flex items-center gap-1.5 rounded-full bg-bg-modifier px-3 py-1">
						<span class="text-xs text-text-secondary">{tag.name}</span>
						<button
							class="text-text-muted transition-colors hover:text-red-400"
							onclick={() => removeTag(tag.id)}
							title="Delete tag"
						>
							<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
								<path d="M6 18L18 6M6 6l12 12" />
							</svg>
						</button>
					</div>
				{/each}
			</div>
		{:else}
			<p class="text-xs text-text-muted">No tags yet.</p>
		{/if}

		<!-- Add new tag -->
		<div class="flex gap-2">
			<input
				type="text"
				class="input flex-1 text-sm"
				placeholder="New tag name"
				bind:value={newTagName}
				maxlength="50"
				onkeydown={(e) => e.key === 'Enter' && addTag()}
			/>
			<button
				class="btn-primary text-sm"
				onclick={addTag}
				disabled={adding || !newTagName.trim()}
			>
				{adding ? '...' : 'Add'}
			</button>
		</div>
	{/if}
</div>
