<script lang="ts">
	import { goto } from '$app/navigation';
	import Modal from '$components/common/Modal.svelte';
	import { api } from '$lib/api/client';
	import { updateGuild } from '$lib/stores/guilds';

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
	let loading = $state(false);

	async function handleCreate() {
		if (!guildName.trim()) return;
		loading = true;
		error = '';

		try {
			const guild = await api.createGuild(guildName.trim());
			updateGuild(guild);
			goto(`/app/guilds/${guild.id}`);
			onclose?.();
		} catch (err: any) {
			error = err.message || 'Failed to create guild';
		} finally {
			loading = false;
		}
	}

	async function handleJoin() {
		if (!inviteCode.trim()) return;
		loading = true;
		error = '';

		try {
			// Extract code from full URL if pasted.
			const code = inviteCode.trim().split('/').pop() ?? inviteCode.trim();
			const guild = await api.acceptInvite(code);
			updateGuild(guild);
			goto(`/app/guilds/${guild.id}`);
			onclose?.();
		} catch (err: any) {
			error = err.message || 'Invalid invite';
		} finally {
			loading = false;
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
		<button class="btn-primary w-full" onclick={handleCreate} disabled={loading || !guildName.trim()}>
			{loading ? 'Creating...' : 'Create Guild'}
		</button>
	{:else}
		<div class="mb-4">
			<label for="inviteInput" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
				Invite Link or Code
			</label>
			<input id="inviteInput" type="text" bind:value={inviteCode} class="input w-full" placeholder="abc123 or https://..." />
		</div>
		<button class="btn-primary w-full" onclick={handleJoin} disabled={loading || !inviteCode.trim()}>
			{loading ? 'Joining...' : 'Join Guild'}
		</button>
	{/if}
</Modal>
