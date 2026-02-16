<script lang="ts">
	import type { Snippet } from 'svelte';
	import { currentUser } from '$lib/stores/auth';

	interface Props {
		children: Snippet;
	}

	let { children }: Props = $props();

	const isAdmin = $derived(($currentUser?.flags ?? 0) & 4);
</script>

{#if isAdmin}
	<div class="flex h-full bg-bg-tertiary">
		{@render children()}
	</div>
{:else}
	<div class="flex h-full items-center justify-center bg-bg-tertiary">
		<div class="text-center">
			<h1 class="mb-2 text-2xl font-bold text-text-primary">Access Denied</h1>
			<p class="text-sm text-text-muted">You don't have permission to view the admin panel.</p>
			<a href="/app" class="mt-4 inline-block text-sm text-brand-400 hover:text-brand-300">Back to app</a>
		</div>
	</div>
{/if}
