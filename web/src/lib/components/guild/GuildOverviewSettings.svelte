<script lang="ts">
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import Avatar from '$components/common/Avatar.svelte';
	import { updateGuild } from '$lib/stores/guilds';
	import { addToast } from '$lib/stores/toast';
	import { fileUrl } from '$lib/utils/avatar';
	import { createAsyncOp } from '$lib/utils/asyncOp';
	import { getErrorMessage } from '$lib/utils/apiError';
	import type { Guild } from '$lib/types';

	interface Props {
		guild: Guild;
		isOwner: boolean;
	}

	let { guild, isOwner }: Props = $props();

	let name = $state('');
	let description = $state('');
	let verificationLevel = $state(0);
	let iconFile = $state<File | null>(null);
	let iconPreview = $state<string | null>(null);
	let guildTags = $state<string[]>([]);
	let discoverable = $state(false);
	let deleteConfirm = $state('');
	let saveOp = $state(createAsyncOp());

	const availableTags = ['Gaming', 'Music', 'Education', 'Science & Tech', 'Entertainment', 'Art & Creative', 'Community', 'Other'];

	$effect(() => {
		name = guild.name;
		description = guild.description ?? '';
		verificationLevel = guild.verification_level ?? 0;
		guildTags = [...(guild.tags ?? [])];
		discoverable = guild.discoverable ?? false;
		deleteConfirm = '';
		iconFile = null;
		iconPreview = null;
	});

	function handleIconSelect(e: Event) {
		const file = (e.target as HTMLInputElement).files?.[0];
		if (!file?.type.startsWith('image/')) return;
		iconFile = file;
		iconPreview = URL.createObjectURL(file);
	}

	async function handleSave() {
		await saveOp.run(async () => {
			let iconId: string | undefined;
			if (iconFile) {
				const uploaded = await api.uploadFile(iconFile);
				iconId = uploaded.id;
			}

			const payload: Record<string, unknown> = {
				name,
				description: description || undefined,
				verification_level: verificationLevel,
				tags: guildTags,
				discoverable
			};
			if (iconId) payload.icon_id = iconId;

			const updated = await api.updateGuild(guild.id, payload as any);
			updateGuild(updated);
			iconFile = null;
			iconPreview = null;
			addToast('Server updated', 'success');
		}, msg => addToast(msg, 'error'), 'Failed to save server');
	}

	async function handleDelete() {
		if (deleteConfirm !== guild.name) {
			addToast('Type the server name to confirm deletion', 'error');
			return;
		}
		try {
			await api.deleteGuild(guild.id);
			goto('/app');
		} catch (err: unknown) {
			addToast(getErrorMessage(err, 'Failed to delete server'), 'error');
		}
	}

	function removeTag(index: number) {
		guildTags = guildTags.filter((_, candidateIndex) => candidateIndex !== index);
	}

	function addTag(tag: string) {
		guildTags = [...guildTags, tag];
	}
</script>

<h1 class="mb-6 text-xl font-bold text-text-primary">Server Overview</h1>

<div class="mb-6 flex items-center gap-4">
	<div class="relative">
		<Avatar
			name={guild.name}
			src={iconPreview ?? (guild.icon_id ? fileUrl(guild.icon_id, guild.instance_id || undefined) : null)}
			size="lg"
		/>
		<label class="absolute inset-0 flex cursor-pointer items-center justify-center rounded-full bg-black/50 opacity-0 transition-opacity hover:opacity-100">
			<svg class="h-6 w-6 text-white" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M3 9a2 2 0 012-2h.93a2 2 0 001.664-.89l.812-1.22A2 2 0 0110.07 4h3.86a2 2 0 011.664.89l.812 1.22A2 2 0 0018.07 7H19a2 2 0 012 2v9a2 2 0 01-2 2H5a2 2 0 01-2-2V9z" />
				<circle cx="12" cy="13" r="3" />
			</svg>
			<input type="file" accept="image/*" class="hidden" onchange={handleIconSelect} />
		</label>
	</div>
	<div class="text-sm text-text-muted">Click the icon to upload a new server image.</div>
</div>

<div class="mb-4">
	<label for="guildName" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Server Name</label>
	<input id="guildName" type="text" bind:value={name} class="input w-full" maxlength="100" />
</div>

<div class="mb-6">
	<label for="guildDesc" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Description</label>
	<textarea id="guildDesc" bind:value={description} class="input w-full" rows="3" maxlength="1024"></textarea>
</div>

<div class="mb-6">
	<label for="verificationLevel" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Verification Level</label>
	<select id="verificationLevel" bind:value={verificationLevel} class="input w-full">
		<option value={0}>None</option>
		<option value={1}>Low - Verified email</option>
		<option value={2}>Medium - Registered 5+ min</option>
		<option value={3}>High - Member 10+ min</option>
		<option value={4}>Highest - Phone verified</option>
	</select>
	<p class="mt-1 text-xs text-text-muted">
		{#if verificationLevel === 0}
			No verification required to participate. Anyone can join and send messages immediately.
		{:else if verificationLevel === 1}
			Members must have a verified email address on their account.
		{:else if verificationLevel === 2}
			Members must be registered on this instance for at least 5 minutes.
		{:else if verificationLevel === 3}
			Members must have been a member of this server for at least 10 minutes before they can participate.
		{:else}
			Members must have a verified phone number linked to their account.
		{/if}
	</p>
</div>

<div class="mb-6">
	<div class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Category Tags</div>
	<p class="mb-2 text-xs text-text-muted">Add tags to help users find your server in discovery. Max 5 tags.</p>
	<div class="mb-2 flex flex-wrap gap-1.5">
		{#each guildTags as tag, index}
			<span class="flex items-center gap-1 rounded-full bg-brand-500/15 px-2.5 py-1 text-xs text-brand-400">
				{tag}
				<button class="ml-0.5 text-brand-400/70 hover:text-brand-400" aria-label={`Remove ${tag} tag`} onclick={() => removeTag(index)}>
					<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path d="M6 18L18 6M6 6l12 12" /></svg>
				</button>
			</span>
		{/each}
	</div>
	{#if guildTags.length < 5}
		<div class="flex flex-wrap gap-1.5">
			{#each availableTags.filter((tag) => !guildTags.includes(tag)) as tag}
				<button class="rounded-full bg-bg-modifier px-2.5 py-1 text-xs text-text-muted transition-colors hover:bg-bg-tertiary hover:text-text-primary" onclick={() => addTag(tag)}>
					+ {tag}
				</button>
			{/each}
		</div>
	{/if}
</div>

<div class="mb-6">
	<label class="flex items-center gap-3">
		<input type="checkbox" bind:checked={discoverable} class="rounded" />
		<div>
			<span class="text-sm font-medium text-text-primary">Show in Server Discovery</span>
			<p class="text-xs text-text-muted">Allow anyone to find and join this server from the Discover Servers page</p>
		</div>
	</label>
</div>

<button class="btn-primary" onclick={handleSave} disabled={saveOp.loading}>
	{saveOp.loading ? 'Saving...' : 'Save Changes'}
</button>

{#if isOwner}
	<div class="mt-12 border-t border-bg-modifier pt-6">
		<h2 class="mb-2 text-lg font-semibold text-red-400">Danger Zone</h2>
		<p class="mb-3 text-sm text-text-muted">
			Deleting a server is permanent and cannot be undone. Type <strong class="text-text-primary">{guild.name}</strong> to confirm.
		</p>
		<input type="text" class="input mb-3 w-full" bind:value={deleteConfirm} placeholder="Type server name to confirm..." />
		<button
			class="rounded bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
			onclick={handleDelete}
			disabled={deleteConfirm !== guild.name}
		>
			Delete Server
		</button>
	</div>
{/if}
