<script lang="ts">
	import { currentUser } from '$lib/stores/auth';
	import { api } from '$lib/api/client';
	import { getErrorMessage } from '$lib/utils/apiError';
	import type { User } from '$lib/types';

	interface Props {
		onProfileImported?: (user: User) => void;
		onOpenAccountTab?: () => void;
	}

	let { onProfileImported, onOpenAccountTab }: Props = $props();

	let exportingData = $state(false);
	let exportDataSuccess = $state('');
	let exportDataError = $state('');
	let exportingAccount = $state(false);
	let exportAccountSuccess = $state('');
	let exportAccountError = $state('');
	let importingAccount = $state(false);
	let importAccountSuccess = $state('');
	let importAccountError = $state('');
	let accountImportFileInput = $state<HTMLInputElement>();

	function downloadJson(data: unknown, filename: string) {
		const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = filename;
		a.click();
		URL.revokeObjectURL(url);
	}

	async function handleExportData() {
		exportingData = true;
		exportDataSuccess = '';
		exportDataError = '';
		try {
			const exportData = await api.exportUserData();
			downloadJson(exportData, `amityvox-data-export-${new Date().toISOString().slice(0, 10)}.json`);
			exportDataSuccess = 'Data exported successfully! Check your downloads.';
			setTimeout(() => (exportDataSuccess = ''), 5000);
		} catch (err: unknown) {
			exportDataError = getErrorMessage(err, 'Failed to export data');
		} finally {
			exportingData = false;
		}
	}

	async function handleExportAccount() {
		exportingAccount = true;
		exportAccountSuccess = '';
		exportAccountError = '';
		try {
			const exportData = await api.exportAccount();
			downloadJson(exportData, `amityvox-account-export-${new Date().toISOString().slice(0, 10)}.json`);
			exportAccountSuccess = 'Account exported successfully! Check your downloads.';
			setTimeout(() => (exportAccountSuccess = ''), 5000);
		} catch (err: unknown) {
			exportAccountError = getErrorMessage(err, 'Failed to export account');
		} finally {
			exportingAccount = false;
		}
	}

	async function handleImportAccount() {
		if (!accountImportFileInput?.files?.length) {
			importAccountError = 'Please select a file to import.';
			return;
		}
		const file = accountImportFileInput.files[0];
		if (file.size > 5 * 1024 * 1024) {
			importAccountError = 'File too large (max 5 MB).';
			return;
		}

		importingAccount = true;
		importAccountSuccess = '';
		importAccountError = '';
		try {
			const text = await file.text();
			const imported = JSON.parse(text);

			const payload: Record<string, unknown> = {};
			if (imported.profile) payload.profile = imported.profile;
			if (imported.settings) payload.settings = imported.settings;

			const user = await api.importAccount(payload);
			currentUser.set(user);
			onProfileImported?.(user);
			importAccountSuccess = 'Account data imported successfully! Profile updated.';
			setTimeout(() => (importAccountSuccess = ''), 5000);
		} catch (err: unknown) {
			if (err instanceof SyntaxError) {
				importAccountError = 'Invalid JSON file. Please select a valid AmityVox account export.';
			} else {
				importAccountError = getErrorMessage(err, 'Failed to import account');
			}
		} finally {
			importingAccount = false;
			if (accountImportFileInput) accountImportFileInput.value = '';
		}
	}
</script>

<h1 class="mb-6 text-xl font-bold text-text-primary">Data & Privacy</h1>

<div class="mb-6 rounded-lg bg-bg-secondary p-4">
	<h3 class="mb-1 text-sm font-semibold text-text-primary">Export My Data</h3>
	<p class="mb-3 text-xs text-text-muted">
		Download a copy of all your data including your profile, messages, server memberships,
		bookmarks, reactions, read states, and relationships. This complies with GDPR data
		portability requirements. Limited to one export per 24 hours.
	</p>
	{#if exportDataSuccess}
		<div class="mb-3 rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{exportDataSuccess}</div>
	{/if}
	{#if exportDataError}
		<div class="mb-3 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{exportDataError}</div>
	{/if}
	<button class="btn-primary" onclick={handleExportData} disabled={exportingData}>
		{#if exportingData}
			<span class="flex items-center gap-2">
				<span class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
				Exporting...
			</span>
		{:else}
			Export My Data
		{/if}
	</button>
</div>

<div class="mb-6 rounded-lg bg-bg-secondary p-4">
	<h3 class="mb-1 text-sm font-semibold text-text-primary">Account Migration</h3>
	<p class="mb-3 text-xs text-text-muted">
		Export your account profile and settings for migration to another AmityVox instance,
		or import data from another instance. Messages are not included as they belong to
		the originating instance.
	</p>

	{#if exportAccountSuccess}
		<div class="mb-3 rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{exportAccountSuccess}</div>
	{/if}
	{#if exportAccountError}
		<div class="mb-3 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{exportAccountError}</div>
	{/if}
	{#if importAccountSuccess}
		<div class="mb-3 rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{importAccountSuccess}</div>
	{/if}
	{#if importAccountError}
		<div class="mb-3 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{importAccountError}</div>
	{/if}

	<div class="flex flex-wrap items-center gap-3">
		<button class="btn-primary" onclick={handleExportAccount} disabled={exportingAccount}>
			{#if exportingAccount}
				<span class="flex items-center gap-2">
					<span class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
					Exporting...
				</span>
			{:else}
				Export Account
			{/if}
		</button>

		<div class="flex items-center gap-2">
			<input
				bind:this={accountImportFileInput}
				type="file"
				accept=".json"
				class="text-xs text-text-muted file:mr-2 file:cursor-pointer file:rounded file:border-0 file:bg-bg-modifier file:px-3 file:py-1.5 file:text-xs file:text-text-primary hover:file:bg-bg-modifier/80"
			/>
			<button class="btn-secondary text-sm" onclick={handleImportAccount} disabled={importingAccount}>
				{importingAccount ? 'Importing...' : 'Import Account'}
			</button>
		</div>
	</div>
</div>

<div class="rounded-lg bg-bg-secondary p-4">
	<h3 class="mb-1 text-sm font-semibold text-text-primary">About Your Data</h3>
	<div class="space-y-2 text-xs text-text-muted">
		<p>
			<span class="font-semibold text-text-secondary">Full data export</span> includes
			everything: your profile, messages (up to 10,000 most recent), server memberships with
			roles, bookmarks, reactions, read states, and friend/block relationships.
		</p>
		<p>
			<span class="font-semibold text-text-secondary">Account migration export</span> includes
			only your profile settings (display name, bio, status, pronouns, accent color) and
			client settings. Messages remain on the originating instance.
		</p>
		<p>
			<span class="font-semibold text-text-secondary">Account import</span> applies the
			profile and settings from an export file to your current account. It will not change
			your username, email, or password.
		</p>
		<p>
			To permanently delete your account and all associated data, use the
			<button class="text-red-400 underline hover:text-red-300" onclick={onOpenAccountTab}>
				My Account
			</button>
			tab.
		</p>
	</div>
</div>
