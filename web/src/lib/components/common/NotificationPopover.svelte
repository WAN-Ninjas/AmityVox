<script lang="ts">
	import { goto } from '$app/navigation';
	import {
		notifications,
		unreadNotificationCount,
		markAllNotificationsRead,
		markNotificationRead,
		lastNotificationSuppressed,
	} from '$lib/stores/notifications';
	import { isDndActive } from '$lib/stores/settings';
	import { getNotificationNavigationUrl } from '$lib/utils/notificationHelpers';
	import NotificationItem from './NotificationItem.svelte';
	import type { ServerNotification } from '$lib/types';

	interface Props {
		open?: boolean;
		onclose?: () => void;
	}

	let { open = $bindable(false), onclose }: Props = $props();

	// Limit popover to latest 20 notifications.
	const popoverNotifications = $derived($notifications.slice(0, 20));

	function close() {
		open = false;
		onclose?.();
	}

	function handleNotificationClick(n: ServerNotification) {
		if (!n.read) markNotificationRead(n.id);
		const url = getNotificationNavigationUrl(n);
		if (url) goto(url);
		close();
	}

	function handleSeeAll() {
		goto('/app/notifications');
		close();
	}

	function handleClickOutside(e: MouseEvent) {
		const target = e.target as HTMLElement;
		if (!target.isConnected) return;
		if (!target.closest('[data-notification-popover]')) {
			close();
		}
	}

	$effect(() => {
		if (open) {
			const timer = setTimeout(() => {
				document.addEventListener('click', handleClickOutside);
			}, 0);
			return () => {
				clearTimeout(timer);
				document.removeEventListener('click', handleClickOutside);
			};
		}
	});
</script>

{#if open}
	<div
		data-notification-popover
		class="fixed bottom-16 left-1 z-[100] flex w-[380px] max-w-[calc(100vw-1rem)] flex-col overflow-hidden rounded-lg border border-bg-modifier bg-bg-floating shadow-2xl"
		role="dialog"
		aria-modal="true"
		aria-label="Notifications"
	>
		<!-- Header -->
		<div class="flex items-center justify-between border-b border-bg-modifier px-4 py-3">
			<div class="flex items-center gap-2">
				<h2 class="text-sm font-semibold text-text-primary">Notifications</h2>
				{#if $unreadNotificationCount > 0}
					<span class="flex h-5 min-w-5 items-center justify-center rounded-full bg-red-500 px-1.5 text-2xs font-bold text-white">
						{$unreadNotificationCount > 99 ? '99+' : $unreadNotificationCount}
					</span>
				{/if}
				{#if $isDndActive}
					<span class="rounded bg-status-dnd/20 px-1.5 py-0.5 text-2xs font-medium text-status-dnd">DND</span>
				{/if}
			</div>
			<div class="flex items-center gap-1">
				{#if $notifications.length > 0}
					<button
						class="rounded px-2 py-1 text-2xs font-medium text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-primary"
						onclick={() => markAllNotificationsRead()}
						title="Mark all as read"
					>
						Mark all read
					</button>
				{/if}
			</div>
		</div>

		<!-- DND suppression indicator -->
		{#if $lastNotificationSuppressed}
			<div class="flex items-center gap-2 bg-status-dnd/10 px-4 py-2 text-2xs text-status-dnd">
				<svg class="h-3.5 w-3.5 shrink-0" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636" />
				</svg>
				Notification suppressed by Do Not Disturb
			</div>
		{/if}

		<!-- Notification list -->
		<div class="max-h-[400px] overflow-y-auto">
			{#if popoverNotifications.length === 0}
				<!-- Empty state -->
				<div class="flex flex-col items-center justify-center px-4 py-12 text-center">
					<svg class="mb-3 h-12 w-12 text-text-muted/30" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
						<path d="M14.857 17.082a23.848 23.848 0 005.454-1.31A8.967 8.967 0 0118 9.75v-.7V9A6 6 0 006 9v.75a8.967 8.967 0 01-2.312 6.022c1.733.64 3.56 1.085 5.455 1.31m5.714 0a24.255 24.255 0 01-5.714 0m5.714 0a3 3 0 11-5.714 0" />
					</svg>
					<h3 class="mb-1 text-sm font-medium text-text-secondary">You're all caught up</h3>
					<p class="text-2xs text-text-muted">New mentions, replies, and events will appear here.</p>
				</div>
			{:else}
				{#each popoverNotifications as notification (notification.id)}
					<NotificationItem {notification} compact onclick={handleNotificationClick} />
				{/each}
			{/if}
		</div>

		<!-- Footer -->
		{#if $notifications.length > 0}
			<div class="border-t border-bg-modifier">
				<button
					class="flex w-full items-center justify-center gap-1.5 py-2.5 text-xs font-medium text-brand-400 transition-colors hover:bg-bg-modifier hover:text-brand-300"
					onclick={handleSeeAll}
				>
					See all notifications
					<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
						<path d="M9 5l7 7-7 7" />
					</svg>
				</button>
			</div>
		{/if}
	</div>
{/if}
