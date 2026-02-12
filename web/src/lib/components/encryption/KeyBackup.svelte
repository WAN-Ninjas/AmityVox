<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';

	const API_BASE = '/api/v1';

	async function apiRequest<T>(method: string, path: string, body?: unknown): Promise<T> {
		const headers: Record<string, string> = { 'Content-Type': 'application/json' };
		const token = api.getToken();
		if (token) headers['Authorization'] = `Bearer ${token}`;
		const res = await fetch(`${API_BASE}${path}`, {
			method,
			headers,
			body: body ? JSON.stringify(body) : undefined
		});
		if (res.status === 204) return undefined as T;
		const json = await res.json();
		if (!res.ok) {
			const err = json as { error?: { message?: string; code?: string } };
			throw new ApiRequestError(
				err.error?.message || res.statusText,
				err.error?.code || 'unknown',
				res.status
			);
		}
		return (json as { data: T }).data;
	}

	interface KeyBackupMeta {
		id: string;
		key_count: number;
		version: number;
		created_at: string;
		updated_at: string;
	}

	let backupExists = $state(false);
	let backup = $state<KeyBackupMeta | null>(null);
	let loading = $state(true);
	let error = $state('');

	// Backup creation state
	let passphrase = $state('');
	let passphraseConfirm = $state('');
	let creating = $state(false);

	// Recovery state
	let recoveryPassphrase = $state('');
	let recovering = $state(false);
	let recoveryCodes = $state<string[] | null>(null);
	let showRecoveryCodes = $state(false);
	let generatingCodes = $state(false);

	// Delete state
	let showDeleteConfirm = $state(false);
	let deleting = $state(false);

	$effect(() => {
		loadBackupStatus();
	});

	async function loadBackupStatus() {
		loading = true;
		error = '';
		try {
			const resp = await apiRequest<{ exists: boolean; backup?: KeyBackupMeta }>('GET', '/encryption/key-backup');
			backupExists = resp.exists;
			backup = resp.backup || null;
		} catch (err: any) {
			error = err.message || 'Failed to check backup status';
		} finally {
			loading = false;
		}
	}

	async function createBackup() {
		if (passphrase.length < 12) {
			addToast('Passphrase must be at least 12 characters', 'error');
			return;
		}
		if (passphrase !== passphraseConfirm) {
			addToast('Passphrases do not match', 'error');
			return;
		}

		creating = true;
		try {
			// In a real implementation, this would:
			// 1. Collect all MLS private keys from IndexedDB
			// 2. Derive an AES-256-GCM key from the passphrase via PBKDF2
			// 3. Encrypt the key material
			// 4. Upload to the server
			//
			// For now, we generate a placeholder to demonstrate the flow.
			const salt = new Uint8Array(32);
			crypto.getRandomValues(salt);
			const nonce = new Uint8Array(12);
			crypto.getRandomValues(nonce);

			// Derive key from passphrase.
			const encoder = new TextEncoder();
			const keyMaterial = await crypto.subtle.importKey(
				'raw',
				encoder.encode(passphrase),
				'PBKDF2',
				false,
				['deriveBits', 'deriveKey']
			);

			const key = await crypto.subtle.deriveKey(
				{
					name: 'PBKDF2',
					salt: salt,
					iterations: 600000,
					hash: 'SHA-256'
				},
				keyMaterial,
				{ name: 'AES-GCM', length: 256 },
				false,
				['encrypt']
			);

			// Encrypt placeholder key data.
			const plaintext = encoder.encode(JSON.stringify({
				keys: [],
				created_at: new Date().toISOString(),
				note: 'MLS key backup'
			}));

			const ciphertext = await crypto.subtle.encrypt(
				{ name: 'AES-GCM', iv: nonce },
				key,
				plaintext
			);

			await apiRequest('PUT', '/encryption/key-backup', {
				encrypted_data: Array.from(new Uint8Array(ciphertext)),
				salt: Array.from(salt),
				nonce: Array.from(nonce),
				key_count: 0
			});

			addToast('Key backup created successfully', 'success');
			passphrase = '';
			passphraseConfirm = '';
			await loadBackupStatus();
		} catch (err: any) {
			addToast(err.message || 'Failed to create key backup', 'error');
		} finally {
			creating = false;
		}
	}

	async function downloadBackup() {
		if (!recoveryPassphrase) {
			addToast('Enter your backup passphrase to download', 'error');
			return;
		}

		recovering = true;
		try {
			const resp = await apiRequest<{
				encrypted_data: number[];
				salt: number[];
				nonce: number[];
				key_count: number;
			}>('POST', '/encryption/key-backup/download');

			// Derive key from passphrase.
			const encoder = new TextEncoder();
			const keyMaterial = await crypto.subtle.importKey(
				'raw',
				encoder.encode(recoveryPassphrase),
				'PBKDF2',
				false,
				['deriveBits', 'deriveKey']
			);

			const key = await crypto.subtle.deriveKey(
				{
					name: 'PBKDF2',
					salt: new Uint8Array(resp.salt),
					iterations: 600000,
					hash: 'SHA-256'
				},
				keyMaterial,
				{ name: 'AES-GCM', length: 256 },
				false,
				['decrypt']
			);

			// Decrypt the backup.
			const plaintext = await crypto.subtle.decrypt(
				{ name: 'AES-GCM', iv: new Uint8Array(resp.nonce) },
				key,
				new Uint8Array(resp.encrypted_data)
			);

			const decoder = new TextDecoder();
			const keyData = JSON.parse(decoder.decode(plaintext));

			addToast(`Key backup restored: ${keyData.keys?.length || 0} keys recovered`, 'success');
			recoveryPassphrase = '';
		} catch (err: any) {
			if (err.name === 'OperationError') {
				addToast('Incorrect passphrase. Please try again.', 'error');
			} else {
				addToast(err.message || 'Failed to restore key backup', 'error');
			}
		} finally {
			recovering = false;
		}
	}

	async function generateRecoveryCodes() {
		generatingCodes = true;
		try {
			const resp = await apiRequest<{ codes: string[] }>('POST', '/encryption/key-backup/recovery-codes');
			recoveryCodes = resp.codes;
			showRecoveryCodes = true;
			addToast('Recovery codes generated. Save them somewhere safe!', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to generate recovery codes', 'error');
		} finally {
			generatingCodes = false;
		}
	}

	async function deleteBackup() {
		deleting = true;
		try {
			await apiRequest('DELETE', '/encryption/key-backup');
			backupExists = false;
			backup = null;
			showDeleteConfirm = false;
			addToast('Key backup deleted', 'success');
		} catch (err: any) {
			addToast(err.message || 'Failed to delete key backup', 'error');
		} finally {
			deleting = false;
		}
	}

	let passphraseStrength = $derived(
		passphrase.length < 12
			? 'weak'
			: passphrase.length < 20
				? 'fair'
				: 'strong'
	);

	let strengthColor = $derived(
		passphraseStrength === 'weak'
			? 'bg-red-500'
			: passphraseStrength === 'fair'
				? 'bg-yellow-500'
				: 'bg-green-500'
	);
</script>

<div class="flex flex-col gap-6 rounded-lg border border-bg-modifier bg-bg-secondary p-6">
	<div class="flex items-center gap-3">
		<div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-brand-500/15">
			<svg class="h-5 w-5 text-brand-400" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
			</svg>
		</div>
		<div>
			<h3 class="text-base font-semibold text-text-primary">Key Backup & Recovery</h3>
			<p class="text-xs text-text-muted">
				Back up your encryption keys so you can recover them on a new device.
			</p>
		</div>
	</div>

	{#if loading}
		<div class="flex items-center justify-center py-8">
			<span class="inline-block h-5 w-5 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></span>
		</div>
	{:else if error}
		<div class="rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>
	{:else if backupExists && backup}
		<!-- Backup exists -->
		<div class="rounded-lg bg-bg-primary p-4">
			<div class="flex items-center gap-2">
				<svg class="h-5 w-5 text-status-online" fill="currentColor" viewBox="0 0 20 20">
					<path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
				</svg>
				<span class="text-sm font-medium text-text-primary">Backup Active</span>
			</div>
			<div class="mt-3 grid grid-cols-2 gap-3">
				<div class="rounded bg-bg-secondary px-3 py-2">
					<span class="text-xs text-text-muted">Keys backed up</span>
					<p class="text-sm font-medium text-text-primary">{backup.key_count}</p>
				</div>
				<div class="rounded bg-bg-secondary px-3 py-2">
					<span class="text-xs text-text-muted">Version</span>
					<p class="text-sm font-medium text-text-primary">{backup.version}</p>
				</div>
				<div class="col-span-2 rounded bg-bg-secondary px-3 py-2">
					<span class="text-xs text-text-muted">Last updated</span>
					<p class="text-sm font-medium text-text-primary">{new Date(backup.updated_at).toLocaleString()}</p>
				</div>
			</div>
		</div>

		<!-- Recovery section -->
		<div class="border-t border-bg-modifier pt-4">
			<h4 class="mb-3 text-sm font-semibold text-text-primary">Restore Keys</h4>
			<p class="mb-3 text-xs text-text-muted">
				Enter your backup passphrase to decrypt and restore your encryption keys on this device.
			</p>
			<div class="flex gap-2">
				<input
					type="password"
					class="input flex-1"
					placeholder="Enter your backup passphrase..."
					bind:value={recoveryPassphrase}
					autocomplete="off"
				/>
				<button
					class="btn-primary"
					onclick={downloadBackup}
					disabled={recovering || !recoveryPassphrase}
				>
					{recovering ? 'Restoring...' : 'Restore'}
				</button>
			</div>
		</div>

		<!-- Recovery codes -->
		<div class="border-t border-bg-modifier pt-4">
			<h4 class="mb-2 text-sm font-semibold text-text-primary">Recovery Codes</h4>
			<p class="mb-3 text-xs text-text-muted">
				Generate one-time recovery codes in case you forget your passphrase.
			</p>

			{#if showRecoveryCodes && recoveryCodes}
				<div class="mb-3 rounded-lg bg-bg-primary p-4">
					<p class="mb-2 text-xs font-bold text-red-400">Save these codes! They will not be shown again.</p>
					<div class="grid grid-cols-2 gap-2">
						{#each recoveryCodes as code}
							<div class="rounded bg-bg-secondary px-3 py-2 font-mono text-xs text-text-primary">{code}</div>
						{/each}
					</div>
				</div>
			{/if}

			<button
				class="btn-secondary text-xs"
				onclick={generateRecoveryCodes}
				disabled={generatingCodes}
			>
				{generatingCodes ? 'Generating...' : showRecoveryCodes ? 'Regenerate Codes' : 'Generate Recovery Codes'}
			</button>
		</div>

		<!-- Danger zone -->
		<div class="border-t border-bg-modifier pt-4">
			{#if showDeleteConfirm}
				<div class="rounded-lg border border-red-500/30 bg-red-500/5 p-4">
					<p class="mb-3 text-sm text-red-400">
						Are you sure? Deleting your key backup means you will lose access to encrypted messages if you lose your device.
					</p>
					<div class="flex gap-2">
						<button
							class="rounded bg-red-500 px-4 py-2 text-xs font-medium text-white hover:bg-red-600"
							onclick={deleteBackup}
							disabled={deleting}
						>
							{deleting ? 'Deleting...' : 'Yes, Delete Backup'}
						</button>
						<button class="btn-secondary text-xs" onclick={() => (showDeleteConfirm = false)}>
							Cancel
						</button>
					</div>
				</div>
			{:else}
				<button
					class="text-xs text-red-400 transition-colors hover:text-red-300"
					onclick={() => (showDeleteConfirm = true)}
				>
					Delete Key Backup
				</button>
			{/if}
		</div>
	{:else}
		<!-- No backup exists -->
		<div class="rounded-lg border border-yellow-500/20 bg-yellow-500/5 p-4">
			<div class="flex items-center gap-2">
				<svg class="h-5 w-5 text-yellow-400" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
				</svg>
				<span class="text-sm font-medium text-yellow-400">No Backup Found</span>
			</div>
			<p class="mt-2 text-xs text-text-muted">
				Without a backup, you will lose access to encrypted messages if you lose your device or log out.
			</p>
		</div>

		<!-- Create backup form -->
		<div>
			<h4 class="mb-3 text-sm font-semibold text-text-primary">Create Key Backup</h4>
			<p class="mb-4 text-xs text-text-muted">
				Choose a strong passphrase to encrypt your keys. You will need this passphrase to restore your backup.
			</p>

			<div class="mb-3">
				<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
					Backup Passphrase
				</label>
				<input
					type="password"
					class="input w-full"
					placeholder="Choose a strong passphrase (12+ characters)..."
					bind:value={passphrase}
					autocomplete="new-password"
				/>
				{#if passphrase.length > 0}
					<div class="mt-1.5 flex items-center gap-2">
						<div class="h-1.5 flex-1 overflow-hidden rounded-full bg-bg-modifier">
							<div
								class="h-full rounded-full transition-all {strengthColor}"
								style="width: {passphrase.length < 12 ? 33 : passphrase.length < 20 ? 66 : 100}%"
							></div>
						</div>
						<span class="text-xs text-text-muted capitalize">{passphraseStrength}</span>
					</div>
				{/if}
			</div>

			<div class="mb-4">
				<label class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
					Confirm Passphrase
				</label>
				<input
					type="password"
					class="input w-full"
					placeholder="Confirm your passphrase..."
					bind:value={passphraseConfirm}
					autocomplete="new-password"
				/>
				{#if passphraseConfirm && passphrase !== passphraseConfirm}
					<p class="mt-1 text-xs text-red-400">Passphrases do not match</p>
				{/if}
			</div>

			<button
				class="btn-primary w-full"
				onclick={createBackup}
				disabled={creating || passphrase.length < 12 || passphrase !== passphraseConfirm}
			>
				{creating ? 'Creating Backup...' : 'Create Key Backup'}
			</button>
		</div>
	{/if}
</div>
