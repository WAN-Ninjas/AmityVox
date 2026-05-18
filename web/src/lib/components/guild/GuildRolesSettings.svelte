<script lang="ts">
	import RoleEditor from '$components/guild/RoleEditor.svelte';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import { getErrorMessage } from '$lib/utils/apiError';
	import { createAsyncOp } from '$lib/utils/asyncOp';
	import type { Role } from '$lib/types';

	interface Props {
		guildId: string;
	}

	let { guildId }: Props = $props();

	let roles = $state<Role[]>([]);
	let loadOp = $state(createAsyncOp());
	let loadedGuildId = $state<string | null>(null);

	$effect(() => {
		if (guildId && loadedGuildId !== guildId && !loadOp.loading) {
			loadRoles();
		}
	});

	async function loadRoles() {
		const result = await loadOp.run(
			() => api.getRoles(guildId),
			msg => addToast(msg, 'error'),
			'Failed to load roles'
		);
		if (result) {
			roles = result;
			loadedGuildId = guildId;
		}
	}
</script>

<h1 class="mb-6 text-xl font-bold text-text-primary">Roles</h1>

{#if loadOp.loading}
	<p class="text-sm text-text-muted">Loading roles...</p>
{:else}
	<RoleEditor
		{guildId}
		bind:roles
		onError={(msg) => addToast(msg, 'error')}
		onSuccess={(msg) => addToast(msg, 'success')}
	/>
{/if}
