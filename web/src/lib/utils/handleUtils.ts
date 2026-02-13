/** Parsed result of a user handle string. */
export interface ParsedHandle {
	username: string;
	domain: string | null;
}

/**
 * Parse a handle string like "@user" or "@user@domain" into its parts.
 * Strips leading "@" and splits on the first remaining "@".
 * Returns null if the input is empty or has no username part.
 */
export function parseHandle(handle: string): ParsedHandle | null {
	if (!handle) return null;

	// Trim whitespace and strip leading @
	let h = handle.trim();
	h = h.startsWith('@') ? h.slice(1) : h;

	if (!h) return null;

	const atIndex = h.indexOf('@');
	if (atIndex === -1) {
		return { username: h, domain: null };
	}

	const username = h.slice(0, atIndex);
	const domain = h.slice(atIndex + 1);

	if (!username) return null;

	return {
		username,
		domain: domain || null
	};
}

/**
 * Format a username and optional domain into a handle string.
 * Returns "@username" for local users or "@username@domain" for federated users.
 */
export function formatHandle(username: string, domain?: string | null): string {
	if (!username) return '';
	if (domain) {
		return `@${username}@${domain}`;
	}
	return `@${username}`;
}
