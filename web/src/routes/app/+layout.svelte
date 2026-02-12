<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import type { Snippet } from 'svelte';
	import { api } from '$lib/api/client';
	import { initAuth, currentUser, isLoading } from '$lib/stores/auth';
	import { connectGateway, disconnectGateway } from '$lib/stores/gateway';
	import GuildSidebar from '$components/layout/GuildSidebar.svelte';
	import ChannelSidebar from '$components/layout/ChannelSidebar.svelte';
	import ToastContainer from '$components/common/ToastContainer.svelte';
	import KeyboardShortcuts from '$components/common/KeyboardShortcuts.svelte';

	interface Props {
		children: Snippet;
	}

	let { children }: Props = $props();
	let mobileSidebarOpen = $state(false);

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

	function closeMobileSidebar() {
		mobileSidebarOpen = false;
	}
</script>

<KeyboardShortcuts />
<ToastContainer />

{#if $isLoading}
	<div class="flex h-screen items-center justify-center bg-bg-primary">
		<div class="text-center">
			<div class="mx-auto mb-4 h-8 w-8 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
			<p class="text-text-muted">Connecting...</p>
		</div>
	</div>
{:else if $currentUser}
	<div class="flex h-screen overflow-hidden bg-bg-primary">
		<!-- Mobile hamburger button -->
		<button
			class="fixed left-2 top-2 z-[60] rounded-lg bg-bg-secondary p-2 text-text-muted shadow-lg md:hidden"
			onclick={() => (mobileSidebarOpen = !mobileSidebarOpen)}
			aria-label="Toggle sidebar"
		>
			<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				{#if mobileSidebarOpen}
					<path d="M6 18L18 6M6 6l12 12" />
				{:else}
					<path d="M4 6h16M4 12h16M4 18h16" />
				{/if}
			</svg>
		</button>

		<!-- Mobile backdrop -->
		{#if mobileSidebarOpen}
			<!-- svelte-ignore a11y_no_static_element_interactions -->
			<div
				class="fixed inset-0 z-40 bg-black/50 md:hidden"
				onclick={closeMobileSidebar}
			></div>
		{/if}

		<!-- Sidebars: hidden on mobile unless open -->
		<div class="hidden md:contents" class:!contents={mobileSidebarOpen}>
			<div class="{mobileSidebarOpen ? 'fixed inset-y-0 left-0 z-50 flex' : 'contents'}">
				<GuildSidebar />
				<ChannelSidebar />
			</div>
		</div>

		<main class="flex min-w-0 flex-1 flex-col">
			{@render children()}
		</main>
	</div>
{/if}
