<script lang="ts">
	import { goto } from '$app/navigation';
	import type { ServerNotification } from '$lib/types';
	import { getNotificationDisplay, formatNotificationTimestamp, isActionableType } from '$lib/utils/notificationHelpers';
	import { markNotificationRead, deleteNotification } from '$lib/stores/notifications';
	import Avatar from './Avatar.svelte';

	interface Props {
		notification: ServerNotification;
		compact?: boolean;
		onclick?: (n: ServerNotification) => void;
		ondelete?: (id: string) => void;
	}

	let { notification, compact = false, onclick, ondelete }: Props = $props();

	const display = $derived(getNotificationDisplay(notification));

	function handleClick() {
		if (onclick) {
			onclick(notification);
			return;
		}
		if (!notification.read) markNotificationRead(notification.id);
		if (display.navigationUrl) goto(display.navigationUrl);
	}

	function handleDismiss(e: MouseEvent) {
		e.stopPropagation();
		if (ondelete) {
			ondelete(notification.id);
		} else {
			deleteNotification(notification.id);
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' || e.key === ' ') {
			e.preventDefault();
			handleClick();
		}
	}
</script>

<div
	role="button"
	tabindex="0"
	class="group flex w-full cursor-pointer items-start gap-3 text-left transition-colors hover:bg-bg-modifier {compact ? 'px-3 py-2' : 'px-4 py-3'} {notification.read ? 'opacity-60' : ''}"
	onclick={handleClick}
	onkeydown={handleKeydown}
>
	<!-- Type icon circle -->
	<div class="mt-0.5 flex {compact ? 'h-7 w-7' : 'h-8 w-8'} shrink-0 items-center justify-center rounded-full {notification.read ? 'bg-bg-tertiary text-text-muted' : 'bg-brand-500/15'} {display.colorClass}">
		{#if notification.actor_avatar_id}
			<Avatar userId={notification.actor_id} avatarId={notification.actor_avatar_id} displayName={notification.actor_name} size={compact ? 28 : 32} />
		{:else}
			<span class="{compact ? 'text-xs' : 'text-sm'} font-bold">{display.icon}</span>
		{/if}
	</div>

	<!-- Content -->
	<div class="min-w-0 flex-1">
		<div class="flex items-center gap-1.5">
			<span class="truncate {compact ? 'text-xs' : 'text-sm'} font-medium text-text-primary">{notification.actor_name}</span>
			{#if !notification.read}
				<span class="h-2 w-2 shrink-0 rounded-full bg-brand-500"></span>
			{/if}
		</div>
		<p class="{compact ? 'text-2xs' : 'text-xs'} text-text-muted">{display.label}</p>
		{#if display.preview && !compact}
			<p class="mt-0.5 truncate text-xs text-text-secondary">{display.preview}</p>
		{/if}
		{#if notification.guild_name || notification.channel_name}
			<p class="mt-0.5 truncate text-2xs text-text-muted/70">
				{#if notification.guild_name}{notification.guild_name}{/if}{#if notification.guild_name && notification.channel_name} / {/if}{#if notification.channel_name}#{notification.channel_name}{/if}
			</p>
		{/if}
		<p class="mt-0.5 text-2xs text-text-muted">{formatNotificationTimestamp(notification.created_at)}</p>

		<!-- Action buttons for actionable types -->
		{#if isActionableType(notification.type) && !notification.read}
			<div class="mt-1.5 flex gap-2">
				<button
					class="rounded bg-brand-500 px-2.5 py-1 text-2xs font-medium text-white transition-colors hover:bg-brand-600"
					onclick={(e) => { e.stopPropagation(); handleClick(); }}
				>
					{notification.type === 'friend_request' ? 'View' : 'Accept'}
				</button>
				<button
					class="rounded bg-bg-tertiary px-2.5 py-1 text-2xs font-medium text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-primary"
					onclick={handleDismiss}
				>
					Dismiss
				</button>
			</div>
		{/if}
	</div>

	<!-- Dismiss button (hover only) -->
	<button
		class="mt-0.5 shrink-0 rounded p-1 text-text-muted opacity-0 transition-opacity hover:bg-bg-tertiary hover:text-text-primary group-hover:opacity-100"
		onclick={handleDismiss}
		title="Dismiss"
	>
		<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path d="M6 18L18 6M6 6l12 12" />
		</svg>
	</button>
</div>
