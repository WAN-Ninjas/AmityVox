<script lang="ts">
	import { goto } from '$app/navigation';
	import {
		notifications,
		groupedNotifications,
		unreadNotificationCount,
		markAllNotificationsRead,
		markNotificationRead,
		deleteNotification,
		clearAllNotifications,
	} from '$lib/stores/notifications';
	import type { ServerNotification } from '$lib/types';
	import {
		formatNotificationTimestamp,
		getNotificationDisplay,
		getNotificationNavigationUrl
	} from '$lib/utils/notificationHelpers';

	interface Props {
		open?: boolean;
		onclose?: () => void;
	}

	let { open = $bindable(false), onclose }: Props = $props();

	function close() {
		open = false;
		onclose?.();
	}

	function handleNotificationClick(notification: ServerNotification) {
		markNotificationRead(notification.id);

		const url = getNotificationNavigationUrl(notification);
		if (url) goto(url);

		close();
	}

	function handleMarkAllRead() {
		markAllNotificationsRead();
	}

	function handleClearAll() {
		clearAllNotifications();
	}

	function handleDismiss(e: MouseEvent, id: string) {
		e.stopPropagation();
		deleteNotification(id);
	}

	function handleBackdrop(e: MouseEvent) {
		if (e.target === e.currentTarget) close();
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') close();
	}
</script>

{#if open}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-[90]"
		onclick={handleBackdrop}
		onkeydown={handleKeydown}
		tabindex="-1"
	>
		<!-- Slide-out panel on the right -->
		<div
			class="absolute right-0 top-0 flex h-full w-full max-w-sm flex-col bg-bg-secondary shadow-2xl"
			role="dialog"
			aria-modal="true"
			aria-label="Notification center"
		>
			<!-- Header -->
			<div class="flex items-center justify-between border-b border-bg-floating px-4 py-3">
				<div class="flex items-center gap-2">
					<h2 class="text-base font-semibold text-text-primary">Notifications</h2>
					{#if $unreadNotificationCount > 0}
						<span class="flex h-5 min-w-5 items-center justify-center rounded-full bg-red-500 px-1.5 text-xs font-bold text-white">
							{$unreadNotificationCount > 99 ? '99+' : $unreadNotificationCount}
						</span>
					{/if}
				</div>
				<div class="flex items-center gap-2">
					{#if $notifications.length > 0}
						<button
							class="rounded px-2 py-1 text-xs font-medium text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-primary"
							onclick={handleMarkAllRead}
							title="Mark all as read"
						>
							Mark all read
						</button>
						<button
							class="rounded px-2 py-1 text-xs font-medium text-text-muted transition-colors hover:bg-bg-modifier hover:text-red-400"
							onclick={handleClearAll}
							title="Clear all notifications"
						>
							Clear all
						</button>
					{/if}
					<button
						class="rounded p-1 text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-primary"
						onclick={close}
						title="Close notifications"
					>
						<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path d="M6 18L18 6M6 6l12 12" />
						</svg>
					</button>
				</div>
			</div>

			<!-- Notification list -->
			<div class="flex-1 overflow-y-auto">
				{#if $notifications.length === 0}
					<!-- Empty state -->
					<div class="flex flex-col items-center justify-center px-4 py-16 text-center">
						<svg class="mb-4 h-16 w-16 text-text-muted/30" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
							<path d="M14.857 17.082a23.848 23.848 0 005.454-1.31A8.967 8.967 0 0118 9.75v-.7V9A6 6 0 006 9v.75a8.967 8.967 0 01-2.312 6.022c1.733.64 3.56 1.085 5.455 1.31m5.714 0a24.255 24.255 0 01-5.714 0m5.714 0a3 3 0 11-5.714 0" />
						</svg>
						<h3 class="mb-1 text-sm font-medium text-text-secondary">No notifications</h3>
						<p class="text-xs text-text-muted">You're all caught up! New mentions, replies, and DMs will appear here.</p>
					</div>
				{:else}
					{#each $groupedNotifications as group}
						<div class="border-b border-bg-modifier last:border-b-0">
							<h3 class="sticky top-0 bg-bg-secondary/95 px-4 py-2 text-2xs font-bold uppercase tracking-wide text-text-muted backdrop-blur-sm">
								{group.label}
							</h3>
							{#each group.notifications as notification (notification.id)}
								{@const display = getNotificationDisplay(notification)}
								<div
									role="button"
									tabindex="0"
									class="group flex w-full cursor-pointer items-start gap-3 px-4 py-3 text-left transition-colors hover:bg-bg-modifier {notification.read ? 'opacity-60' : ''}"
									onclick={() => handleNotificationClick(notification)}
									onkeydown={(e) => { if (e.key === 'Enter') handleNotificationClick(notification); }}
								>
									<!-- Type icon -->
									<div class="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-full {notification.read ? 'bg-bg-tertiary text-text-muted' : 'bg-brand-500/15 text-brand-400'}">
										<span class="text-sm font-bold {display.colorClass}">{display.icon}</span>
									</div>

									<!-- Content -->
									<div class="min-w-0 flex-1">
										<div class="flex items-center gap-1.5">
											<span class="truncate text-sm font-medium text-text-primary">{notification.actor_name}</span>
											{#if !notification.read}
												<span class="h-2 w-2 shrink-0 rounded-full bg-brand-500"></span>
											{/if}
										</div>
										<p class="text-xs text-text-muted">{display.label}</p>
										{#if display.preview}
											<p class="mt-0.5 truncate text-xs text-text-secondary">{display.preview}</p>
										{/if}
										<p class="mt-1 text-2xs text-text-muted">{formatNotificationTimestamp(notification.created_at)}</p>
									</div>

									<!-- Dismiss button -->
									<button
										class="mt-0.5 shrink-0 rounded p-1 text-text-muted opacity-0 transition-opacity hover:bg-bg-tertiary hover:text-text-primary group-hover:opacity-100 [button:hover_&]:opacity-100"
										onclick={(e) => handleDismiss(e, notification.id)}
										title="Dismiss"
									>
										<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
											<path d="M6 18L18 6M6 6l12 12" />
										</svg>
									</button>
								</div>
							{/each}
						</div>
					{/each}
				{/if}
			</div>
		</div>
	</div>
{/if}
