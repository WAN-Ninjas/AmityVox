import { describe, it, expect, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import {
	blockedUsers,
	blockedUserIds,
	addBlockedUser,
	removeBlockedUser,
	updateBlockedUserLevel,
	getBlockLevel,
	isBlocked,
	type BlockLevel
} from '../blocked';

describe('blocked store', () => {
	beforeEach(() => {
		// Reset the store to empty before each test.
		// We can't import the writable directly, but we can remove all users.
		const current = get(blockedUsers);
		for (const userId of current.keys()) {
			removeBlockedUser(userId);
		}
	});

	it('starts empty', () => {
		expect(get(blockedUsers).size).toBe(0);
		expect(get(blockedUserIds).size).toBe(0);
	});

	it('addBlockedUser adds user with default block level', () => {
		addBlockedUser('user1');
		expect(get(blockedUsers).get('user1')).toBe('block');
		expect(get(blockedUserIds).has('user1')).toBe(true);
	});

	it('addBlockedUser adds user with ignore level', () => {
		addBlockedUser('user2', 'ignore');
		expect(get(blockedUsers).get('user2')).toBe('ignore');
	});

	it('addBlockedUser adds user with block level', () => {
		addBlockedUser('user3', 'block');
		expect(get(blockedUsers).get('user3')).toBe('block');
	});

	it('removeBlockedUser removes a user', () => {
		addBlockedUser('user1');
		expect(get(blockedUsers).has('user1')).toBe(true);
		removeBlockedUser('user1');
		expect(get(blockedUsers).has('user1')).toBe(false);
	});

	it('removeBlockedUser is safe for non-existent user', () => {
		removeBlockedUser('nonexistent');
		expect(get(blockedUsers).size).toBe(0);
	});

	it('updateBlockedUserLevel changes level from block to ignore', () => {
		addBlockedUser('user1', 'block');
		updateBlockedUserLevel('user1', 'ignore');
		expect(get(blockedUsers).get('user1')).toBe('ignore');
	});

	it('updateBlockedUserLevel changes level from ignore to block', () => {
		addBlockedUser('user1', 'ignore');
		updateBlockedUserLevel('user1', 'block');
		expect(get(blockedUsers).get('user1')).toBe('block');
	});

	it('updateBlockedUserLevel does nothing for non-existent user', () => {
		updateBlockedUserLevel('nonexistent', 'block');
		expect(get(blockedUsers).size).toBe(0);
	});

	it('getBlockLevel returns correct level', () => {
		addBlockedUser('user1', 'ignore');
		addBlockedUser('user2', 'block');
		expect(getBlockLevel('user1')).toBe('ignore');
		expect(getBlockLevel('user2')).toBe('block');
	});

	it('getBlockLevel returns null for non-blocked user', () => {
		expect(getBlockLevel('nobody')).toBeNull();
	});

	it('isBlocked returns true for blocked users', () => {
		addBlockedUser('user1', 'ignore');
		addBlockedUser('user2', 'block');
		expect(isBlocked('user1')).toBe(true);
		expect(isBlocked('user2')).toBe(true);
	});

	it('isBlocked returns false for non-blocked users', () => {
		expect(isBlocked('nobody')).toBe(false);
	});

	it('blockedUserIds derived store includes all blocked users', () => {
		addBlockedUser('a', 'ignore');
		addBlockedUser('b', 'block');
		addBlockedUser('c', 'ignore');
		const ids = get(blockedUserIds);
		expect(ids.size).toBe(3);
		expect(ids.has('a')).toBe(true);
		expect(ids.has('b')).toBe(true);
		expect(ids.has('c')).toBe(true);
	});

	it('multiple operations maintain consistency', () => {
		addBlockedUser('user1', 'block');
		addBlockedUser('user2', 'ignore');
		addBlockedUser('user3', 'block');

		removeBlockedUser('user2');
		updateBlockedUserLevel('user1', 'ignore');

		expect(get(blockedUsers).size).toBe(2);
		expect(getBlockLevel('user1')).toBe('ignore');
		expect(getBlockLevel('user2')).toBeNull();
		expect(getBlockLevel('user3')).toBe('block');
		expect(get(blockedUserIds).size).toBe(2);
	});
});
