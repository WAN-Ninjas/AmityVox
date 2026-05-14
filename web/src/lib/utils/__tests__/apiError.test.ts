import { describe, expect, it } from 'vitest';
import { ApiRequestError } from '$lib/api/client';
import { getErrorMessage } from '../apiError';

describe('getErrorMessage', () => {
	it('adds useful context for permission errors', () => {
		const error = new ApiRequestError('Missing permission', 'forbidden', 403);
		expect(getErrorMessage(error, 'Fallback')).toBe('Permission denied: Missing permission');
	});

	it('uses normal Error messages', () => {
		expect(getErrorMessage(new Error('Network failed'), 'Fallback')).toBe('Network failed');
	});

	it('falls back for unknown errors', () => {
		expect(getErrorMessage('bad', 'Fallback')).toBe('Fallback');
	});
});
