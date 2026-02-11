<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import type { Snippet } from 'svelte';
	import { api } from '$lib/api/client';
	import { initAuth, currentUser, isLoading } from '$lib/stores/auth';
	import { connectGateway, disconnectGateway } from '$lib/stores/gateway';
	import GuildSidebar from '$components/layout/GuildSidebar.svelte';
	import ChannelSidebar from '$components/layout/ChannelSidebar.svelte';

	interface Props {
		children: Snippet;
	}

	let { children }: Props = $props();

	onMount(() => {
		initAuth().then(() => {
			const token = api.getToken();
			if (!token) {
				goto('/login');
				return;
			}
			connectGateway(token);
		});

		return () => {
			disconnectGateway();
		};
	});
</script>

{#if $isLoading}
	<div class="flex h-screen items-center justify-center bg-bg-primary">
		<div class="text-center">
			<div class="mx-auto mb-4 h-8 w-8 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
			<p class="text-text-muted">Connecting...</p>
		</div>
	</div>
{:else if $currentUser}
	<div class="flex h-screen overflow-hidden">
		<GuildSidebar />
		<ChannelSidebar />
		<main class="flex min-w-0 flex-1 flex-col">
			{@render children()}
		</main>
	</div>
{/if}
