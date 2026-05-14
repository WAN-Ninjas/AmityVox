<script lang="ts">
	import { logout } from '$lib/stores/auth';
	import { goto } from '$app/navigation';
	import SettingsDataTab from '$lib/components/settings/SettingsDataTab.svelte';
	import SettingsVoiceTab from '$lib/components/settings/SettingsVoiceTab.svelte';
	import SettingsBotsTab from '$lib/components/settings/SettingsBotsTab.svelte';
	import SettingsPrivacyTab from '$lib/components/settings/SettingsPrivacyTab.svelte';
	import SettingsNotificationsTab from '$lib/components/settings/SettingsNotificationsTab.svelte';
	import SettingsAppearanceTab from '$lib/components/settings/SettingsAppearanceTab.svelte';
	import SettingsSecurityTab from '$lib/components/settings/SettingsSecurityTab.svelte';
	import SettingsEncryptionTab from '$lib/components/settings/SettingsEncryptionTab.svelte';
	import SettingsAccountTab from '$lib/components/settings/SettingsAccountTab.svelte';
	import { isDndActive } from '$lib/stores/settings';

	import type { User } from '$lib/types';

	type Tab = 'account' | 'security' | 'notifications' | 'privacy' | 'appearance' | 'voice' | 'encryption' | 'bots' | 'data';
	let currentTab = $state<Tab>('account');
	let importedProfile = $state<User | null>(null);

	async function handleLogout() {
		await logout();
		goto('/login');
	}

	const tabs: { id: Tab; label: string }[] = [
		{ id: 'account', label: 'My Account' },
		{ id: 'security', label: 'Security' },
		{ id: 'notifications', label: 'Notifications' },
		{ id: 'privacy', label: 'Privacy' },
		{ id: 'appearance', label: 'Appearance' },
		{ id: 'voice', label: 'Voice & Video' },
		{ id: 'encryption', label: 'Encryption' },
		{ id: 'bots', label: 'Bots' },
		{ id: 'data', label: 'Data & Privacy' }
	];

	function handleImportedProfile(user: User) {
		importedProfile = user;
	}
</script>

<svelte:head>
	<title>Settings — AmityVox</title>
</svelte:head>

<div class="flex h-full flex-col md:flex-row">
	<!-- Mobile tab selector -->
	<div class="flex items-center gap-2 border-b border-bg-floating bg-bg-secondary px-4 py-2 md:hidden">
		<button
			class="rounded p-1 text-text-muted hover:text-text-primary"
			onclick={() => goto('/app')}
			title="Back"
		>
			<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M15 19l-7-7 7-7" />
			</svg>
		</button>
		<select
			class="flex-1 rounded border border-bg-modifier bg-bg-primary px-2 py-1.5 text-sm text-text-primary outline-none focus:border-brand-500"
			bind:value={currentTab}
		>
			{#each tabs as tab (tab.id)}
				<option value={tab.id}>{tab.label}</option>
			{/each}
		</select>
		<button
			class="rounded p-1 text-red-400 hover:text-red-300"
			onclick={handleLogout}
			title="Log Out"
		>
			<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
			</svg>
		</button>
	</div>

	<!-- Settings sidebar (desktop only) -->
	<nav class="hidden w-52 shrink-0 overflow-y-auto bg-bg-secondary p-4 md:block">
		<h3 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">User Settings</h3>
		<ul class="space-y-0.5">
			{#each tabs as tab (tab.id)}
				<li>
					<button
						class="w-full rounded px-2 py-1.5 text-left text-sm transition-colors {currentTab === tab.id ? 'bg-bg-modifier text-text-primary' : 'text-text-muted hover:bg-bg-modifier hover:text-text-secondary'}"
						onclick={() => (currentTab = tab.id)}
					>
						{tab.label}
						{#if tab.id === 'notifications' && $isDndActive}
							<span class="ml-1 inline-block h-2 w-2 rounded-full bg-status-dnd" title="DND active"></span>
						{/if}
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
	<div class="flex-1 overflow-y-auto bg-bg-tertiary p-4 md:p-8">
		<div class="max-w-xl">
			<!-- ==================== MY ACCOUNT ==================== -->
			{#if currentTab === 'account'}
				<SettingsAccountTab importedProfile={importedProfile} />

			<!-- ==================== SECURITY ==================== -->
			{:else if currentTab === 'security'}
				<SettingsSecurityTab />

			<!-- ==================== NOTIFICATIONS ==================== -->
			{:else if currentTab === 'notifications'}
				<SettingsNotificationsTab />

			<!-- ==================== PRIVACY ==================== -->
			{:else if currentTab === 'privacy'}
				<SettingsPrivacyTab />

			<!-- ==================== APPEARANCE ==================== -->
			{:else if currentTab === 'appearance'}
				<SettingsAppearanceTab />

			<!-- ==================== BOTS ==================== -->
			{:else if currentTab === 'bots'}
				<SettingsBotsTab />

			<!-- ==================== VOICE & VIDEO ==================== -->
			{:else if currentTab === 'voice'}
				<SettingsVoiceTab />

			<!-- ==================== ENCRYPTION ==================== -->
			{:else if currentTab === 'encryption'}
				<SettingsEncryptionTab />

			<!-- ==================== DATA & PRIVACY ==================== -->
			{:else if currentTab === 'data'}
				<SettingsDataTab
					onProfileImported={handleImportedProfile}
					onOpenAccountTab={() => (currentTab = 'account')}
				/>
			{/if}
		</div>
	</div>
</div>
