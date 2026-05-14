<script lang="ts">
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { confirmAction } from '$lib/stores/confirm';
	import Modal from '$components/common/Modal.svelte';
	import type { RegistrationSettings, RegistrationToken } from '$lib/types';

	let regSettings = $state<RegistrationSettings | null>(null);
	let regTokens = $state<RegistrationToken[]>([]);
	let loadingReg = $state(false);
	let savingReg = $state(false);
	let regMode = $state<'open' | 'invite_only' | 'closed'>('open');
	let regMessage = $state('');
	let createTokenModalOpen = $state(false);
	let newTokenMaxUses = $state(1);
	let newTokenNote = $state('');
	let newTokenExpiryHours = $state(0);
	let creatingToken = $state(false);

	$effect(() => {
		if (!regSettings && !loadingReg) {
			loadRegistration();
		}
	});

	async function loadRegistration() {
		loadingReg = true;
		try {
			const [settings, tokens] = await Promise.all([
				api.getRegistrationSettings(),
				api.getRegistrationTokens()
			]);
			regSettings = settings;
			regMode = settings.mode;
			regMessage = settings.message ?? '';
			regTokens = tokens;
		} catch {
			regTokens = [];
		} finally {
			loadingReg = false;
		}
	}

	async function saveRegistration() {
		savingReg = true;
		try {
			regSettings = await api.updateRegistrationSettings({
				mode: regMode,
				message: regMessage || null
			});
			addToast('Registration settings saved', 'success');
		} catch {
			addToast('Failed to save registration settings', 'error');
		} finally {
			savingReg = false;
		}
	}

	function openCreateTokenModal() {
		newTokenMaxUses = 1;
		newTokenNote = '';
		newTokenExpiryHours = 0;
		createTokenModalOpen = true;
	}

	async function handleCreateToken() {
		creatingToken = true;
		try {
			const token = await api.createRegistrationToken({
				max_uses: newTokenMaxUses || undefined,
				note: newTokenNote || undefined,
				expires_in_hours: newTokenExpiryHours || undefined
			});
			regTokens = [...regTokens, token];
			createTokenModalOpen = false;
			addToast('Registration token created', 'success');
		} catch {
			addToast('Failed to create token', 'error');
		} finally {
			creatingToken = false;
		}
	}

	async function handleDeleteToken(tokenId: string) {
		if (!(await confirmAction({ title: 'Delete Registration Token', message: 'Delete this registration token?', confirmLabel: 'Delete' }))) return;
		try {
			await api.deleteRegistrationToken(tokenId);
			regTokens = regTokens.filter((token) => token.id !== tokenId);
			addToast('Token deleted', 'success');
		} catch {
			addToast('Failed to delete token', 'error');
		}
	}

	function copyToken(token: string) {
		navigator.clipboard.writeText(token).then(
			() => addToast('Token copied to clipboard', 'success'),
			() => addToast('Failed to copy token', 'error')
		);
	}

	function getTokenStatus(token: RegistrationToken): string {
		if (token.expires_at && new Date(token.expires_at) < new Date()) return 'expired';
		if (token.max_uses > 0 && token.uses >= token.max_uses) return 'exhausted';
		return 'active';
	}
</script>

<div class="mb-6 flex items-center justify-between">
	<div>
		<h1 class="text-2xl font-bold text-text-primary">Registration Settings</h1>
		<p class="mt-1 text-sm text-text-muted">Control how new users can join this instance.</p>
	</div>
	<button class="btn-primary text-sm" onclick={openCreateTokenModal}>Create Token</button>
</div>

{#if loadingReg}
	<p class="text-sm text-text-muted">Loading registration settings...</p>
{:else}
	<div class="grid gap-6 lg:grid-cols-2">
		<div class="rounded-lg bg-bg-secondary p-4">
			<h3 class="mb-4 font-semibold text-text-primary">Registration Mode</h3>
			<div class="space-y-3">
				{#each ['open', 'invite_only', 'closed'] as mode}
					<label class="flex cursor-pointer items-center gap-3 rounded bg-bg-primary p-3 hover:bg-bg-modifier">
						<input type="radio" bind:group={regMode} value={mode} class="accent-brand-500" />
						<span class="font-medium capitalize text-text-primary">{mode.replace('_', ' ')}</span>
					</label>
				{/each}
			</div>
			<label for="admin-registration-message" class="mt-4 mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Registration Message</label>
			<textarea id="admin-registration-message" class="input min-h-24 w-full" bind:value={regMessage}></textarea>
			<button class="btn-primary mt-4 text-sm" onclick={saveRegistration} disabled={savingReg}>
				{savingReg ? 'Saving...' : 'Save Settings'}
			</button>
		</div>
		<div class="rounded-lg bg-bg-secondary p-4">
			<h3 class="mb-4 font-semibold text-text-primary">Registration Tokens</h3>
			{#if regTokens.length === 0}
				<p class="text-sm text-text-muted">No registration tokens created.</p>
			{:else}
				<div class="space-y-2">
					{#each regTokens as token (token.id)}
						{@const status = getTokenStatus(token)}
						<div class="flex items-center justify-between rounded bg-bg-primary p-3">
							<div class="min-w-0">
								<code class="text-xs text-text-muted">{token.token.slice(0, 16)}...</code>
								<p class="text-xs text-text-muted">{token.uses} / {token.max_uses || 'unlimited'} uses · {status}</p>
								{#if token.note}<p class="text-xs text-text-muted">{token.note}</p>{/if}
							</div>
							<div class="flex gap-2">
								<button class="text-xs text-brand-400 hover:text-brand-300" onclick={() => copyToken(token.token)}>Copy</button>
								<button class="text-xs text-red-400 hover:text-red-300" onclick={() => handleDeleteToken(token.id)}>Delete</button>
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	</div>
{/if}

<Modal open={createTokenModalOpen} title="Create Registration Token" onclose={() => (createTokenModalOpen = false)}>
	<div class="space-y-4">
		<div>
			<label for="admin-token-max-uses" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Max Uses</label>
			<input id="admin-token-max-uses" type="number" class="input w-full" bind:value={newTokenMaxUses} min="1" max="1000" />
			<p class="mt-1 text-xs text-text-muted">How many times this token can be used. Set to 0 for unlimited.</p>
		</div>
		<div>
			<label for="admin-token-note" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Note</label>
			<input id="admin-token-note" type="text" class="input w-full" bind:value={newTokenNote} placeholder="e.g., For team members" maxlength="200" />
		</div>
		<div>
			<label for="admin-token-expiry-hours" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">Expires In (Hours)</label>
			<input id="admin-token-expiry-hours" type="number" class="input w-full" bind:value={newTokenExpiryHours} min="0" max="8760" />
			<p class="mt-1 text-xs text-text-muted">Set to 0 for no expiration.</p>
		</div>
		<div class="flex justify-end gap-2">
			<button class="btn-secondary text-sm" onclick={() => (createTokenModalOpen = false)}>Cancel</button>
			<button class="btn-primary text-sm" onclick={handleCreateToken} disabled={creatingToken}>
				{creatingToken ? 'Creating...' : 'Create Token'}
			</button>
		</div>
	</div>
</Modal>
