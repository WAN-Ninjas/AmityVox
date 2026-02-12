<script lang="ts">
	import { goto } from '$app/navigation';
	import {
		notifications,
		groupedNotifications,
		unreadNotificationCount,
		markAllNotificationsRead,
		markNotificationRead,
		removeNotification,
		clearAllNotifications,
		type AppNotification
	} from '$lib/stores/notifications';

	interface Props {
		open?: boolean;
		onclose?: () => void;
	}

	let { open = $bindable(false), onclose }: Props = $props();

	function close() {
		open = false;
		onclose?.();
	}

	function handleNotificationClick(notification: AppNotification) {
		markNotificationRead(notification.id);

		if (notification.type === 'friend_request') {
			goto('/app/friends');
		} else if (notification.channel_id) {
			if (notification.guild_id) {
				goto(`/app/guilds/${notification.guild_id}/channels/${notification.channel_id}`);
			} else {
				goto(`/app/dms/${notification.channel_id}`);
			}
		}

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
		removeNotification(id);
	}

	function getTypeIcon(type: AppNotification['type']): string {
		switch (type) {
			case 'mention':
				return '<path d="M4 9h16M4 15h16M10 3l-2 18M16 3l-2 18" />';
			case 'reply':
				return '<path d="M3 10h10a8 8 0 018 8v2M3 10l6 6m-6-6l6-6" />';
			case 'dm':
				return '<path d="M21 11.5a8.38 8.38 0 01-.9 3.8 8.5 8.5 0 01-7.6 4.7 8.38 8.38 0 01-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 01-.9-3.8 8.5 8.5 0 014.7-7.6 8.38 8.38 0 013.8-.9h.5a8.48 8.48 0 018 8v.5z" />';
			case 'friend_request':
				return '<path d="M16 21v-2a4 4 0 00-4-4H5a4 4 0 00-4 4v2" /><circle cx="8.5" cy="7" r="4" /><line x1="20" y1="8" x2="20" y2="14" /><line x1="23" y1="11" x2="17" y2="11" />';
			default:
				return '<circle cx="12" cy="12" r="10" />';
		}
	}

	function getTypeLabel(type: AppNotification['type']): string {
		switch (type) {
			case 'mention':
				return 'Mentioned you';
			case 'reply':
				return 'Replied to you';
			case 'dm':
				return 'Direct message';
			case 'friend_request':
				return 'Friend request';
			default:
				return 'Notification';
		}
	}

	function formatTimestamp(isoStr: string): string {
		const date = new Date(isoStr);
		const now = new Date();
		const diffMs = now.getTime() - date.getTime();
		const diffMin = Math.floor(diffMs / 60000);
		const diffHr = Math.floor(diffMs / 3600000);
		const diffDay = Math.floor(diffMs / 86400000);

		if (diffMin < 1) return 'Just now';
		if (diffMin < 60) return `${diffMin}m ago`;
		if (diffHr < 24) return `${diffHr}h ago`;
		if (diffDay < 7) return `${diffDay}d ago`;
		return date.toLocaleDateString();
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
		<aside
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
								<div
									role="button"
									tabindex="0"
									class="group flex w-full cursor-pointer items-start gap-3 px-4 py-3 text-left transition-colors hover:bg-bg-modifier {notification.read ? 'opacity-60' : ''}"
									onclick={() => handleNotificationClick(notification)}
									onkeydown={(e) => { if (e.key === 'Enter') handleNotificationClick(notification); }}
								>
									<!-- Type icon -->
									<div class="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-full {notification.read ? 'bg-bg-tertiary text-text-muted' : 'bg-brand-500/15 text-brand-400'}">
										<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
											{@html getTypeIcon(notification.type)}
										</svg>
									</div>

									<!-- Content -->
									<div class="min-w-0 flex-1">
										<div class="flex items-center gap-1.5">
											<span class="truncate text-sm font-medium text-text-primary">{notification.sender_name}</span>
											{#if !notification.read}
												<span class="h-2 w-2 shrink-0 rounded-full bg-brand-500"></span>
											{/if}
										</div>
										<p class="text-xs text-text-muted">{getTypeLabel(notification.type)}</p>
										{#if notification.content}
											<p class="mt-0.5 truncate text-xs text-text-secondary">{notification.content}</p>
										{/if}
										<p class="mt-1 text-2xs text-text-muted">{formatTimestamp(notification.created_at)}</p>
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
		</aside>
	</div>
{/if}
