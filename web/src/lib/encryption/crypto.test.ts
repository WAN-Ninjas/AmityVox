import { describe, it, expect } from 'vitest';
import {
	arrayBufferToBase64,
	base64ToArrayBuffer,
	deriveKeyFromPassphrase,
	generateSessionKey,
	exportSessionKey,
	importSessionKey,
	encrypt,
	decrypt,
	encryptForWire,
	decryptFromWire
} from './crypto';

describe('base64 helpers', () => {
	it('round-trips ArrayBuffer through base64', () => {
		const data = new Uint8Array([1, 2, 3, 4, 5, 255, 0, 128]);
		const b64 = arrayBufferToBase64(data.buffer);
		const result = new Uint8Array(base64ToArrayBuffer(b64));
		expect(result).toEqual(data);
	});

	it('handles empty buffer', () => {
		const empty = new Uint8Array([]);
		const b64 = arrayBufferToBase64(empty.buffer);
		expect(b64).toBe('');
		const result = new Uint8Array(base64ToArrayBuffer(b64));
		expect(result.length).toBe(0);
	});
});

describe('deriveKeyFromPassphrase', () => {
	it('produces consistent key for same passphrase and channelId', async () => {
		const key1 = await deriveKeyFromPassphrase('bobaganoosh', 'channel-123');
		const key2 = await deriveKeyFromPassphrase('bobaganoosh', 'channel-123');

		const raw1 = await crypto.subtle.exportKey('raw', key1);
		const raw2 = await crypto.subtle.exportKey('raw', key2);

		expect(arrayBufferToBase64(raw1)).toBe(arrayBufferToBase64(raw2));
	});

	it('produces different keys for different passphrases', async () => {
		const key1 = await deriveKeyFromPassphrase('passphrase-one', 'channel-123');
		const key2 = await deriveKeyFromPassphrase('passphrase-two', 'channel-123');

		const raw1 = await crypto.subtle.exportKey('raw', key1);
		const raw2 = await crypto.subtle.exportKey('raw', key2);

		expect(arrayBufferToBase64(raw1)).not.toBe(arrayBufferToBase64(raw2));
	});

	it('produces different keys for different channelIds (domain separation)', async () => {
		const key1 = await deriveKeyFromPassphrase('same-passphrase', 'channel-aaa');
		const key2 = await deriveKeyFromPassphrase('same-passphrase', 'channel-bbb');

		const raw1 = await crypto.subtle.exportKey('raw', key1);
		const raw2 = await crypto.subtle.exportKey('raw', key2);

		expect(arrayBufferToBase64(raw1)).not.toBe(arrayBufferToBase64(raw2));
	});

	it('returns an AES-256-GCM key with encrypt/decrypt usage', async () => {
		const key = await deriveKeyFromPassphrase('test', 'ch-1');
		expect(key.algorithm).toEqual({ name: 'AES-GCM', length: 256 });
		expect(key.usages).toContain('encrypt');
		expect(key.usages).toContain('decrypt');
	});

	it('handles empty passphrase', async () => {
		const key = await deriveKeyFromPassphrase('', 'channel-123');
		expect(key).toBeTruthy();
		expect(key.type).toBe('secret');
	});

	it('derived key can encrypt and decrypt round-trip', async () => {
		const key = await deriveKeyFromPassphrase('my-secret', 'channel-xyz');
		const plaintext = 'Hello, encrypted world!';

		const { ciphertext, iv } = await encrypt(key, plaintext);
		const decrypted = await decrypt(key, ciphertext, iv);

		expect(decrypted).toBe(plaintext);
	});
});

describe('session key', () => {
	it('generates, exports, and imports a session key', async () => {
		const key = await generateSessionKey();
		const raw = await exportSessionKey(key);
		expect(raw.byteLength).toBe(32); // 256 bits

		const imported = await importSessionKey(raw);
		const reExported = await exportSessionKey(imported);
		expect(arrayBufferToBase64(raw)).toBe(arrayBufferToBase64(reExported));
	});
});

describe('AES-GCM encrypt/decrypt', () => {
	it('encrypts and decrypts plaintext', async () => {
		const key = await generateSessionKey();
		const plaintext = 'The quick brown fox jumps over the lazy dog';

		const { ciphertext, iv } = await encrypt(key, plaintext);
		expect(ciphertext.byteLength).toBeGreaterThan(0);
		expect(iv.byteLength).toBe(12);

		const decrypted = await decrypt(key, ciphertext, iv);
		expect(decrypted).toBe(plaintext);
	});

	it('fails to decrypt with wrong key', async () => {
		const key1 = await generateSessionKey();
		const key2 = await generateSessionKey();
		const plaintext = 'secret message';

		const { ciphertext, iv } = await encrypt(key1, plaintext);

		await expect(decrypt(key2, ciphertext, iv)).rejects.toThrow();
	});

	it('handles unicode text', async () => {
		const key = await generateSessionKey();
		const plaintext = 'Hello 🌍 Привет мир 你好世界';

		const { ciphertext, iv } = await encrypt(key, plaintext);
		const decrypted = await decrypt(key, ciphertext, iv);
		expect(decrypted).toBe(plaintext);
	});

	it('handles empty string', async () => {
		const key = await generateSessionKey();
		const { ciphertext, iv } = await encrypt(key, '');
		const decrypted = await decrypt(key, ciphertext, iv);
		expect(decrypted).toBe('');
	});
});

describe('wire format', () => {
	it('encryptForWire and decryptFromWire round-trip', async () => {
		const key = await generateSessionKey();
		const plaintext = 'Wire format test message';

		const wire = await encryptForWire(key, plaintext);
		expect(typeof wire).toBe('string');
		expect(wire.length).toBeGreaterThan(0);

		const decrypted = await decryptFromWire(key, wire);
		expect(decrypted).toBe(plaintext);
	});

	it('wire format is base64(12-byte IV + ciphertext)', async () => {
		const key = await generateSessionKey();
		const wire = await encryptForWire(key, 'test');

		const decoded = new Uint8Array(base64ToArrayBuffer(wire));
		expect(decoded.byteLength).toBeGreaterThan(12);

		// First 12 bytes are IV
		const iv = decoded.slice(0, 12);
		const ct = decoded.slice(12);
		const decrypted = await decrypt(key, ct.buffer, iv);
		expect(decrypted).toBe('test');
	});

	it('different encryptions of same plaintext produce different wire output (random IV)', async () => {
		const key = await generateSessionKey();
		const wire1 = await encryptForWire(key, 'same message');
		const wire2 = await encryptForWire(key, 'same message');

		// Different IVs mean different ciphertext
		expect(wire1).not.toBe(wire2);

		// But both decrypt to the same plaintext
		expect(await decryptFromWire(key, wire1)).toBe('same message');
		expect(await decryptFromWire(key, wire2)).toBe('same message');
	});
});
