<script lang="ts">
	import type { UserLink } from '$lib/types';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { createAsyncOp } from '$lib/utils/asyncOp';

	let links = $state<UserLink[]>([]);
	let loadOp = $state(createAsyncOp(true));
	let newPlatform = $state('website');
	let newLabel = $state('');
	let newUrl = $state('');
	let addOp = $state(createAsyncOp());
	let editingLinkId = $state<string | null>(null);
	let editPlatform = $state('website');
	let editLabel = $state('');
	let editUrl = $state('');
	let saveOp = $state(createAsyncOp());

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
		loadOp.run(
			async () => {
				links = await api.getMyLinks();
			},
			undefined,
			'Failed to load profile links'
		);
	});

	async function addLink() {
		if (!newLabel.trim() || !newUrl.trim()) return;
		await addOp.run(
			async () => {
				const link = await api.createLink(newPlatform, newLabel.trim(), newUrl.trim());
				links = [...links, link];
				newLabel = '';
				newUrl = '';
				newPlatform = 'website';
				addToast('Link added', 'success');
			},
			(message) => addToast(message, 'error'),
			'Failed to add link'
		);
	}

	function startEdit(link: UserLink) {
		editingLinkId = link.id;
		editPlatform = link.platform;
		editLabel = link.label;
		editUrl = link.url;
	}

	async function saveLink(linkId: string) {
		if (!editLabel.trim() || !editUrl.trim()) return;
		const updated = await saveOp.run(
			() => api.updateLink(linkId, {
				platform: editPlatform,
				label: editLabel.trim(),
				url: editUrl.trim()
			}),
			(message) => addToast(message, 'error'),
			'Failed to update link'
		);
		if (updated) {
			links = links.map((link) => link.id === linkId ? updated : link);
			editingLinkId = null;
			addToast('Link updated', 'success');
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

	{#if loadOp.loading}
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
						{#if editingLinkId === link.id}
							<select class="input w-28 text-sm" bind:value={editPlatform}>
								{#each platformOptions as opt}
									<option value={opt.value}>{opt.label}</option>
								{/each}
							</select>
							<input class="input min-w-0 flex-1 text-sm" bind:value={editLabel} maxlength="100" />
							<input class="input min-w-0 flex-1 text-sm" type="url" bind:value={editUrl} maxlength="500" />
							<button class="text-xs text-brand-400 hover:text-brand-300" onclick={() => saveLink(link.id)} disabled={saveOp.loading || !editLabel.trim() || !editUrl.trim()}>
								{saveOp.loading ? 'Saving...' : 'Save'}
							</button>
							<button class="text-xs text-text-muted hover:text-text-primary" onclick={() => (editingLinkId = null)}>
								Cancel
							</button>
						{:else}
							<span class="rounded bg-bg-modifier px-1.5 py-0.5 text-2xs font-medium text-text-muted">{link.platform}</span>
							<span class="flex-1 truncate text-sm text-text-secondary">{link.label}</span>
							<a href={link.url} target="_blank" rel="noopener" class="text-xs text-text-link hover:underline truncate max-w-32">{link.url}</a>
							<button
								class="text-text-muted hover:text-text-primary"
								onclick={() => startEdit(link)}
								title="Edit link"
							>
								<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
									<path d="M15.232 5.232l3.536 3.536M4 20h4l10.5-10.5a2.5 2.5 0 00-3.536-3.536L4 16.928V20z" />
								</svg>
							</button>
							<button
								class="text-text-muted hover:text-red-400"
								onclick={() => removeLink(link.id)}
								title="Remove link"
							>
								<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
									<path d="M6 18L18 6M6 6l12 12" />
								</svg>
							</button>
						{/if}
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
					disabled={addOp.loading || !newLabel.trim() || !newUrl.trim()}
				>
					{addOp.loading ? '...' : 'Add'}
				</button>
			</div>
		</div>
	{/if}
</div>
