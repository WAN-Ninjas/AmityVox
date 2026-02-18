<script lang="ts">
	import { goto } from '$app/navigation';
	import Modal from '$components/common/Modal.svelte';
	import { api } from '$lib/api/client';
	import { updateGuild } from '$lib/stores/guilds';
	import { createAsyncOp } from '$lib/utils/asyncOp';

	interface Props {
		open?: boolean;
		onclose?: () => void;
		initialMode?: 'create' | 'join';
	}

	let { open = $bindable(false), onclose, initialMode = 'create' }: Props = $props();

	let mode = $state<'create' | 'join'>('create');

	$effect(() => {
		if (open) mode = initialMode;
	});
	let guildName = $state('');
	let inviteCode = $state('');
	let error = $state('');
	let createOp = $state(createAsyncOp());
	let joinOp = $state(createAsyncOp());

	async function handleCreate() {
		if (!guildName.trim()) return;
		error = '';
		const guild = await createOp.run(
			() => api.createGuild(guildName.trim()),
			msg => (error = msg)
		);
		if (!createOp.error) {
			updateGuild(guild!);
			goto(`/app/guilds/${guild!.id}`);
			onclose?.();
		}
	}

	async function handleJoin() {
		if (!inviteCode.trim()) return;
		error = '';
		// Extract code from full URL if pasted.
		const code = inviteCode.trim().split('/').pop() ?? inviteCode.trim();
		const guild = await joinOp.run(
			() => api.acceptInvite(code),
			msg => (error = msg)
		);
		if (!joinOp.error) {
			updateGuild(guild!);
			goto(`/app/guilds/${guild!.id}`);
			onclose?.();
		}
	}
</script>

<Modal {open} {onclose} title={mode === 'create' ? 'Create a Guild' : 'Join a Guild'}>
	<div class="mb-4 flex gap-2">
		<button
			class="flex-1 rounded py-2 text-sm font-medium transition-colors"
			class:bg-brand-500={mode === 'create'}
			class:text-white={mode === 'create'}
			class:bg-bg-modifier={mode !== 'create'}
			class:text-text-muted={mode !== 'create'}
			onclick={() => (mode = 'create')}
		>
			Create
		</button>
		<button
			class="flex-1 rounded py-2 text-sm font-medium transition-colors"
			class:bg-brand-500={mode === 'join'}
			class:text-white={mode === 'join'}
			class:bg-bg-modifier={mode !== 'join'}
			class:text-text-muted={mode !== 'join'}
			onclick={() => (mode = 'join')}
		>
			Join
		</button>
	</div>

	{#if error}
		<div class="mb-4 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>
	{/if}

	{#if mode === 'create'}
		<div class="mb-4">
			<label for="guildNameInput" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
				Guild Name
			</label>
			<input id="guildNameInput" type="text" bind:value={guildName} class="input w-full" placeholder="My Guild" maxlength="100" />
		</div>
		<button class="btn-primary w-full" onclick={handleCreate} disabled={createOp.loading || !guildName.trim()}>
			{createOp.loading ? 'Creating...' : 'Create Guild'}
		</button>
	{:else}
		<div class="mb-4">
			<label for="inviteInput" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
				Invite Link or Code
			</label>
			<input id="inviteInput" type="text" bind:value={inviteCode} class="input w-full" placeholder="abc123 or https://..." />
		</div>
		<button class="btn-primary w-full" onclick={handleJoin} disabled={joinOp.loading || !inviteCode.trim()}>
			{joinOp.loading ? 'Joining...' : 'Join Guild'}
		</button>
	{/if}
</Modal>
