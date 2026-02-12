<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import type { Snippet } from 'svelte';
	import { api } from '$lib/api/client';
	import { initAuth, currentUser, isLoading } from '$lib/stores/auth';
	import { connectGateway, disconnectGateway } from '$lib/stores/gateway';
	import { loadSettings, startDndChecker, stopDndChecker, loadSettingsFromApi } from '$lib/stores/settings';
	import { loadBlockedUsers } from '$lib/stores/blocked';
	import GuildSidebar from '$components/layout/GuildSidebar.svelte';
	import ChannelSidebar from '$components/layout/ChannelSidebar.svelte';
	import ToastContainer from '$components/common/ToastContainer.svelte';
	import KeyboardShortcuts from '$components/common/KeyboardShortcuts.svelte';
	import CommandPalette from '$components/common/CommandPalette.svelte';
	import NotificationCenter from '$components/common/NotificationCenter.svelte';
	import AnnouncementBanner from '$components/common/AnnouncementBanner.svelte';

	interface Props {
		children: Snippet;
	}

	let { children }: Props = $props();
	let mobileSidebarOpen = $state(false);
	let commandPaletteOpen = $state(false);
	let notificationCenterOpen = $state(false);

	onMount(() => {
		// Load settings from localStorage immediately.
		loadSettings();
		startDndChecker();

		initAuth().then(() => {
			const token = api.getToken();
			if (!token) {
				goto('/login');
				return;
			}
			connectGateway(token);
			// Also try to sync settings from server.
			loadSettingsFromApi();
			// Load the user's blocked list for message filtering.
			loadBlockedUsers();
		});

		return () => {
			disconnectGateway();
			stopDndChecker();
		};
	});

	function closeMobileSidebar() {
		mobileSidebarOpen = false;
	}

	function toggleCommandPalette() {
		commandPaletteOpen = !commandPaletteOpen;
	}

	function toggleNotificationCenter() {
		notificationCenterOpen = !notificationCenterOpen;
	}
</script>

<KeyboardShortcuts onToggleSearch={toggleCommandPalette} />
<ToastContainer />
<CommandPalette bind:open={commandPaletteOpen} />
<NotificationCenter bind:open={notificationCenterOpen} />

{#if $isLoading}
	<div class="flex h-screen items-center justify-center bg-bg-primary">
		<div class="text-center">
			<div class="mx-auto mb-4 h-8 w-8 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
			<p class="text-text-muted">Connecting...</p>
		</div>
	</div>
{:else if $currentUser}
	<div class="flex h-screen flex-col overflow-hidden bg-bg-primary">
		<!-- Announcement banner at the very top -->
		<AnnouncementBanner />

		<div class="flex min-h-0 flex-1">
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
					<GuildSidebar onToggleNotifications={toggleNotificationCenter} />
					<ChannelSidebar />
				</div>
			</div>

			<main class="flex min-w-0 flex-1 flex-col">
				{@render children()}
			</main>
		</div>
	</div>
{/if}
