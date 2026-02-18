// DM display helpers — extract recipient names and avatars from DM channels.

import type { Channel, User } from '$lib/types';

/**
 * Returns whether a user is from a remote (federated) instance.
 * A user is remote if their instance_id differs from the local instance ID.
 */
export function isRemoteUser(user: User, localInstanceId: string | undefined): boolean {
	if (!localInstanceId || !user.instance_id) return false;
	return user.instance_id !== localInstanceId;
}

/**
 * Returns a user's handle string.
 * Local users: @username
 * Remote users: @username@domain (requires domain lookup from instance)
 * If no domain is available, falls back to @username only.
 */
export function getUserHandle(user: User, localInstanceId: string | undefined, remoteDomain?: string): string {
	if (!localInstanceId || !user.instance_id || user.instance_id === localInstanceId) {
		return `@${user.username}`;
	}
	if (remoteDomain) {
		return `@${user.username}@${remoteDomain}`;
	}
	return `@${user.username}`;
}

/**
 * Returns the display name for a user, with domain suffix for remote users.
 */
export function getDisplayNameWithDomain(user: User, localInstanceId: string | undefined, remoteDomain?: string): string {
	const name = user.display_name ?? user.username;
	if (localInstanceId && user.instance_id && user.instance_id !== localInstanceId && remoteDomain) {
		return `${name} (${remoteDomain})`;
	}
	return name;
}

/**
 * Returns the display name for a DM channel based on its recipients.
 * For 1-on-1 DMs, returns the other user's display_name or username.
 * For group DMs with a name, returns the name.
 * Falls back to 'Direct Message'.
 */
export function getDMDisplayName(channel: Channel, selfUserId: string | undefined): string {
	if (channel.channel_type === 'group' && channel.name) {
		return channel.name;
	}

	if (channel.recipients && channel.recipients.length > 0 && selfUserId) {
		const others = channel.recipients.filter((u) => u.id !== selfUserId);
		if (others.length > 0) {
			if (channel.channel_type === 'group') {
				return others.map((u) => u.display_name ?? u.username).join(', ');
			}
			const other = others[0];
			return other.display_name ?? other.username;
		}
	}

	return channel.name ?? 'Direct Message';
}

/**
 * Returns the other user in a 1-on-1 DM channel, or undefined if not available.
 * Useful for avatar and presence display.
 */
export function getDMRecipient(channel: Channel, selfUserId: string | undefined): User | undefined {
	if (!channel.recipients || !selfUserId) return undefined;
	return channel.recipients.find((u) => u.id !== selfUserId);
}
