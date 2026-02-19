import { describe, it, expect } from 'vitest';

// Pure-function tests mirroring the encryption decision logic in MessageInput.svelte
// and the attachment visibility logic in MessageItem.svelte.
// Svelte 5 components can't be rendered in happy-dom (SSR mode).

// --- Encryption decision logic (mirrors MessageInput.svelte) ---

interface ChannelLike {
	encrypted?: boolean;
}

/**
 * Determines whether a send operation should include `encrypted: true`.
 * Mirrors the pattern used across all 6 send paths in MessageInput.svelte.
 */
function shouldEncrypt(channel: ChannelLike | null | undefined): boolean {
	return !!channel?.encrypted;
}

/**
 * Determines whether text content needs to be encrypted before sending.
 * Content-bearing sends (handleSubmit, uploadPendingFiles with text, insertGif)
 * must encrypt the text. Content-free sends (file-only, sticker, voice) only
 * need the `encrypted: true` flag.
 */
function shouldEncryptContent(channel: ChannelLike | null | undefined, content: string): boolean {
	return !!channel?.encrypted && !!content;
}

/**
 * Determines whether scheduling is allowed in the given channel.
 * Scheduling is blocked in encrypted channels because the backend's
 * scheduleMessageRequest has no Encrypted field.
 */
function canSchedule(channel: ChannelLike | null | undefined): boolean {
	return !channel?.encrypted;
}

// --- Attachment visibility logic (mirrors MessageItem.svelte) ---

interface MessageLike {
	encrypted?: boolean;
	content?: string;
	attachments?: { id: string }[];
}

/**
 * Determines whether attachments should be rendered for a message.
 * In encrypted channels, attachments are only shown when the user has the key.
 */
function shouldShowAttachments(
	message: MessageLike,
	hasEncryptionKey: boolean | null
): boolean {
	if (!message.attachments?.length) return false;
	if (message.encrypted && hasEncryptionKey !== true) return false;
	return true;
}

type FileOnlyEncryptedState = 'checking' | 'locked' | 'unlocked' | 'not-applicable';

/**
 * Determines the display state for a file-only encrypted message (no text content).
 * - 'checking': key status unknown (hasEncryptionKey === null)
 * - 'locked': no key available — show passphrase prompt
 * - 'unlocked': key available — show attachments normally
 * - 'not-applicable': message is not encrypted or has text content
 */
function fileOnlyEncryptedState(
	message: MessageLike,
	hasEncryptionKey: boolean | null
): FileOnlyEncryptedState {
	if (!message.encrypted || message.content) return 'not-applicable';
	if (hasEncryptionKey === null) return 'checking';
	if (hasEncryptionKey === false) return 'locked';
	return 'unlocked';
}

// --- Tests ---

describe('MessageInput encryption decision logic', () => {
	describe('shouldEncrypt', () => {
		it('returns true for encrypted channels', () => {
			expect(shouldEncrypt({ encrypted: true })).toBe(true);
		});

		it('returns false for non-encrypted channels', () => {
			expect(shouldEncrypt({ encrypted: false })).toBe(false);
			expect(shouldEncrypt({})).toBe(false);
		});

		it('returns false for null/undefined channel', () => {
			expect(shouldEncrypt(null)).toBe(false);
			expect(shouldEncrypt(undefined)).toBe(false);
		});
	});

	describe('shouldEncryptContent', () => {
		it('returns true for encrypted channel with text content', () => {
			expect(shouldEncryptContent({ encrypted: true }, 'hello')).toBe(true);
		});

		it('returns false for encrypted channel with empty content', () => {
			expect(shouldEncryptContent({ encrypted: true }, '')).toBe(false);
		});

		it('returns false for non-encrypted channel with text content', () => {
			expect(shouldEncryptContent({ encrypted: false }, 'hello')).toBe(false);
		});

		it('returns false for null channel', () => {
			expect(shouldEncryptContent(null, 'hello')).toBe(false);
		});
	});

	describe('canSchedule', () => {
		it('returns false for encrypted channels', () => {
			expect(canSchedule({ encrypted: true })).toBe(false);
		});

		it('returns true for non-encrypted channels', () => {
			expect(canSchedule({ encrypted: false })).toBe(true);
			expect(canSchedule({})).toBe(true);
		});

		it('returns true for null/undefined channel', () => {
			expect(canSchedule(null)).toBe(true);
			expect(canSchedule(undefined)).toBe(true);
		});
	});
});

describe('MessageItem attachment visibility', () => {
	describe('shouldShowAttachments', () => {
		it('shows attachments for non-encrypted message', () => {
			expect(shouldShowAttachments(
				{ attachments: [{ id: '1' }] },
				null
			)).toBe(true);
		});

		it('hides attachments when message has no attachments', () => {
			expect(shouldShowAttachments({ attachments: [] }, null)).toBe(false);
			expect(shouldShowAttachments({}, null)).toBe(false);
		});

		it('hides attachments for encrypted message without key', () => {
			expect(shouldShowAttachments(
				{ encrypted: true, attachments: [{ id: '1' }] },
				false
			)).toBe(false);
		});

		it('hides attachments for encrypted message while checking key', () => {
			expect(shouldShowAttachments(
				{ encrypted: true, attachments: [{ id: '1' }] },
				null
			)).toBe(false);
		});

		it('shows attachments for encrypted message with key', () => {
			expect(shouldShowAttachments(
				{ encrypted: true, attachments: [{ id: '1' }] },
				true
			)).toBe(true);
		});
	});

	describe('fileOnlyEncryptedState', () => {
		it('returns not-applicable for non-encrypted messages', () => {
			expect(fileOnlyEncryptedState({ content: '' }, null)).toBe('not-applicable');
			expect(fileOnlyEncryptedState({ encrypted: false }, null)).toBe('not-applicable');
		});

		it('returns not-applicable for encrypted messages with text content', () => {
			expect(fileOnlyEncryptedState(
				{ encrypted: true, content: 'hello' },
				false
			)).toBe('not-applicable');
		});

		it('returns checking when key status is unknown', () => {
			expect(fileOnlyEncryptedState(
				{ encrypted: true },
				null
			)).toBe('checking');
		});

		it('returns locked when no key available', () => {
			expect(fileOnlyEncryptedState(
				{ encrypted: true },
				false
			)).toBe('locked');
		});

		it('returns unlocked when key is available', () => {
			expect(fileOnlyEncryptedState(
				{ encrypted: true },
				true
			)).toBe('unlocked');
		});

		it('treats empty string content as no content', () => {
			expect(fileOnlyEncryptedState(
				{ encrypted: true, content: '' },
				false
			)).toBe('locked');
		});
	});
});
