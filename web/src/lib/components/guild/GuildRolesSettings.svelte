<script lang="ts">
	import RoleEditor from '$components/guild/RoleEditor.svelte';
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';
	import type { Role } from '$lib/types';

	interface Props {
		guildId: string;
	}

	let { guildId }: Props = $props();

	let roles = $state<Role[]>([]);
	let loadingRoles = $state(false);
	let loadedGuildId = $state<string | null>(null);

	$effect(() => {
		if (guildId && loadedGuildId !== guildId && !loadingRoles) {
			loadRoles();
		}
	});

	async function loadRoles() {
		loadingRoles = true;
		try {
			roles = await api.getRoles(guildId);
			loadedGuildId = guildId;
		} catch (err: any) {
			addToast(err.message || 'Failed to load roles', 'error');
		} finally {
			loadingRoles = false;
		}
	}
</script>

<h1 class="mb-6 text-xl font-bold text-text-primary">Roles</h1>

{#if loadingRoles}
	<p class="text-sm text-text-muted">Loading roles...</p>
{:else}
	<RoleEditor
		{guildId}
		bind:roles
		onError={(msg) => addToast(msg, 'error')}
		onSuccess={(msg) => addToast(msg, 'success')}
	/>
{/if}
