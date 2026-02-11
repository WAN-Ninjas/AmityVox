<script lang="ts">
	import { currentUser, logout } from '$lib/stores/auth';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import Avatar from '$components/common/Avatar.svelte';

	let displayName = $state('');
	let bio = $state('');
	let statusText = $state('');
	let saving = $state(false);
	let error = $state('');
	let success = $state('');

	$effect(() => {
		if ($currentUser) {
			displayName = $currentUser.display_name ?? '';
			bio = $currentUser.bio ?? '';
			statusText = $currentUser.status_text ?? '';
		}
	});

	async function handleSave() {
		saving = true;
		error = '';
		success = '';

		try {
			const updated = await api.updateMe({
				display_name: displayName || undefined,
				bio: bio || undefined,
				status_text: statusText || undefined
			} as any);
			currentUser.set(updated);
			success = 'Profile updated!';
		} catch (err: any) {
			error = err.message || 'Failed to save';
		} finally {
			saving = false;
		}
	}

	async function handleLogout() {
		await logout();
		goto('/login');
	}
</script>

<svelte:head>
	<title>Settings — AmityVox</title>
</svelte:head>

<div class="flex h-full">
	<!-- Settings sidebar -->
	<nav class="w-52 shrink-0 overflow-y-auto bg-bg-secondary p-4">
		<h3 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">User Settings</h3>
		<ul class="space-y-0.5">
			<li>
				<button class="w-full rounded px-2 py-1.5 text-left text-sm text-text-primary bg-bg-modifier">
					My Account
				</button>
			</li>
			<li>
				<button class="w-full rounded px-2 py-1.5 text-left text-sm text-text-muted hover:bg-bg-modifier hover:text-text-secondary">
					Privacy & Safety
				</button>
			</li>
			<li>
				<button class="w-full rounded px-2 py-1.5 text-left text-sm text-text-muted hover:bg-bg-modifier hover:text-text-secondary">
					Notifications
				</button>
			</li>
			<li>
				<button class="w-full rounded px-2 py-1.5 text-left text-sm text-text-muted hover:bg-bg-modifier hover:text-text-secondary">
					Appearance
				</button>
			</li>
			<li>
				<button class="w-full rounded px-2 py-1.5 text-left text-sm text-text-muted hover:bg-bg-modifier hover:text-text-secondary">
					Sessions
				</button>
			</li>
		</ul>

		<div class="my-2 border-t border-bg-modifier"></div>
		<button
			class="w-full rounded px-2 py-1.5 text-left text-sm text-text-muted hover:bg-bg-modifier hover:text-text-secondary"
			onclick={() => goto('/app')}
		>
			Back
		</button>

		<div class="my-2 border-t border-bg-modifier"></div>
		<button
			class="w-full rounded px-2 py-1.5 text-left text-sm text-red-400 hover:bg-bg-modifier"
			onclick={handleLogout}
		>
			Log Out
		</button>
	</nav>

	<!-- Settings content -->
	<div class="flex-1 overflow-y-auto bg-bg-tertiary p-8">
		<div class="max-w-xl">
			<h1 class="mb-6 text-xl font-bold text-text-primary">My Account</h1>

			{#if $currentUser}
				<!-- Profile card -->
				<div class="mb-8 rounded-lg bg-bg-secondary p-6">
					<div class="flex items-center gap-4">
						<Avatar
							name={$currentUser.display_name ?? $currentUser.username}
							size="lg"
							status={$currentUser.status_presence}
						/>
						<div>
							<h2 class="text-lg font-semibold text-text-primary">
								{$currentUser.display_name ?? $currentUser.username}
							</h2>
							<p class="text-sm text-text-muted">{$currentUser.username}</p>
							{#if $currentUser.email}
								<p class="text-sm text-text-muted">{$currentUser.email}</p>
							{/if}
						</div>
					</div>
				</div>

				{#if error}
					<div class="mb-4 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>
				{/if}
				{#if success}
					<div class="mb-4 rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{success}</div>
				{/if}

				<div class="mb-4">
					<label for="displayName" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
						Display Name
					</label>
					<input id="displayName" type="text" bind:value={displayName} class="input w-full" maxlength="32" />
				</div>

				<div class="mb-4">
					<label for="statusText" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
						Custom Status
					</label>
					<input id="statusText" type="text" bind:value={statusText} class="input w-full" maxlength="128" />
				</div>

				<div class="mb-6">
					<label for="bio" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
						About Me
					</label>
					<textarea id="bio" bind:value={bio} class="input w-full" rows="3" maxlength="190"></textarea>
				</div>

				<button class="btn-primary" onclick={handleSave} disabled={saving}>
					{saving ? 'Saving...' : 'Save Changes'}
				</button>
			{/if}
		</div>
	</div>
</div>
