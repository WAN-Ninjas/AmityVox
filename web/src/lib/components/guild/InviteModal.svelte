<script lang="ts">
	import Modal from '$components/common/Modal.svelte';
	import { api } from '$lib/api/client';
	import { currentGuildId } from '$lib/stores/guilds';
	import type { Invite } from '$lib/types';

	interface Props {
		open?: boolean;
		onclose?: () => void;
	}

	let { open = $bindable(false), onclose }: Props = $props();

	let invite = $state<Invite | null>(null);
	let loading = $state(false);
	let error = $state('');
	let copied = $state(false);

	let maxUses = $state(0);
	let maxAge = $state(86400); // 24 hours default

	async function generateInvite() {
		const guildId = $currentGuildId;
		if (!guildId) return;
		loading = true;
		error = '';
		invite = null;

		try {
			invite = await api.createInvite(guildId, {
				max_uses: maxUses || undefined,
				max_age_seconds: maxAge
			});
		} catch (err: any) {
			error = err.message || 'Failed to create invite';
		} finally {
			loading = false;
		}
	}

	function copyInvite() {
		if (!invite) return;
		const url = `${location.origin}/invite/${invite.code}`;
		navigator.clipboard.writeText(url).then(() => {
			copied = true;
			setTimeout(() => (copied = false), 2000);
		});
	}

	const ageOptions = [
		{ value: 1800, label: '30 minutes' },
		{ value: 3600, label: '1 hour' },
		{ value: 21600, label: '6 hours' },
		{ value: 43200, label: '12 hours' },
		{ value: 86400, label: '24 hours' },
		{ value: 604800, label: '7 days' },
		{ value: 0, label: 'Never' }
	];
</script>

<Modal {open} title="Create Invite" {onclose}>
	{#if error}
		<div class="mb-4 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>
	{/if}

	{#if invite}
		<div class="mb-4">
			<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
				Invite Link
			</label>
			<div class="flex gap-2">
				<input
					type="text"
					class="input flex-1"
					readonly
					value={`${location.origin}/invite/${invite.code}`}
				/>
				<button class="btn-primary" onclick={copyInvite}>
					{copied ? 'Copied!' : 'Copy'}
				</button>
			</div>
			<p class="mt-2 text-xs text-text-muted">
				{invite.max_uses ? `${invite.max_uses} uses max` : 'Unlimited uses'}
				{#if invite.expires_at}
					&middot; Expires {new Date(invite.expires_at).toLocaleDateString()}
				{:else}
					&middot; Never expires
				{/if}
			</p>
		</div>

		<button class="btn-secondary w-full" onclick={() => (invite = null)}>Generate New</button>
	{:else}
		<div class="mb-4">
			<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
				Expire After
			</label>
			<select class="input w-full" bind:value={maxAge}>
				{#each ageOptions as opt}
					<option value={opt.value}>{opt.label}</option>
				{/each}
			</select>
		</div>

		<div class="mb-4">
			<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
				Max Uses (0 = unlimited)
			</label>
			<input type="number" class="input w-full" bind:value={maxUses} min="0" max="1000" />
		</div>

		<div class="flex justify-end gap-2">
			<button class="btn-secondary" onclick={onclose}>Cancel</button>
			<button class="btn-primary" onclick={generateInvite} disabled={loading}>
				{loading ? 'Creating...' : 'Generate Invite'}
			</button>
		</div>
	{/if}
</Modal>
