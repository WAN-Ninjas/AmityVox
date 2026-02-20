import { describe, it, expect } from 'vitest';
import { urlBase64ToUint8Array } from '../pushNotifications';

describe('urlBase64ToUint8Array', () => {
	it('converts a base64url string to Uint8Array', () => {
		// Known base64url string → expected bytes.
		const input = 'SGVsbG8';  // "Hello" in base64url
		const result = urlBase64ToUint8Array(input);
		expect(result).toBeInstanceOf(Uint8Array);
		expect(result.length).toBe(5);
		expect(Array.from(result)).toEqual([72, 101, 108, 108, 111]); // H, e, l, l, o
	});

	it('handles base64url characters (- and _)', () => {
		// '+' in base64 is '-' in base64url, '/' is '_'
		const input = 'ab-c_d';
		const result = urlBase64ToUint8Array(input);
		expect(result).toBeInstanceOf(Uint8Array);
		expect(result.length).toBeGreaterThan(0);
	});

	it('handles strings that need padding', () => {
		const input = 'YQ'; // "a" without padding
		const result = urlBase64ToUint8Array(input);
		expect(result.length).toBe(1);
		expect(result[0]).toBe(97); // 'a'
	});

	it('handles empty string', () => {
		const result = urlBase64ToUint8Array('');
		expect(result).toBeInstanceOf(Uint8Array);
		expect(result.length).toBe(0);
	});

	it('handles VAPID-like key', () => {
		// Real VAPID public keys are 65 bytes (uncompressed P-256 point).
		const vapidKey = 'BNcRdreALRFXTkOOUHK1EtK2wtaz5Ry4YfYCA_0QTpQtUbVlUls0VJXg7A8u-Ts1XbjhazAkj7I99e8QcYP7DkM';
		const result = urlBase64ToUint8Array(vapidKey);
		expect(result).toBeInstanceOf(Uint8Array);
		expect(result.length).toBe(65);
	});
});
