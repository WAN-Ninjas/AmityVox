<script lang="ts">
	import { onMount } from 'svelte';
	import { currentUser, logout } from '$lib/stores/auth';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import Avatar from '$components/common/Avatar.svelte';

	type Tab = 'account' | 'privacy' | 'notifications' | 'appearance' | 'sessions';
	let currentTab = $state<Tab>('account');

	// --- Account tab state ---
	let displayName = $state('');
	let bio = $state('');
	let statusText = $state('');
	let saving = $state(false);
	let error = $state('');
	let success = $state('');

	// --- Appearance tab state ---
	let theme = $state<'dark' | 'light'>('dark');
	let fontSize = $state(16);
	let compactMode = $state(false);

	// Load initial values once on mount, not reactively.
	onMount(() => {
		if ($currentUser) {
			displayName = $currentUser.display_name ?? '';
			bio = $currentUser.bio ?? '';
			statusText = $currentUser.status_text ?? '';
		}

		theme = (localStorage.getItem('av-theme') as 'dark' | 'light') ?? 'dark';
		fontSize = parseInt(localStorage.getItem('av-font-size') ?? '16', 10);
		compactMode = localStorage.getItem('av-compact') === 'true';
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

	function saveAppearance() {
		localStorage.setItem('av-theme', theme);
		localStorage.setItem('av-font-size', String(fontSize));
		localStorage.setItem('av-compact', String(compactMode));
		document.documentElement.style.fontSize = `${fontSize}px`;
		success = 'Appearance settings saved!';
		setTimeout(() => (success = ''), 3000);
	}

	async function handleLogout() {
		await logout();
		goto('/login');
	}

	const tabs: { id: Tab; label: string }[] = [
		{ id: 'account', label: 'My Account' },
		{ id: 'privacy', label: 'Privacy & Safety' },
		{ id: 'notifications', label: 'Notifications' },
		{ id: 'appearance', label: 'Appearance' },
		{ id: 'sessions', label: 'Sessions' }
	];

	function themeButtonClass(t: 'dark' | 'light'): string {
		const base = 'rounded-lg border-2 px-4 py-2 text-sm transition-colors';
		if (theme === t) return `${base} border-brand-500 bg-brand-500/10 text-text-primary`;
		return `${base} border-bg-modifier text-text-muted`;
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
			{#if error}
				<div class="mb-4 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>
			{/if}
			{#if success}
				<div class="mb-4 rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{success}</div>
			{/if}

			{#if currentTab === 'account'}
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

			{:else if currentTab === 'privacy'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Privacy & Safety</h1>

				<div class="space-y-6">
					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">Direct Messages</h3>
						<p class="mb-3 text-xs text-text-muted">Control who can send you direct messages.</p>
						<label class="flex items-center gap-2">
							<input type="checkbox" checked class="accent-brand-500" />
							<span class="text-sm text-text-secondary">Allow DMs from guild members</span>
						</label>
					</div>

					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">Friend Requests</h3>
						<p class="mb-3 text-xs text-text-muted">Control who can send you friend requests.</p>
						<label class="flex items-center gap-2">
							<input type="checkbox" checked class="accent-brand-500" />
							<span class="text-sm text-text-secondary">Allow friend requests from everyone</span>
						</label>
					</div>

					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">Blocked Users</h3>
						<p class="text-sm text-text-muted">You haven't blocked anyone.</p>
					</div>
				</div>

			{:else if currentTab === 'notifications'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Notifications</h1>

				<div class="space-y-6">
					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">Desktop Notifications</h3>
						<p class="mb-3 text-xs text-text-muted">Control browser notification preferences.</p>
						<label class="flex items-center gap-2">
							<input type="checkbox" class="accent-brand-500" />
							<span class="text-sm text-text-secondary">Enable desktop notifications</span>
						</label>
					</div>

					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">Message Notifications</h3>
						<p class="mb-3 text-xs text-text-muted">Choose when to be notified about messages.</p>
						<div class="space-y-2">
							<label class="flex items-center gap-2">
								<input type="radio" name="msgNotif" value="all" checked class="accent-brand-500" />
								<span class="text-sm text-text-secondary">All messages</span>
							</label>
							<label class="flex items-center gap-2">
								<input type="radio" name="msgNotif" value="mentions" class="accent-brand-500" />
								<span class="text-sm text-text-secondary">Only mentions</span>
							</label>
							<label class="flex items-center gap-2">
								<input type="radio" name="msgNotif" value="none" class="accent-brand-500" />
								<span class="text-sm text-text-secondary">Nothing</span>
							</label>
						</div>
					</div>

					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">Sounds</h3>
						<label class="flex items-center gap-2">
							<input type="checkbox" checked class="accent-brand-500" />
							<span class="text-sm text-text-secondary">Play notification sounds</span>
						</label>
					</div>
				</div>

			{:else if currentTab === 'appearance'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Appearance</h1>

				<div class="space-y-6">
					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">Theme</h3>
						<p class="mb-3 text-xs text-text-muted">Choose your interface theme.</p>
						<div class="flex gap-3">
							<button class={themeButtonClass('dark')} onclick={() => (theme = 'dark')}>
								Dark
							</button>
							<button class={themeButtonClass('light')} onclick={() => (theme = 'light')}>
								Light
							</button>
						</div>
					</div>

					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">Font Size</h3>
						<p class="mb-3 text-xs text-text-muted">Adjust the base font size ({fontSize}px).</p>
						<input
							type="range"
							min="12"
							max="20"
							bind:value={fontSize}
							class="w-full accent-brand-500"
						/>
						<div class="mt-1 flex justify-between text-xs text-text-muted">
							<span>12px</span>
							<span>16px</span>
							<span>20px</span>
						</div>
					</div>

					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">Compact Mode</h3>
						<label class="flex items-center gap-2">
							<input type="checkbox" bind:checked={compactMode} class="accent-brand-500" />
							<span class="text-sm text-text-secondary">Use compact message layout</span>
						</label>
					</div>

					<button class="btn-primary" onclick={saveAppearance}>Save Appearance</button>
				</div>

			{:else if currentTab === 'sessions'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Sessions</h1>

				<div class="space-y-4">
					<div class="rounded-lg bg-bg-secondary p-4">
						<div class="flex items-center justify-between">
							<div>
								<h3 class="text-sm font-semibold text-text-primary">Current Session</h3>
								<p class="text-xs text-text-muted">This device &middot; Active now</p>
							</div>
							<span class="rounded bg-green-500/10 px-2 py-0.5 text-xs text-green-400">Active</span>
						</div>
					</div>

					<p class="text-sm text-text-muted">
						Session management (view and revoke other sessions) will be available in a future update.
					</p>
				</div>
			{/if}
		</div>
	</div>
</div>
