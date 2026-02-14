import { describe, it, expect } from 'vitest';
import { getDMDisplayName, getDMRecipient } from '../dm';
import type { Channel, User } from '$lib/types';

function makeUser(overrides: Partial<User> = {}): User {
	return {
		id: 'user1',
		instance_id: 'inst1',
		username: 'alice',
		display_name: null,
		avatar_id: null,
		status_text: null,
		status_emoji: null,
		status_presence: 'online',
		status_expires_at: null,
		bio: null,
		bot_owner_id: null,
		email: null,
		banner_id: null,
		accent_color: null,
		pronouns: null,
		flags: 0,
		created_at: '2024-01-01T00:00:00Z',
		...overrides,
	};
}

function makeChannel(overrides: Partial<Channel> = {}): Channel {
	return {
		id: 'ch1',
		guild_id: null,
		category_id: null,
		channel_type: 'dm',
		name: null,
		topic: null,
		position: 0,
		slowmode_seconds: 0,
		nsfw: false,
		encrypted: false,
		last_message_id: null,
		owner_id: null,
		user_limit: 0,
		bitrate: 0,
		locked: false,
		locked_by: null,
		locked_at: null,
		archived: false,
		created_at: '2024-01-01T00:00:00Z',
		...overrides,
	};
}

describe('getDMDisplayName', () => {
	it('returns other user display_name for 1-on-1 DM', () => {
		const self = makeUser({ id: 'me', username: 'me' });
		const other = makeUser({ id: 'other', username: 'bob', display_name: 'Bob Smith' });
		const channel = makeChannel({ recipients: [self, other] });

		expect(getDMDisplayName(channel, 'me')).toBe('Bob Smith');
	});

	it('returns other user username when display_name is null', () => {
		const self = makeUser({ id: 'me', username: 'me' });
		const other = makeUser({ id: 'other', username: 'bob', display_name: null });
		const channel = makeChannel({ recipients: [self, other] });

		expect(getDMDisplayName(channel, 'me')).toBe('bob');
	});

	it('returns channel name for group DM with name set', () => {
		const channel = makeChannel({
			channel_type: 'group',
			name: 'Cool Group',
			recipients: [
				makeUser({ id: 'me' }),
				makeUser({ id: 'a', username: 'alice' }),
				makeUser({ id: 'b', username: 'bob' }),
			],
		});

		expect(getDMDisplayName(channel, 'me')).toBe('Cool Group');
	});

	it('returns comma-separated names for unnamed group DM', () => {
		const channel = makeChannel({
			channel_type: 'group',
			name: null,
			recipients: [
				makeUser({ id: 'me' }),
				makeUser({ id: 'a', username: 'alice', display_name: 'Alice' }),
				makeUser({ id: 'b', username: 'bob', display_name: null }),
			],
		});

		expect(getDMDisplayName(channel, 'me')).toBe('Alice, bob');
	});

	it('returns "Direct Message" when no recipients', () => {
		const channel = makeChannel({ recipients: undefined });
		expect(getDMDisplayName(channel, 'me')).toBe('Direct Message');
	});

	it('returns "Direct Message" when selfUserId is undefined', () => {
		const channel = makeChannel({
			recipients: [makeUser({ id: 'a' }), makeUser({ id: 'b' })],
		});
		expect(getDMDisplayName(channel, undefined)).toBe('Direct Message');
	});

	it('returns channel name fallback when name is set but no recipients', () => {
		const channel = makeChannel({ name: 'Named DM', recipients: undefined });
		expect(getDMDisplayName(channel, 'me')).toBe('Named DM');
	});
});

describe('getDMRecipient', () => {
	it('returns the other user in a 1-on-1 DM', () => {
		const self = makeUser({ id: 'me' });
		const other = makeUser({ id: 'other', username: 'bob' });
		const channel = makeChannel({ recipients: [self, other] });

		const result = getDMRecipient(channel, 'me');
		expect(result?.id).toBe('other');
		expect(result?.username).toBe('bob');
	});

	it('returns undefined when no recipients', () => {
		const channel = makeChannel({ recipients: undefined });
		expect(getDMRecipient(channel, 'me')).toBeUndefined();
	});

	it('returns undefined when selfUserId is undefined', () => {
		const channel = makeChannel({
			recipients: [makeUser({ id: 'a' })],
		});
		expect(getDMRecipient(channel, undefined)).toBeUndefined();
	});

	it('returns undefined when only self is in recipients', () => {
		const channel = makeChannel({
			recipients: [makeUser({ id: 'me' })],
		});
		expect(getDMRecipient(channel, 'me')).toBeUndefined();
	});
});
