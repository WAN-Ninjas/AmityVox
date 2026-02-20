<script lang="ts">
	import { onMount } from 'svelte';
	import { goto, afterNavigate } from '$app/navigation';
	import { page } from '$app/stores';
	import type { Snippet } from 'svelte';
	import { api } from '$lib/api/client';
	import { initAuth, currentUser, isLoading } from '$lib/stores/auth';
	import { connectGateway, disconnectGateway, gatewayConnected } from '$lib/stores/gateway';
	import { loadSettings, startDndChecker, stopDndChecker, loadSettingsFromApi } from '$lib/stores/settings';
	import { loadBlockedUsers } from '$lib/stores/blocked';
	import { pushChannel, setCurrentUrl } from '$lib/stores/navigation';
	import GuildSidebar from '$components/layout/GuildSidebar.svelte';
	import ChannelSidebar from '$components/layout/ChannelSidebar.svelte';
	import ToastContainer from '$components/common/ToastContainer.svelte';
	import KeyboardShortcuts from '$components/common/KeyboardShortcuts.svelte';
	import CommandPalette from '$components/common/CommandPalette.svelte';
	import QuickSwitcher from '$components/common/QuickSwitcher.svelte';
	import AnnouncementBanner from '$components/common/AnnouncementBanner.svelte';
	import ModerationModals from '$components/common/ModerationModals.svelte';
	import IncomingCallModal from '$components/common/IncomingCallModal.svelte';
	import ResizeHandle from '$components/common/ResizeHandle.svelte';
	import { channelSidebarWidth } from '$lib/stores/layout';

	interface Props {
		children: Snippet;
	}

	let { children }: Props = $props();
	let mobileSidebarOpen = $state(false);
	let commandPaletteOpen = $state(false);
	let quickSwitcherOpen = $state(false);

	// Track route changes for navigation history.
	$effect(() => {
		const url = $page.url.pathname;
		const params = $page.params;

		// Detect channel navigation and record it.
		if (params.channelId) {
			const guildId = params.guildId ?? null;
			pushChannel(guildId, params.channelId);
		} else if (url.includes('/app/')) {
			// Non-channel pages: just set current URL without pushing to recent channels.
			setCurrentUrl(url);
		}
	});

	// Auto-close mobile sidebar on navigation.
	$effect(() => {
		$page.url.pathname;
		mobileSidebarOpen = false;
	});

	afterNavigate(() => {
		if (typeof window !== 'undefined' && window.location.pathname !== '/app') {
			window.history.replaceState(history.state, '', '/app');
		}
	});

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

	function toggleQuickSwitcher() {
		quickSwitcherOpen = !quickSwitcherOpen;
	}

</script>

<KeyboardShortcuts onToggleSearch={toggleCommandPalette} onToggleQuickSwitcher={toggleQuickSwitcher} />
<ToastContainer />
<ModerationModals />
<IncomingCallModal />
<CommandPalette bind:open={commandPaletteOpen} />
<QuickSwitcher bind:open={quickSwitcherOpen} />

{#if $isLoading}
	<div class="flex h-screen items-center justify-center bg-bg-primary">
		<div class="text-center">
			<div class="mx-auto mb-4 h-8 w-8 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
			<p class="text-text-muted">Connecting...</p>
		</div>
	</div>
{:else if $currentUser}
	<div class="flex h-screen flex-col overflow-hidden bg-bg-primary" oncontextmenu={(e) => { if (e.button === 2) e.preventDefault(); }}>
		<div class="accent-stripe"></div>
		<!-- Reconnecting banner -->
		{#if !$gatewayConnected}
			<div class="flex items-center justify-center gap-2 bg-red-500 px-3 py-1.5 text-sm font-medium text-white">
				<div class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"></div>
				Reconnecting...
			</div>
		{/if}
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
					<GuildSidebar />
					{#if !$page.url.pathname.startsWith('/app/admin')}
						<ChannelSidebar width={$channelSidebarWidth} />
					{/if}
				</div>
			</div>

			<!-- Resize handle between sidebar and main content -->
			{#if !mobileSidebarOpen && !$page.url.pathname.startsWith('/app/admin')}
				<div class="hidden md:flex">
					<ResizeHandle
						width={$channelSidebarWidth}
						onresize={(w) => channelSidebarWidth.set(w)}
						onreset={() => channelSidebarWidth.reset()}
						side="left"
					/>
				</div>
			{/if}

			<main class="flex min-w-0 flex-1 flex-col">
				{@render children()}
			</main>
		</div>
	</div>
{/if}
