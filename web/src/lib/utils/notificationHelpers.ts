// Pure helper functions for notification display, formatting, and navigation.
// These are extracted from components for testability.

import type { ServerNotification, ServerNotificationType, NotificationCategory } from '$lib/types';

export interface NotificationDisplay {
	icon: string;
	colorClass: string;
	label: string;
	preview: string;
	navigationUrl: string | null;
}

const TYPE_DISPLAY: Record<ServerNotificationType, { icon: string; colorClass: string; label: string }> = {
	mention: { icon: '@', colorClass: 'text-indigo-400', label: 'mentioned you' },
	reply: { icon: '↩', colorClass: 'text-blue-400', label: 'replied to you' },
	dm: { icon: '💬', colorClass: 'text-green-400', label: 'sent you a message' },
	thread_reply: { icon: '🧵', colorClass: 'text-purple-400', label: 'replied in thread' },
	message_pinned: { icon: '📌', colorClass: 'text-amber-400', label: 'pinned a message' },
	reaction_added: { icon: '❤', colorClass: 'text-pink-400', label: 'reacted' },
	friend_request: { icon: '👤+', colorClass: 'text-yellow-400', label: 'sent a friend request' },
	friend_accepted: { icon: '✓', colorClass: 'text-green-400', label: 'accepted your friend request' },
	guild_invite: { icon: '📨', colorClass: 'text-indigo-400', label: 'invited you' },
	member_joined: { icon: '→', colorClass: 'text-teal-400', label: 'joined' },
	warned: { icon: '⚠', colorClass: 'text-orange-400', label: 'You were warned' },
	muted: { icon: '🔇', colorClass: 'text-orange-400', label: 'You were muted' },
	kicked: { icon: '✕', colorClass: 'text-red-400', label: 'You were kicked' },
	banned: { icon: '🛡', colorClass: 'text-red-500', label: 'You were banned' },
	report_resolved: { icon: '✓', colorClass: 'text-green-400', label: 'Your report was resolved' },
	event_starting: { icon: '📅', colorClass: 'text-cyan-400', label: 'Event starting' },
	announcement: { icon: '📢', colorClass: 'text-indigo-400', label: 'New announcement' },
};

export function getNotificationDisplay(n: ServerNotification): NotificationDisplay {
	const typeInfo = TYPE_DISPLAY[n.type] ?? { icon: '🔔', colorClass: 'text-gray-400', label: n.type };

	let label = typeInfo.label;
	// Append context for certain types.
	if (n.type === 'reaction_added' && n.metadata) {
		const emoji = (n.metadata as Record<string, string>).emoji;
		if (emoji) label = `reacted ${emoji}`;
	}
	if (n.type === 'guild_invite' && n.guild_name) {
		label = `invited you to ${n.guild_name}`;
	}
	if (n.type === 'member_joined' && n.guild_name) {
		label = `joined ${n.guild_name}`;
	}
	if (n.type === 'event_starting' && n.content) {
		label = `Event starting: ${n.content}`;
	}

	const preview = n.content ?? '';

	return {
		icon: typeInfo.icon,
		colorClass: typeInfo.colorClass,
		label,
		preview,
		navigationUrl: getNotificationNavigationUrl(n),
	};
}

export function getNotificationNavigationUrl(n: ServerNotification): string | null {
	if (n.guild_id && n.channel_id) {
		const url = `/app/guilds/${n.guild_id}/channels/${n.channel_id}`;
		if (n.message_id) return `${url}?message=${n.message_id}`;
		return url;
	}
	if (n.channel_id) {
		return `/app/dms/${n.channel_id}`;
	}
	if (n.type === 'friend_request' || n.type === 'friend_accepted') {
		return '/app/friends';
	}
	return null;
}

export function formatNotificationTimestamp(isoStr: string): string {
	const date = new Date(isoStr);
	const now = new Date();
	const diffMs = now.getTime() - date.getTime();
	const diffSec = Math.floor(diffMs / 1000);
	const diffMin = Math.floor(diffSec / 60);
	const diffHr = Math.floor(diffMin / 60);
	const diffDay = Math.floor(diffHr / 24);

	if (diffSec < 60) return 'Just now';
	if (diffMin < 60) return `${diffMin}m ago`;
	if (diffHr < 24) return `${diffHr}h ago`;
	if (diffDay < 7) return `${diffDay}d ago`;

	return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}

export function getCategoryForType(type: ServerNotificationType): NotificationCategory {
	switch (type) {
		case 'mention':
		case 'reply':
		case 'dm':
		case 'thread_reply':
		case 'message_pinned':
		case 'reaction_added':
			return 'messages';
		case 'friend_request':
		case 'friend_accepted':
		case 'guild_invite':
		case 'member_joined':
			return 'social';
		case 'warned':
		case 'muted':
		case 'kicked':
		case 'banned':
		case 'report_resolved':
			return 'moderation';
		case 'event_starting':
		case 'announcement':
			return 'content';
		default:
			return 'messages';
	}
}

// Returns true for notification types that have inline action buttons (accept/decline).
export function isActionableType(type: ServerNotificationType): boolean {
	return type === 'friend_request' || type === 'guild_invite';
}

// All notification types in display order.
export const ALL_NOTIFICATION_TYPES: ServerNotificationType[] = [
	'mention', 'reply', 'dm', 'thread_reply', 'message_pinned', 'reaction_added',
	'friend_request', 'friend_accepted', 'guild_invite', 'member_joined',
	'warned', 'muted', 'kicked', 'banned', 'report_resolved',
	'event_starting', 'announcement',
];

// Human-readable label for each notification type (for settings).
export const NOTIFICATION_TYPE_LABELS: Record<ServerNotificationType, string> = {
	mention: 'Mentions',
	reply: 'Replies',
	dm: 'Direct Messages',
	thread_reply: 'Thread Replies',
	message_pinned: 'Pinned Messages',
	reaction_added: 'Reactions',
	friend_request: 'Friend Requests',
	friend_accepted: 'Friend Accepted',
	guild_invite: 'Server Invites',
	member_joined: 'Member Joined',
	warned: 'Warnings',
	muted: 'Muted',
	kicked: 'Kicked',
	banned: 'Banned',
	report_resolved: 'Report Resolved',
	event_starting: 'Event Starting',
	announcement: 'Announcements',
};

// Category labels and which types belong to each.
export const NOTIFICATION_CATEGORIES: { label: string; category: NotificationCategory; types: ServerNotificationType[] }[] = [
	{ label: 'Messages', category: 'messages', types: ['mention', 'reply', 'dm', 'thread_reply', 'message_pinned', 'reaction_added'] },
	{ label: 'Social', category: 'social', types: ['friend_request', 'friend_accepted', 'guild_invite', 'member_joined'] },
	{ label: 'Moderation', category: 'moderation', types: ['warned', 'muted', 'kicked', 'banned', 'report_resolved'] },
	{ label: 'Content', category: 'content', types: ['event_starting', 'announcement'] },
];
