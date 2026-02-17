import { describe, it, expect } from 'vitest';
import { isEmojiOnly } from '../emoji';

describe('isEmojiOnly', () => {
	it('returns true for a single emoji', () => {
		expect(isEmojiOnly('😀')).toBe(true);
	});

	it('returns true for multiple emoji', () => {
		expect(isEmojiOnly('😀😂🎉')).toBe(true);
	});

	it('returns true for emoji with spaces', () => {
		expect(isEmojiOnly('😀 😂 🎉')).toBe(true);
	});

	it('returns false for empty string', () => {
		expect(isEmojiOnly('')).toBe(false);
	});

	it('returns false for whitespace only', () => {
		expect(isEmojiOnly('   ')).toBe(false);
	});

	it('returns false for text only', () => {
		expect(isEmojiOnly('hello')).toBe(false);
	});

	it('returns false for mixed text and emoji', () => {
		expect(isEmojiOnly('hello 😀')).toBe(false);
	});

	it('returns false for text after emoji', () => {
		expect(isEmojiOnly('😀 hello')).toBe(false);
	});

	it('returns true for emoji with skin tone modifiers', () => {
		expect(isEmojiOnly('👍🏽')).toBe(true);
		expect(isEmojiOnly('👋🏻👋🏿')).toBe(true);
	});

	it('returns true for flag sequences', () => {
		expect(isEmojiOnly('🇺🇸')).toBe(true);
		expect(isEmojiOnly('🇯🇵🇫🇷')).toBe(true);
	});

	it('returns true for ZWJ sequences (family, etc)', () => {
		expect(isEmojiOnly('❤️')).toBe(true);
		expect(isEmojiOnly('👨‍👩‍👧‍👦')).toBe(true);
	});

	it('returns false for more than maxEmoji emoji', () => {
		const eleven = '😀😀😀😀😀😀😀😀😀😀😀';
		expect(isEmojiOnly(eleven)).toBe(false);
	});

	it('returns true for exactly maxEmoji emoji', () => {
		const ten = '😀😀😀😀😀😀😀😀😀😀';
		expect(isEmojiOnly(ten)).toBe(true);
	});

	it('allows custom maxEmoji', () => {
		expect(isEmojiOnly('😀😀😀', 2)).toBe(false);
		expect(isEmojiOnly('😀😀', 2)).toBe(true);
	});

	it('returns false for numbers', () => {
		expect(isEmojiOnly('123')).toBe(false);
	});

	it('returns false for special characters', () => {
		expect(isEmojiOnly('!!!')).toBe(false);
	});

	it('returns true for common emoji', () => {
		expect(isEmojiOnly('👍')).toBe(true);
		expect(isEmojiOnly('❤️')).toBe(true);
		expect(isEmojiOnly('😂')).toBe(true);
		expect(isEmojiOnly('🎉')).toBe(true);
		expect(isEmojiOnly('😮')).toBe(true);
		expect(isEmojiOnly('😢')).toBe(true);
	});
});
