// E2EE Manager — singleton orchestrating passphrase-based encryption.
// Derives AES-256-GCM keys from user passphrases via PBKDF2, stores them in IndexedDB,
// and provides encrypt/decrypt for channel messages.

import {
	deriveKeyFromPassphrase,
	encryptForWire,
	decryptFromWire
} from './crypto';
import {
	saveChannelSessionKey,
	loadChannelSessionKey,
	removeChannelSessionKey,
	clearAll
} from './keyStore';

class E2EEManager {
	private sessionKeyCache: Map<string, CryptoKey> = new Map();

	/**
	 * Derive and store a key from a passphrase for a channel.
	 * The key is deterministic: same passphrase + channelId = same key.
	 */
	async setPassphrase(channelId: string, passphrase: string): Promise<void> {
		const key = await deriveKeyFromPassphrase(passphrase, channelId);

		// Store in IndexedDB as JWK
		const jwk = await crypto.subtle.exportKey('jwk', key);
		await saveChannelSessionKey(channelId, jwk);

		// Cache in memory
		this.sessionKeyCache.set(channelId, key);
	}

	/** Check if we have a key for a channel (in cache or IndexedDB). */
	async hasChannelKey(channelId: string): Promise<boolean> {
		if (this.sessionKeyCache.has(channelId)) return true;
		const jwk = await loadChannelSessionKey(channelId);
		return jwk !== null;
	}

	/** Encrypt a message for a channel. Returns the wire-format ciphertext. */
	async encryptMessage(channelId: string, plaintext: string): Promise<string> {
		const key = await this.getSessionKey(channelId);
		if (!key) throw new Error('No encryption key for this channel');
		return encryptForWire(key, plaintext);
	}

	/** Decrypt a wire-format encrypted message. */
	async decryptMessage(channelId: string, ciphertext: string): Promise<string> {
		const key = await this.getSessionKey(channelId);
		if (!key) throw new Error('No decryption key for this channel');
		return decryptFromWire(key, ciphertext);
	}

	/** Remove the key for a channel. */
	async clearChannelKey(channelId: string): Promise<void> {
		this.sessionKeyCache.delete(channelId);
		await removeChannelSessionKey(channelId);
	}

	/** Clear all E2EE data from this device. */
	async reset(): Promise<void> {
		this.sessionKeyCache.clear();
		await clearAll();
	}

	/** Get the session key for a channel (from cache or IndexedDB). */
	private async getSessionKey(channelId: string): Promise<CryptoKey | null> {
		const cached = this.sessionKeyCache.get(channelId);
		if (cached) return cached;

		const jwk = await loadChannelSessionKey(channelId);
		if (!jwk) return null;

		const key = await crypto.subtle.importKey(
			'jwk',
			jwk,
			{ name: 'AES-GCM', length: 256 },
			true,
			['encrypt', 'decrypt']
		);
		this.sessionKeyCache.set(channelId, key);
		return key;
	}
}

/** Singleton E2EE manager instance. */
export const e2ee = new E2EEManager();
