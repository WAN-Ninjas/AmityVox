import { describe, it, expect } from 'vitest';

// FederationBadge is a pure display component — test the domain formatting
// logic that it relies on (isRemoteUser, getUserHandle from dm.ts).
// Component rendering tests aren't feasible with Svelte 5 + happy-dom.
import { isRemoteUser, getUserHandle } from '$lib/utils/dm';

describe('FederationBadge display logic', () => {
	const localInstanceId = 'local-instance-id';

	it('identifies remote users correctly', () => {
		const remoteUser = {
			id: 'u1',
			instance_id: 'remote-instance-id',
			username: 'alice',
			display_name: null,
			avatar_id: null,
			status_text: null,
			status_emoji: null,
			status_presence: 'offline' as const,
			status_expires_at: null,
			bio: null,
			bot_owner_id: null,
			email: null,
			banner_id: null,
			accent_color: null,
			pronouns: null,
			flags: 0,
			last_online: null,
			created_at: '2024-01-01T00:00:00Z',
		};

		expect(isRemoteUser(remoteUser, localInstanceId)).toBe(true);
	});

	it('does not flag local users as remote', () => {
		const localUser = {
			id: 'u2',
			instance_id: localInstanceId,
			username: 'bob',
			display_name: null,
			avatar_id: null,
			status_text: null,
			status_emoji: null,
			status_presence: 'online' as const,
			status_expires_at: null,
			bio: null,
			bot_owner_id: null,
			email: null,
			banner_id: null,
			accent_color: null,
			pronouns: null,
			flags: 0,
			last_online: null,
			created_at: '2024-01-01T00:00:00Z',
		};

		expect(isRemoteUser(localUser, localInstanceId)).toBe(false);
	});

	it('formats federated handle with domain', () => {
		const user = {
			id: 'u3',
			instance_id: 'remote-inst',
			username: 'charlie',
			display_name: null,
			avatar_id: null,
			status_text: null,
			status_emoji: null,
			status_presence: 'offline' as const,
			status_expires_at: null,
			bio: null,
			bot_owner_id: null,
			email: null,
			banner_id: null,
			accent_color: null,
			pronouns: null,
			flags: 0,
			last_online: null,
			created_at: '2024-01-01T00:00:00Z',
		};

		expect(getUserHandle(user, localInstanceId, 'other.chat')).toBe('@charlie@other.chat');
	});
});
