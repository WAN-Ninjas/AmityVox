<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';

	onMount(() => {
		// Redirect based on auth state.
		if (api.getToken()) {
			goto('/app');
		} else {
			goto('/login');
		}
	});
</script>

<div class="flex h-screen items-center justify-center bg-bg-primary">
	<p class="text-text-muted">Loading...</p>
</div>
