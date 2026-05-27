<script lang="ts">
	import Modal from '$components/common/Modal.svelte';
	import { api } from '$lib/api/client';
	import { currentGuildId } from '$lib/stores/guilds';
	import { createAsyncOp } from '$lib/utils/asyncOp';
	import { getPublicOrigin } from '$lib/desktop/instances';
	import type { Invite } from '$lib/types';

	interface Props {
		open?: boolean;
		guildId?: string | null;
		onclose?: () => void;
	}

	let { open = $bindable(false), guildId = null, onclose }: Props = $props();

	let invite = $state<Invite | null>(null);
	let error = $state('');
	let createOp = $state(createAsyncOp());
	let copied = $state(false);

	let maxUses = $state(0);
	let maxAge = $state(86400); // 24 hours default

	async function generateInvite() {
		const resolvedGuildId = guildId || $currentGuildId;
		if (!resolvedGuildId) return;
		error = '';
		invite = null;

		const result = await createOp.run(
			() => api.createInvite(resolvedGuildId, {
				max_uses: maxUses || undefined,
				max_age_seconds: maxAge
			}),
			msg => (error = msg),
			'Failed to create invite'
		);
		if (result) invite = result;
	}

	function copyInvite() {
		if (!invite) return;
		const url = `${getPublicOrigin()}/invite/${invite.code}`;
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
			<label for="invite-link" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
				Invite Link
			</label>
			<div class="flex gap-2">
				<input
					id="invite-link"
					type="text"
					class="input flex-1"
					readonly
					value={`${getPublicOrigin()}/invite/${invite.code}`}
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
			<label for="invite-max-age" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
				Expire After
			</label>
			<select id="invite-max-age" class="input w-full" bind:value={maxAge}>
				{#each ageOptions as opt}
					<option value={opt.value}>{opt.label}</option>
				{/each}
			</select>
		</div>

		<div class="mb-4">
			<label for="invite-max-uses" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
				Max Uses (0 = unlimited)
			</label>
			<input id="invite-max-uses" type="number" class="input w-full" bind:value={maxUses} min="0" max="1000" />
		</div>

		<div class="flex justify-end gap-2">
			<button class="btn-secondary" onclick={onclose}>Cancel</button>
			<button class="btn-primary" onclick={generateInvite} disabled={createOp.loading}>
				{createOp.loading ? 'Creating...' : 'Generate Invite'}
			</button>
		</div>
	{/if}
</Modal>
