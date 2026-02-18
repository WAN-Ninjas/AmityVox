<script lang="ts">
	import { api } from '$lib/api/client';
	import { e2ee } from '$lib/encryption/e2eeManager';
	import { addToast } from '$lib/stores/toast';
	import { updateChannel as updateChannelStore } from '$lib/stores/channels';
	import { createAsyncOp } from '$lib/utils/asyncOp';

	interface Props {
		channelId: string;
		encrypted: boolean;
		onchange?: () => void;
	}

	let { channelId, encrypted, onchange }: Props = $props();

	// --- State ---
	let passphrase = $state('');
	let passphraseConfirm = $state('');
	let unlockPassphrase = $state('');
	let hasKey = $state(false);
	let checkingKey = $state(true);
	let enableOp = $state(createAsyncOp());
	let unlockOp = $state(createAsyncOp());
	let disableOp = $state(createAsyncOp());
	let showPassphraseResult = $state('');
	let showChangePassphrase = $state(false);
	let newPassphrase = $state('');
	let newPassphraseConfirm = $state('');
	let changePassphraseOp = $state(createAsyncOp());
	let showDisableOptions = $state(false);
	let decryptProgress = $state<{ current: number; total: number } | null>(null);

	let keyCheckToken = 0;
	// Check if we have a key when channelId changes
	$effect(() => {
		const id = channelId;
		const token = ++keyCheckToken;
		checkingKey = true;
		hasKey = false;
		e2ee.hasChannelKey(id).then((has) => {
			if (token !== keyCheckToken) return;
			hasKey = has;
		}).finally(() => {
			if (token !== keyCheckToken) return;
			checkingKey = false;
		});
	});

	async function handleEnable() {
		if (!passphrase.trim()) {
			addToast('Enter a passphrase', 'error');
			return;
		}
		if (passphrase !== passphraseConfirm) {
			addToast('Passphrases do not match', 'error');
			return;
		}

		const pp = passphrase;
		await enableOp.run(async () => {
			// Derive and store the key
			await e2ee.setPassphrase(channelId, pp);
			// Enable encryption on the server
			await api.updateChannel(channelId, { encrypted: true } as any);
		}, msg => addToast(msg, 'error'));
		if (!enableOp.error) {
			// Show the passphrase one time
			showPassphraseResult = pp;
			hasKey = true;
			passphrase = '';
			passphraseConfirm = '';
			onchange?.();
		}
	}

	async function handleUnlock() {
		if (!unlockPassphrase.trim()) {
			addToast('Enter the channel passphrase', 'error');
			return;
		}

		await unlockOp.run(
			() => e2ee.setPassphrase(channelId, unlockPassphrase),
			msg => addToast(msg, 'error')
		);
		if (!unlockOp.error) {
			hasKey = true;
			unlockPassphrase = '';
			addToast('Channel unlocked', 'success');
		}
	}

	async function handleChangePassphrase() {
		if (!newPassphrase.trim()) {
			addToast('Enter a new passphrase', 'error');
			return;
		}
		if (newPassphrase !== newPassphraseConfirm) {
			addToast('Passphrases do not match', 'error');
			return;
		}

		await changePassphraseOp.run(
			() => e2ee.setPassphrase(channelId, newPassphrase),
			msg => addToast(msg, 'error')
		);
		if (!changePassphraseOp.error) {
			showChangePassphrase = false;
			newPassphrase = '';
			newPassphraseConfirm = '';
			addToast('Passphrase changed. Only future messages will use the new key.', 'success');
		}
	}

	async function handleDisableKeepEncrypted() {
		await disableOp.run(async () => {
			await api.updateChannel(channelId, { encrypted: false } as any);
			await e2ee.clearChannelKey(channelId);
		}, msg => addToast(msg, 'error'));
		if (!disableOp.error) {
			hasKey = false;
			showDisableOptions = false;
			addToast('Encryption disabled. Old messages remain encrypted.', 'success');
			onchange?.();
		}
	}

	async function handleDisableDecryptAll() {
		decryptProgress = { current: 0, total: 0 };
		const chId = channelId;
		await disableOp.run(async () => {
			// Fetch all encrypted messages and decrypt them in batches.
			// NOTE: Messages created during this loop may be missed since encryption
			// remains enabled until the sweep completes. This is an accepted limitation.
			let before = '';
			let totalDecrypted = 0;

			while (true) {
				const params: Record<string, string> = { limit: '100' };
				if (before) params.before = before;
				const messages = await api.getMessages(chId, params);

				const encrypted = messages.filter((m: any) => m.encrypted && m.content);
				if (encrypted.length === 0 && messages.length === 0) break;

				if (encrypted.length > 0) {
					const decrypted: { id: string; content: string }[] = [];
					for (const msg of encrypted) {
						try {
							const plaintext = await e2ee.decryptMessage(chId, msg.content);
							decrypted.push({ id: msg.id, content: plaintext });
						} catch {
							// Skip messages we can't decrypt (wrong key)
						}
					}

					if (decrypted.length > 0) {
						await api.batchDecryptMessages(chId, decrypted);
						totalDecrypted += decrypted.length;
						decryptProgress = { current: totalDecrypted, total: totalDecrypted };
					}
				}

				if (messages.length < 100) break;
				before = messages[messages.length - 1].id;
			}

			// Disable encryption on the channel
			await api.updateChannel(chId, { encrypted: false } as any);
			await e2ee.clearChannelKey(chId);

			hasKey = false;
			showDisableOptions = false;
			decryptProgress = null;
			addToast(`Encryption disabled. ${totalDecrypted} message(s) decrypted.`, 'success');
			onchange?.();
		}, msg => {
			addToast(msg, 'error');
			decryptProgress = null;
		});
	}

	function copyPassphrase() {
		navigator.clipboard.writeText(showPassphraseResult);
		addToast('Passphrase copied to clipboard', 'success');
	}
</script>

<div class="flex flex-col gap-4 rounded-lg border border-bg-modifier bg-bg-secondary p-4">
	<!-- Encryption status -->
	<div class="flex items-center gap-3">
		{#if encrypted}
			<div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-status-online/15">
				<svg class="h-5 w-5 text-status-online" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
				</svg>
			</div>
			<div>
				<p class="text-sm font-semibold text-text-primary">End-to-End Encrypted</p>
				<p class="text-xs text-text-muted">Messages are encrypted with a shared passphrase.</p>
			</div>
		{:else}
			<div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-bg-modifier">
				<svg class="h-5 w-5 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M8 11V7a4 4 0 118 0m-4 8v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2z" />
				</svg>
			</div>
			<div>
				<p class="text-sm font-semibold text-text-primary">Not Encrypted</p>
				<p class="text-xs text-text-muted">Messages in this channel are not end-to-end encrypted.</p>
			</div>
		{/if}
	</div>

	{#if showPassphraseResult}
		<!-- One-time passphrase display -->
		<div class="border-t border-bg-modifier pt-4">
			<div class="rounded-lg border border-yellow-500/30 bg-yellow-500/5 p-4">
				<div class="mb-2 flex items-center gap-2">
					<svg class="h-4 w-4 text-yellow-400" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
					</svg>
					<p class="text-sm font-semibold text-yellow-400">Save Your Passphrase</p>
				</div>
				<p class="mb-3 text-xs text-text-muted">
					Share this passphrase with channel members securely. It will not be shown again.
				</p>
				<div class="flex items-center gap-2">
					<code class="flex-1 rounded bg-bg-primary px-3 py-2 font-mono text-sm text-text-primary">
						{showPassphraseResult}
					</code>
					<button
						class="shrink-0 rounded bg-brand-500 px-3 py-2 text-xs font-medium text-white hover:bg-brand-600"
						onclick={copyPassphrase}
					>
						Copy
					</button>
				</div>
				<button
					class="mt-3 text-xs text-text-muted hover:text-text-primary"
					onclick={() => (showPassphraseResult = '')}
				>
					I've saved it, dismiss
				</button>
			</div>
		</div>
	{/if}

	{#if encrypted}
		<div class="border-t border-bg-modifier"></div>

		{#if checkingKey}
			<div class="flex items-center gap-2 text-xs text-text-muted">
				<span class="inline-block h-3 w-3 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></span>
				Checking encryption key...
			</div>
		{:else if !hasKey}
			<!-- Passphrase unlock prompt -->
			<div>
				<h4 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Unlock Channel</h4>
				<p class="mb-3 text-xs text-text-muted">
					Enter the passphrase to decrypt messages in this channel.
				</p>
				<div class="flex gap-2">
					<input
						type="password"
						class="flex-1 rounded-md border border-bg-modifier bg-bg-primary px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-brand-500 focus:outline-none"
						placeholder="Channel passphrase"
						bind:value={unlockPassphrase}
						onkeydown={(e) => e.key === 'Enter' && handleUnlock()}
					/>
					<button
						class="shrink-0 rounded-md bg-brand-500 px-4 py-2 text-xs font-medium text-white hover:bg-brand-600 disabled:opacity-50"
						onclick={handleUnlock}
						disabled={unlockOp.loading || !unlockPassphrase.trim()}
					>
						{unlockOp.loading ? 'Unlocking...' : 'Unlock'}
					</button>
				</div>
			</div>
		{:else}
			<!-- Channel is unlocked -->
			<div class="flex items-center gap-2 rounded bg-status-online/10 px-3 py-2">
				<svg class="h-4 w-4 text-status-online" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M5 13l4 4L19 7" />
				</svg>
				<span class="text-xs text-status-online">You have the encryption key for this channel</span>
			</div>

			<!-- Change passphrase -->
			{#if !showChangePassphrase}
				<button
					class="text-xs text-text-muted hover:text-text-primary"
					onclick={() => (showChangePassphrase = true)}
				>
					Change passphrase...
				</button>
			{:else}
				<div class="rounded-lg border border-bg-modifier bg-bg-primary p-3">
					<h4 class="mb-1 text-xs font-bold uppercase tracking-wide text-text-muted">Change Passphrase</h4>
					<p class="mb-3 text-xs text-yellow-400">
						Only future messages will use the new key. Old messages still need the previous passphrase.
					</p>
					<input
						type="password"
						class="mb-2 w-full rounded-md border border-bg-modifier bg-bg-secondary px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-brand-500 focus:outline-none"
						placeholder="New passphrase"
						bind:value={newPassphrase}
					/>
					<input
						type="password"
						class="mb-3 w-full rounded-md border border-bg-modifier bg-bg-secondary px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-brand-500 focus:outline-none"
						placeholder="Confirm new passphrase"
						bind:value={newPassphraseConfirm}
						onkeydown={(e) => e.key === 'Enter' && handleChangePassphrase()}
					/>
					<div class="flex gap-2">
						<button
							class="rounded-md bg-brand-500 px-4 py-2 text-xs font-medium text-white hover:bg-brand-600 disabled:opacity-50"
							onclick={handleChangePassphrase}
							disabled={changePassphraseOp.loading || !newPassphrase.trim()}
						>
							{changePassphraseOp.loading ? 'Changing...' : 'Change'}
						</button>
						<button
							class="rounded-md px-4 py-2 text-xs text-text-muted hover:text-text-primary"
							onclick={() => { showChangePassphrase = false; newPassphrase = ''; newPassphraseConfirm = ''; }}
						>
							Cancel
						</button>
					</div>
				</div>
			{/if}

			<div class="border-t border-bg-modifier"></div>

			<!-- Disable encryption -->
			{#if !showDisableOptions}
				<button
					class="text-xs text-red-400 hover:text-red-300"
					onclick={() => (showDisableOptions = true)}
				>
					Disable encryption...
				</button>
			{:else}
				<div class="rounded-lg border border-red-500/20 bg-red-500/5 p-3">
					<h4 class="mb-2 text-xs font-bold uppercase tracking-wide text-red-400">Disable Encryption</h4>

					{#if decryptProgress}
						<div class="mb-3">
							<div class="mb-1 text-xs text-text-muted">
								Decrypting messages... {decryptProgress.current} processed
							</div>
							<div class="h-1.5 w-full overflow-hidden rounded-full bg-bg-modifier">
								<div class="h-full rounded-full bg-brand-500 transition-all" style="width: 100%"></div>
							</div>
						</div>
					{:else}
						<div class="flex flex-col gap-2">
							<button
								class="rounded-md bg-red-500 px-4 py-2 text-xs font-medium text-white hover:bg-red-600 disabled:opacity-50"
								onclick={handleDisableDecryptAll}
								disabled={disableOp.loading}
							>
								Decrypt all messages & turn off
							</button>
							<button
								class="rounded-md border border-red-500/30 px-4 py-2 text-xs font-medium text-red-400 hover:bg-red-500/10 disabled:opacity-50"
								onclick={handleDisableKeepEncrypted}
								disabled={disableOp.loading}
							>
								Turn off (keep old messages encrypted)
							</button>
							<button
								class="text-xs text-text-muted hover:text-text-primary"
								onclick={() => (showDisableOptions = false)}
								disabled={disableOp.loading}
							>
								Cancel
							</button>
						</div>
					{/if}
				</div>
			{/if}
		{/if}

	{:else}
		<!-- Enable encryption -->
		<div class="border-t border-bg-modifier pt-4">
			<h4 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Enable Encryption</h4>
			<p class="mb-3 text-xs text-text-muted">
				Set a passphrase to encrypt all future messages. Share the passphrase with channel members out-of-band.
			</p>
			<input
				type="password"
				class="mb-2 w-full rounded-md border border-bg-modifier bg-bg-primary px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-brand-500 focus:outline-none"
				placeholder="Enter a passphrase"
				bind:value={passphrase}
			/>
			<input
				type="password"
				class="mb-3 w-full rounded-md border border-bg-modifier bg-bg-primary px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-brand-500 focus:outline-none"
				placeholder="Confirm passphrase"
				bind:value={passphraseConfirm}
				onkeydown={(e) => e.key === 'Enter' && handleEnable()}
			/>
			<button
				class="flex items-center gap-2 rounded-md bg-status-online px-4 py-2 text-xs font-medium text-white hover:opacity-90 disabled:opacity-50"
				onclick={handleEnable}
				disabled={enableOp.loading || !passphrase.trim() || !passphraseConfirm.trim()}
			>
				{#if enableOp.loading}
					<span class="inline-block h-3 w-3 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
					Enabling...
				{:else}
					<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
					</svg>
					Enable Encryption
				{/if}
			</button>
		</div>
	{/if}
</div>
