<script lang="ts">
	import { page } from '$app/stores';
	import { currentGuild } from '$lib/stores/guilds';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import type { Role } from '$lib/types';

	type Tab = 'overview' | 'roles' | 'emoji' | 'automod' | 'audit';
	let currentTab = $state<Tab>('overview');

	// --- Overview tab state ---
	let name = $state('');
	let description = $state('');
	let saving = $state(false);
	let error = $state('');
	let success = $state('');

	// --- Roles tab state ---
	let roles = $state<Role[]>([]);
	let loadingRoles = $state(false);
	let newRoleName = $state('');
	let creatingRole = $state(false);

	$effect(() => {
		if ($currentGuild) {
			name = $currentGuild.name;
			description = $currentGuild.description ?? '';
		}
	});

	async function loadRoles() {
		if (!$currentGuild) return;
		loadingRoles = true;
		try {
			roles = await api.getRoles($currentGuild.id);
		} catch (err: any) {
			error = err.message || 'Failed to load roles';
		} finally {
			loadingRoles = false;
		}
	}

	// Load roles when switching to the roles tab
	$effect(() => {
		if (currentTab === 'roles' && $currentGuild) {
			loadRoles();
		}
	});

	async function handleSave() {
		if (!$currentGuild) return;
		saving = true;
		error = '';
		success = '';

		try {
			await api.updateGuild($currentGuild.id, { name, description: description || undefined } as any);
			success = 'Guild updated!';
			setTimeout(() => (success = ''), 3000);
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

	async function handleCreateRole() {
		if (!$currentGuild || !newRoleName.trim()) return;
		creatingRole = true;
		error = '';
		try {
			const role = await api.createRole($currentGuild.id, newRoleName.trim());
			roles = [...roles, role];
			newRoleName = '';
		} catch (err: any) {
			error = err.message || 'Failed to create role';
		} finally {
			creatingRole = false;
		}
	}

	const tabs: { id: Tab; label: string }[] = [
		{ id: 'overview', label: 'Overview' },
		{ id: 'roles', label: 'Roles' },
		{ id: 'emoji', label: 'Emoji' },
		{ id: 'automod', label: 'AutoMod' },
		{ id: 'audit', label: 'Audit Log' }
	];
</script>

<svelte:head>
	<title>Guild Settings — AmityVox</title>
</svelte:head>

<div class="flex h-full">
	<!-- Settings sidebar -->
	<nav class="w-52 shrink-0 overflow-y-auto bg-bg-secondary p-4">
		<h3 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Guild Settings</h3>
		<ul class="space-y-0.5">
			{#each tabs as tab (tab.id)}
				<li>
					<button
						class="w-full rounded px-2 py-1.5 text-left text-sm transition-colors"
						class:bg-bg-modifier={currentTab === tab.id}
						class:text-text-primary={currentTab === tab.id}
						class:text-text-muted={currentTab !== tab.id}
						class:hover:bg-bg-modifier={currentTab !== tab.id}
						class:hover:text-text-secondary={currentTab !== tab.id}
						onclick={() => (currentTab = tab.id)}
					>
						{tab.label}
					</button>
				</li>
			{/each}
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
			{#if error}
				<div class="mb-4 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>
			{/if}
			{#if success}
				<div class="mb-4 rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{success}</div>
			{/if}

			{#if currentTab === 'overview'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Guild Overview</h1>

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

			{:else if currentTab === 'roles'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Roles</h1>

				<!-- Create role -->
				<div class="mb-6 flex gap-2">
					<input
						type="text"
						class="input flex-1"
						placeholder="New role name..."
						bind:value={newRoleName}
						maxlength="100"
						onkeydown={(e) => e.key === 'Enter' && handleCreateRole()}
					/>
					<button class="btn-primary" onclick={handleCreateRole} disabled={creatingRole || !newRoleName.trim()}>
						{creatingRole ? 'Creating...' : 'Create Role'}
					</button>
				</div>

				<!-- Role list -->
				{#if loadingRoles}
					<p class="text-sm text-text-muted">Loading roles...</p>
				{:else if roles.length === 0}
					<p class="text-sm text-text-muted">No custom roles yet. Create one above.</p>
				{:else}
					<div class="space-y-2">
						{#each roles as role (role.id)}
							<div class="flex items-center justify-between rounded-lg bg-bg-secondary p-3">
								<div class="flex items-center gap-3">
									<div
										class="h-3 w-3 rounded-full"
										style="background-color: {role.color ?? '#99aab5'}"
									></div>
									<span class="text-sm font-medium text-text-primary">{role.name}</span>
								</div>
								<div class="flex items-center gap-2 text-xs text-text-muted">
									{#if role.hoist}
										<span class="rounded bg-bg-modifier px-1.5 py-0.5">Hoisted</span>
									{/if}
									{#if role.mentionable}
										<span class="rounded bg-bg-modifier px-1.5 py-0.5">Mentionable</span>
									{/if}
									<span>Pos: {role.position}</span>
								</div>
							</div>
						{/each}
					</div>
				{/if}

			{:else if currentTab === 'emoji'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Emoji</h1>

				<div class="rounded-lg bg-bg-secondary p-6 text-center">
					<svg class="mx-auto mb-3 h-12 w-12 text-text-muted" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
						<path d="M15.182 15.182a4.5 4.5 0 0 1-6.364 0M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0ZM9.75 9.75c0 .414-.168.75-.375.75S9 10.164 9 9.75 9.168 9 9.375 9s.375.336.375.75Zm-.375 0h.008v.015h-.008V9.75Zm5.625 0c0 .414-.168.75-.375.75s-.375-.336-.375-.75.168-.75.375-.75.375.336.375.75Zm-.375 0h.008v.015h-.008V9.75Z" />
					</svg>
					<h3 class="mb-1 text-sm font-semibold text-text-primary">Custom Emoji</h3>
					<p class="text-sm text-text-muted">Custom emoji uploads will be available in a future update.</p>
				</div>

			{:else if currentTab === 'automod'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">AutoMod</h1>

				<div class="space-y-4">
					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">Spam Protection</h3>
						<p class="mb-3 text-xs text-text-muted">Automatically detect and filter spam messages.</p>
						<label class="flex items-center gap-2">
							<input type="checkbox" class="accent-brand-500" />
							<span class="text-sm text-text-secondary">Enable spam filter</span>
						</label>
					</div>

					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">Link Filter</h3>
						<p class="mb-3 text-xs text-text-muted">Block messages containing suspicious links.</p>
						<label class="flex items-center gap-2">
							<input type="checkbox" class="accent-brand-500" />
							<span class="text-sm text-text-secondary">Enable link filtering</span>
						</label>
					</div>

					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">Word Filter</h3>
						<p class="mb-3 text-xs text-text-muted">Block messages containing specific words or phrases.</p>
						<textarea class="input w-full" rows="3" placeholder="Enter words separated by commas..."></textarea>
					</div>

					<p class="text-xs text-text-muted">AutoMod configuration persistence will be available in a future update.</p>
				</div>

			{:else if currentTab === 'audit'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Audit Log</h1>

				<div class="rounded-lg bg-bg-secondary p-6 text-center">
					<svg class="mx-auto mb-3 h-12 w-12 text-text-muted" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
						<path d="M19.5 14.25v-2.625a3.375 3.375 0 0 0-3.375-3.375h-1.5A1.125 1.125 0 0 1 13.5 7.125v-1.5a3.375 3.375 0 0 0-3.375-3.375H8.25m0 12.75h7.5m-7.5 3H12M10.5 2.25H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 0 0-9-9Z" />
					</svg>
					<h3 class="mb-1 text-sm font-semibold text-text-primary">Audit Log</h3>
					<p class="text-sm text-text-muted">Audit log viewing will be available in a future update.</p>
				</div>
			{/if}
		</div>
	</div>
</div>
