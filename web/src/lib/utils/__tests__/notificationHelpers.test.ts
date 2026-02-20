import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
	getNotificationDisplay,
	getNotificationNavigationUrl,
	formatNotificationTimestamp,
	getCategoryForType,
	isActionableType,
	ALL_NOTIFICATION_TYPES,
	NOTIFICATION_TYPE_LABELS,
	NOTIFICATION_CATEGORIES,
} from '../notificationHelpers';
import type { ServerNotification, ServerNotificationType } from '$lib/types';

function makeNotification(overrides: Partial<ServerNotification> = {}): ServerNotification {
	return {
		id: 'test-id',
		user_id: 'user1',
		type: 'mention',
		category: 'messages',
		guild_id: 'guild1',
		guild_name: 'Test Guild',
		guild_icon_id: null,
		channel_id: 'channel1',
		channel_name: 'general',
		message_id: 'msg1',
		actor_id: 'actor1',
		actor_name: 'Alice',
		actor_avatar_id: null,
		content: 'Hello @user1',
		metadata: null,
		read: false,
		created_at: new Date().toISOString(),
		...overrides,
	};
}

describe('getNotificationDisplay', () => {
	it('returns correct display for mention', () => {
		const n = makeNotification({ type: 'mention' });
		const display = getNotificationDisplay(n);
		expect(display.icon).toBe('@');
		expect(display.colorClass).toBe('text-indigo-400');
		expect(display.label).toBe('mentioned you');
		expect(display.preview).toBe('Hello @user1');
	});

	it('returns correct display for reply', () => {
		const n = makeNotification({ type: 'reply' });
		const display = getNotificationDisplay(n);
		expect(display.icon).toBe('↩');
		expect(display.label).toBe('replied to you');
	});

	it('returns correct display for dm', () => {
		const n = makeNotification({ type: 'dm', guild_id: null, guild_name: null });
		const display = getNotificationDisplay(n);
		expect(display.icon).toBe('💬');
		expect(display.label).toBe('sent you a message');
	});

	it('returns correct display for reaction with emoji metadata', () => {
		const n = makeNotification({
			type: 'reaction_added',
			metadata: { emoji: '🔥' },
			content: null,
		});
		const display = getNotificationDisplay(n);
		expect(display.label).toBe('reacted 🔥');
		expect(display.preview).toBe('');
	});

	it('returns correct display for guild_invite with guild name', () => {
		const n = makeNotification({ type: 'guild_invite', guild_name: 'Cool Server' });
		const display = getNotificationDisplay(n);
		expect(display.label).toBe('invited you to Cool Server');
	});

	it('returns correct display for member_joined with guild name', () => {
		const n = makeNotification({ type: 'member_joined', guild_name: 'My Guild' });
		const display = getNotificationDisplay(n);
		expect(display.label).toBe('joined My Guild');
	});

	it('returns correct display for moderation types', () => {
		const warned = getNotificationDisplay(makeNotification({ type: 'warned' }));
		expect(warned.label).toBe('You were warned');
		expect(warned.colorClass).toBe('text-orange-400');

		const banned = getNotificationDisplay(makeNotification({ type: 'banned' }));
		expect(banned.label).toBe('You were banned');
		expect(banned.colorClass).toBe('text-red-500');
	});

	it('returns correct display for event_starting with content', () => {
		const n = makeNotification({ type: 'event_starting', content: 'Game Night' });
		const display = getNotificationDisplay(n);
		expect(display.label).toBe('Event starting: Game Night');
	});

	it('handles all 17 types without errors', () => {
		for (const type of ALL_NOTIFICATION_TYPES) {
			const n = makeNotification({ type });
			const display = getNotificationDisplay(n);
			expect(display.icon).toBeTruthy();
			expect(display.colorClass).toBeTruthy();
			expect(display.label).toBeTruthy();
		}
	});
});

describe('getNotificationNavigationUrl', () => {
	it('returns guild/channel URL with message', () => {
		const n = makeNotification();
		expect(getNotificationNavigationUrl(n)).toBe('/app/guilds/guild1/channels/channel1?message=msg1');
	});

	it('returns guild/channel URL without message', () => {
		const n = makeNotification({ message_id: null });
		expect(getNotificationNavigationUrl(n)).toBe('/app/guilds/guild1/channels/channel1');
	});

	it('returns DM URL for DM notifications', () => {
		const n = makeNotification({ guild_id: null, guild_name: null, channel_id: 'dm1' });
		expect(getNotificationNavigationUrl(n)).toBe('/app/dms/dm1');
	});

	it('returns friends URL for friend_request', () => {
		const n = makeNotification({ type: 'friend_request', guild_id: null, channel_id: null });
		expect(getNotificationNavigationUrl(n)).toBe('/app/friends');
	});

	it('returns friends URL for friend_accepted', () => {
		const n = makeNotification({ type: 'friend_accepted', guild_id: null, channel_id: null });
		expect(getNotificationNavigationUrl(n)).toBe('/app/friends');
	});

	it('returns null for notifications with no context', () => {
		const n = makeNotification({ type: 'announcement', guild_id: null, channel_id: null });
		expect(getNotificationNavigationUrl(n)).toBeNull();
	});
});

describe('formatNotificationTimestamp', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		vi.setSystemTime(new Date('2026-02-19T12:00:00Z'));
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it('returns "Just now" for < 60 seconds', () => {
		expect(formatNotificationTimestamp('2026-02-19T11:59:30Z')).toBe('Just now');
	});

	it('returns minutes ago', () => {
		expect(formatNotificationTimestamp('2026-02-19T11:55:00Z')).toBe('5m ago');
	});

	it('returns hours ago', () => {
		expect(formatNotificationTimestamp('2026-02-19T09:00:00Z')).toBe('3h ago');
	});

	it('returns days ago', () => {
		expect(formatNotificationTimestamp('2026-02-16T12:00:00Z')).toBe('3d ago');
	});

	it('returns date for > 7 days', () => {
		const result = formatNotificationTimestamp('2026-02-01T12:00:00Z');
		// formatNotificationTimestamp uses 'en-US' locale explicitly
		expect(result).toMatch(/Feb.*1|1.*Feb/);
	});
});

describe('getCategoryForType', () => {
	it('categorizes message types correctly', () => {
		expect(getCategoryForType('mention')).toBe('messages');
		expect(getCategoryForType('reply')).toBe('messages');
		expect(getCategoryForType('dm')).toBe('messages');
		expect(getCategoryForType('thread_reply')).toBe('messages');
		expect(getCategoryForType('message_pinned')).toBe('messages');
		expect(getCategoryForType('reaction_added')).toBe('messages');
	});

	it('categorizes social types correctly', () => {
		expect(getCategoryForType('friend_request')).toBe('social');
		expect(getCategoryForType('friend_accepted')).toBe('social');
		expect(getCategoryForType('guild_invite')).toBe('social');
		expect(getCategoryForType('member_joined')).toBe('social');
	});

	it('categorizes moderation types correctly', () => {
		expect(getCategoryForType('warned')).toBe('moderation');
		expect(getCategoryForType('muted')).toBe('moderation');
		expect(getCategoryForType('kicked')).toBe('moderation');
		expect(getCategoryForType('banned')).toBe('moderation');
		expect(getCategoryForType('report_resolved')).toBe('moderation');
	});

	it('categorizes content types correctly', () => {
		expect(getCategoryForType('event_starting')).toBe('content');
		expect(getCategoryForType('announcement')).toBe('content');
	});
});

describe('isActionableType', () => {
	it('returns true for friend_request and guild_invite', () => {
		expect(isActionableType('friend_request')).toBe(true);
		expect(isActionableType('guild_invite')).toBe(true);
	});

	it('returns false for other types', () => {
		expect(isActionableType('mention')).toBe(false);
		expect(isActionableType('dm')).toBe(false);
		expect(isActionableType('banned')).toBe(false);
	});
});

describe('constants', () => {
	it('ALL_NOTIFICATION_TYPES has 17 entries', () => {
		expect(ALL_NOTIFICATION_TYPES).toHaveLength(17);
	});

	it('NOTIFICATION_TYPE_LABELS covers all types', () => {
		for (const type of ALL_NOTIFICATION_TYPES) {
			expect(NOTIFICATION_TYPE_LABELS[type]).toBeTruthy();
		}
	});

	it('NOTIFICATION_CATEGORIES covers all types', () => {
		const allTypes = NOTIFICATION_CATEGORIES.flatMap(c => c.types);
		expect(allTypes).toHaveLength(17);
		for (const type of ALL_NOTIFICATION_TYPES) {
			expect(allTypes).toContain(type);
		}
	});
});
