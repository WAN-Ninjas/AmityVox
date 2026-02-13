import { describe, it, expect } from 'vitest';
import { parseHandle, formatHandle } from '$lib/utils/handleUtils';

describe('parseHandle', () => {
	it('parses a simple local handle @username', () => {
		const result = parseHandle('@horatio');
		expect(result).toEqual({ username: 'horatio', domain: null });
	});

	it('parses a federated handle @username@domain', () => {
		const result = parseHandle('@horatio@amityvox.chat');
		expect(result).toEqual({ username: 'horatio', domain: 'amityvox.chat' });
	});

	it('handles input without leading @', () => {
		const result = parseHandle('horatio');
		expect(result).toEqual({ username: 'horatio', domain: null });
	});

	it('handles federated input without leading @', () => {
		const result = parseHandle('horatio@amityvox.chat');
		expect(result).toEqual({ username: 'horatio', domain: 'amityvox.chat' });
	});

	it('returns null for empty string', () => {
		expect(parseHandle('')).toBeNull();
	});

	it('returns null for just @', () => {
		expect(parseHandle('@')).toBeNull();
	});

	it('returns null for @@domain (no username)', () => {
		expect(parseHandle('@@domain.com')).toBeNull();
	});

	it('handles domain with trailing @ stripped to null', () => {
		const result = parseHandle('@user@');
		expect(result).toEqual({ username: 'user', domain: null });
	});

	it('handles complex domains', () => {
		const result = parseHandle('@alice@sub.domain.example.com');
		expect(result).toEqual({ username: 'alice', domain: 'sub.domain.example.com' });
	});

	it('handles usernames with dots and hyphens', () => {
		const result = parseHandle('@user.name-test');
		expect(result).toEqual({ username: 'user.name-test', domain: null });
	});

	it('trims surrounding whitespace', () => {
		const result = parseHandle('  @horatio@amityvox.chat  ');
		expect(result).toEqual({ username: 'horatio', domain: 'amityvox.chat' });
	});

	it('only splits on the first @ after username', () => {
		// This edge case: @user@domain@extra should treat "domain@extra" as the domain
		const result = parseHandle('@user@domain@extra');
		expect(result).toEqual({ username: 'user', domain: 'domain@extra' });
	});
});

describe('formatHandle', () => {
	it('formats a local handle', () => {
		expect(formatHandle('horatio')).toBe('@horatio');
	});

	it('formats a federated handle', () => {
		expect(formatHandle('horatio', 'amityvox.chat')).toBe('@horatio@amityvox.chat');
	});

	it('formats local handle when domain is null', () => {
		expect(formatHandle('horatio', null)).toBe('@horatio');
	});

	it('formats local handle when domain is undefined', () => {
		expect(formatHandle('horatio', undefined)).toBe('@horatio');
	});

	it('formats local handle when domain is empty string', () => {
		expect(formatHandle('horatio', '')).toBe('@horatio');
	});

	it('returns empty string for empty username', () => {
		expect(formatHandle('')).toBe('');
	});
});
