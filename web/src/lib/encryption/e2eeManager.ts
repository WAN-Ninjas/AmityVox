// E2EE Manager — singleton orchestrating passphrase-based encryption.
// Derives AES-256-GCM keys from user passphrases via PBKDF2, stores them in IndexedDB,
// and provides encrypt/decrypt for channel messages.

import { writable } from 'svelte/store';
import {
	deriveKeyFromPassphrase,
	encryptForWire,
	decryptFromWire,
	encryptBinary,
	decryptBinary
} from './crypto';
import {
	saveChannelSessionKey,
	loadChannelSessionKey,
	removeChannelSessionKey,
	clearAll
} from './keyStore';

/** Reactive store tracking which channel IDs have a decryption key loaded. */
export const unlockedChannels = writable<Set<string>>(new Set());

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

		// Update reactive store
		unlockedChannels.update(s => { s.add(channelId); return new Set(s); });
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

	/** Encrypt file binary data for a channel. Returns encrypted ArrayBuffer. */
	async encryptFile(channelId: string, data: ArrayBuffer): Promise<ArrayBuffer> {
		const key = await this.getSessionKey(channelId);
		if (!key) throw new Error('No encryption key for this channel');
		return encryptBinary(key, data);
	}

	/** Decrypt file binary data for a channel. Returns original ArrayBuffer. */
	async decryptFile(channelId: string, encryptedData: ArrayBuffer): Promise<ArrayBuffer> {
		const key = await this.getSessionKey(channelId);
		if (!key) throw new Error('No decryption key for this channel');
		return decryptBinary(key, encryptedData);
	}

	/** Remove the key for a channel. */
	async clearChannelKey(channelId: string): Promise<void> {
		this.sessionKeyCache.delete(channelId);
		await removeChannelSessionKey(channelId);
		unlockedChannels.update(s => { s.delete(channelId); return new Set(s); });
	}

	/** Clear all E2EE data from this device. */
	async reset(): Promise<void> {
		this.sessionKeyCache.clear();
		await clearAll();
		unlockedChannels.set(new Set());
	}

	/** Check key status for a list of channel IDs and update the reactive store. */
	async refreshKeyStatus(channelIds: string[]): Promise<void> {
		const results = await Promise.all(
			channelIds.map(async (id) => ({ id, has: await this.hasChannelKey(id) }))
		);
		unlockedChannels.update(s => {
			for (const { id, has } of results) {
				if (has) s.add(id); else s.delete(id);
			}
			return new Set(s);
		});
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
