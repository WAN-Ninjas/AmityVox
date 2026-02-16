// Web Crypto API wrapper for AmityVox E2EE.
// Uses PBKDF2 for passphrase-based key derivation, AES-256-GCM for encryption.
// Zero external dependencies — pure Web Crypto API.

// --- Base64 helpers ---

export function arrayBufferToBase64(buffer: ArrayBuffer): string {
	const bytes = new Uint8Array(buffer);
	let binary = '';
	for (let i = 0; i < bytes.byteLength; i++) {
		binary += String.fromCharCode(bytes[i]);
	}
	return btoa(binary);
}

export function base64ToArrayBuffer(base64: string): ArrayBuffer {
	const binary = atob(base64);
	const bytes = new Uint8Array(binary.length);
	for (let i = 0; i < binary.length; i++) {
		bytes[i] = binary.charCodeAt(i);
	}
	return bytes.buffer;
}

// --- Passphrase-Based Key Derivation ---

/**
 * Derive an AES-256-GCM key from a passphrase using PBKDF2.
 * Salt is deterministic per-channel: "amityvox-e2ee-" + channelId.
 * This means the same passphrase + channelId always produces the same key.
 */
export async function deriveKeyFromPassphrase(
	passphrase: string,
	channelId: string
): Promise<CryptoKey> {
	const encoder = new TextEncoder();
	const keyMaterial = await crypto.subtle.importKey(
		'raw',
		encoder.encode(passphrase),
		'PBKDF2',
		false,
		['deriveKey']
	);

	const salt = encoder.encode('amityvox-e2ee-' + channelId);

	return crypto.subtle.deriveKey(
		{
			name: 'PBKDF2',
			salt,
			iterations: 600000,
			hash: 'SHA-256'
		},
		keyMaterial,
		{ name: 'AES-GCM', length: 256 },
		true,
		['encrypt', 'decrypt']
	);
}

// --- AES-256-GCM Session Key ---

/** Generate a random AES-256-GCM key for a channel session. */
export async function generateSessionKey(): Promise<CryptoKey> {
	return crypto.subtle.generateKey({ name: 'AES-GCM', length: 256 }, true, [
		'encrypt',
		'decrypt'
	]);
}

/** Export an AES session key to raw bytes. */
export async function exportSessionKey(key: CryptoKey): Promise<ArrayBuffer> {
	return crypto.subtle.exportKey('raw', key);
}

/** Import an AES session key from raw bytes. */
export async function importSessionKey(rawBytes: ArrayBuffer): Promise<CryptoKey> {
	return crypto.subtle.importKey('raw', rawBytes, { name: 'AES-GCM', length: 256 }, true, [
		'encrypt',
		'decrypt'
	]);
}

// --- AES-GCM Encrypt / Decrypt ---

/**
 * Encrypt plaintext with AES-256-GCM.
 * Returns { ciphertext, iv } where iv is a random 12-byte nonce.
 */
export async function encrypt(
	key: CryptoKey,
	plaintext: string
): Promise<{ ciphertext: ArrayBuffer; iv: Uint8Array }> {
	const iv = crypto.getRandomValues(new Uint8Array(12));
	const encoder = new TextEncoder();
	const ciphertext = await crypto.subtle.encrypt(
		{ name: 'AES-GCM', iv },
		key,
		encoder.encode(plaintext)
	);
	return { ciphertext, iv };
}

/** Decrypt AES-256-GCM ciphertext back to plaintext. */
export async function decrypt(
	key: CryptoKey,
	ciphertext: ArrayBuffer,
	iv: Uint8Array
): Promise<string> {
	const plainBuffer = await crypto.subtle.decrypt({ name: 'AES-GCM', iv }, key, ciphertext);
	return new TextDecoder().decode(plainBuffer);
}

// --- Wire Format ---

/**
 * Encrypt a message and encode it for transport.
 * Wire format: base64(iv_12_bytes || aes_gcm_ciphertext)
 */
export async function encryptForWire(key: CryptoKey, plaintext: string): Promise<string> {
	const { ciphertext, iv } = await encrypt(key, plaintext);
	const combined = new Uint8Array(12 + ciphertext.byteLength);
	combined.set(iv, 0);
	combined.set(new Uint8Array(ciphertext), 12);
	return arrayBufferToBase64(combined.buffer);
}

/**
 * Decrypt a wire-format encrypted message.
 * Expects base64(iv_12_bytes || aes_gcm_ciphertext).
 */
export async function decryptFromWire(key: CryptoKey, wireData: string): Promise<string> {
	const combined = new Uint8Array(base64ToArrayBuffer(wireData));
	const iv = combined.slice(0, 12);
	const ciphertext = combined.slice(12);
	return decrypt(key, ciphertext.buffer, iv);
}
