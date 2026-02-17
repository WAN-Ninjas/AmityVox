<script lang="ts">
	import type { UserLink } from '$lib/types';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';

	let links = $state<UserLink[]>([]);
	let loading = $state(true);
	let newPlatform = $state('website');
	let newLabel = $state('');
	let newUrl = $state('');
	let adding = $state(false);

	const platformOptions = [
		{ value: 'website', label: 'Website' },
		{ value: 'github', label: 'GitHub' },
		{ value: 'twitter', label: 'X / Twitter' },
		{ value: 'youtube', label: 'YouTube' },
		{ value: 'twitch', label: 'Twitch' },
		{ value: 'discord', label: 'Discord' },
		{ value: 'mastodon', label: 'Mastodon' },
		{ value: 'bluesky', label: 'Bluesky' },
		{ value: 'steam', label: 'Steam' },
		{ value: 'other', label: 'Other' },
	];

	$effect(() => {
		api.getMyLinks()
			.then((l) => { links = l; })
			.catch(() => {})
			.finally(() => { loading = false; });
	});

	async function addLink() {
		if (!newLabel.trim() || !newUrl.trim()) return;
		adding = true;
		try {
			const link = await api.createLink(newPlatform, newLabel.trim(), newUrl.trim());
			links = [...links, link];
			newLabel = '';
			newUrl = '';
			newPlatform = 'website';
			addToast('Link added', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to add link', 'error');
		} finally {
			adding = false;
		}
	}

	async function removeLink(linkId: string) {
		try {
			await api.deleteLink(linkId);
			links = links.filter((l) => l.id !== linkId);
			addToast('Link removed', 'success');
		} catch {
			addToast('Failed to remove link', 'error');
		}
	}
</script>

<div>
	<h3 class="mb-3 text-sm font-semibold text-text-primary">Profile Links</h3>
	<p class="mb-3 text-xs text-text-muted">Add links to your profile that others can see.</p>

	{#if loading}
		<div class="flex items-center gap-2 py-4 text-sm text-text-muted">
			<div class="h-4 w-4 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
			Loading...
		</div>
	{:else}
		<!-- Existing links -->
		{#if links.length > 0}
			<div class="mb-4 space-y-2">
				{#each links as link (link.id)}
					<div class="flex items-center gap-2 rounded-md bg-bg-primary px-3 py-2">
						<span class="rounded bg-bg-modifier px-1.5 py-0.5 text-2xs font-medium text-text-muted">{link.platform}</span>
						<span class="flex-1 truncate text-sm text-text-secondary">{link.label}</span>
						<a href={link.url} target="_blank" rel="noopener" class="text-xs text-text-link hover:underline truncate max-w-32">{link.url}</a>
						<button
							class="text-text-muted hover:text-red-400"
							onclick={() => removeLink(link.id)}
							title="Remove link"
						>
							<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
								<path d="M6 18L18 6M6 6l12 12" />
							</svg>
						</button>
					</div>
				{/each}
			</div>
		{/if}

		<!-- Add new link -->
		<div class="rounded-md border border-bg-modifier p-3">
			<div class="mb-2 flex gap-2">
				<select class="input w-28 text-sm" bind:value={newPlatform}>
					{#each platformOptions as opt}
						<option value={opt.value}>{opt.label}</option>
					{/each}
				</select>
				<input
					type="text"
					class="input flex-1 text-sm"
					placeholder="Label (e.g. My Website)"
					bind:value={newLabel}
					maxlength="100"
				/>
			</div>
			<div class="flex gap-2">
				<input
					type="url"
					class="input flex-1 text-sm"
					placeholder="https://example.com"
					bind:value={newUrl}
					maxlength="500"
					onkeydown={(e) => e.key === 'Enter' && addLink()}
				/>
				<button
					class="btn-primary text-sm"
					onclick={addLink}
					disabled={adding || !newLabel.trim() || !newUrl.trim()}
				>
					{adding ? '...' : 'Add'}
				</button>
			</div>
		</div>
	{/if}
</div>
