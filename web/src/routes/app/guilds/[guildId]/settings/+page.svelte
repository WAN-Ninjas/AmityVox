<script lang="ts">
	import { page } from '$app/stores';
	import { currentGuild } from '$lib/stores/guilds';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';

	let name = $state('');
	let description = $state('');
	let saving = $state(false);
	let error = $state('');

	$effect(() => {
		if ($currentGuild) {
			name = $currentGuild.name;
			description = $currentGuild.description ?? '';
		}
	});

	async function handleSave() {
		if (!$currentGuild) return;
		saving = true;
		error = '';

		try {
			await api.updateGuild($currentGuild.id, { name, description: description || undefined } as any);
		} catch (err: any) {
			error = err.message || 'Failed to save';
		} finally {
			saving = false;
		}
	}

	async function handleDelete() {
		if (!$currentGuild || !confirm('Are you sure you want to delete this guild? This cannot be undone.')) return;

		try {
			await api.deleteGuild($currentGuild.id);
			goto('/app');
		} catch (err: any) {
			error = err.message || 'Failed to delete';
		}
	}
</script>

<svelte:head>
	<title>Guild Settings — AmityVox</title>
</svelte:head>

<div class="flex h-full">
	<!-- Settings sidebar -->
	<nav class="w-52 shrink-0 overflow-y-auto bg-bg-secondary p-4">
		<h3 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Guild Settings</h3>
		<ul class="space-y-0.5">
			<li>
				<button class="w-full rounded px-2 py-1.5 text-left text-sm text-text-primary bg-bg-modifier">
					Overview
				</button>
			</li>
			<li>
				<button class="w-full rounded px-2 py-1.5 text-left text-sm text-text-muted hover:bg-bg-modifier hover:text-text-secondary">
					Roles
				</button>
			</li>
			<li>
				<button class="w-full rounded px-2 py-1.5 text-left text-sm text-text-muted hover:bg-bg-modifier hover:text-text-secondary">
					Emoji
				</button>
			</li>
			<li>
				<button class="w-full rounded px-2 py-1.5 text-left text-sm text-text-muted hover:bg-bg-modifier hover:text-text-secondary">
					AutoMod
				</button>
			</li>
			<li>
				<button class="w-full rounded px-2 py-1.5 text-left text-sm text-text-muted hover:bg-bg-modifier hover:text-text-secondary">
					Audit Log
				</button>
			</li>
		</ul>

		<div class="my-2 border-t border-bg-modifier"></div>
		<button
			class="w-full rounded px-2 py-1.5 text-left text-sm text-text-muted hover:bg-bg-modifier hover:text-text-secondary"
			onclick={() => goto(`/app/guilds/${$page.params.guildId}`)}
		>
			Back to guild
		</button>
	</nav>

	<!-- Settings content -->
	<div class="flex-1 overflow-y-auto bg-bg-tertiary p-8">
		<div class="max-w-xl">
			<h1 class="mb-6 text-xl font-bold text-text-primary">Guild Overview</h1>

			{#if error}
				<div class="mb-4 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>
			{/if}

			<div class="mb-4">
				<label for="guildName" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
					Guild Name
				</label>
				<input id="guildName" type="text" bind:value={name} class="input w-full" maxlength="100" />
			</div>

			<div class="mb-6">
				<label for="guildDesc" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
					Description
				</label>
				<textarea
					id="guildDesc"
					bind:value={description}
					class="input w-full"
					rows="3"
					maxlength="1024"
				></textarea>
			</div>

			<button class="btn-primary" onclick={handleSave} disabled={saving}>
				{saving ? 'Saving...' : 'Save Changes'}
			</button>

			<div class="mt-12 border-t border-bg-modifier pt-6">
				<h2 class="mb-2 text-lg font-semibold text-red-400">Danger Zone</h2>
				<p class="mb-4 text-sm text-text-muted">
					Deleting a guild is permanent and cannot be undone.
				</p>
				<button class="btn-danger" onclick={handleDelete}>Delete Guild</button>
			</div>
		</div>
	</div>
</div>
