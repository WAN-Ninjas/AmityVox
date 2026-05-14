import { describe, expect, it, beforeEach } from 'vitest';
import { avatarUrl, fileUrl, setLocalInstanceId } from '../avatar';

describe('avatar/file URL helpers', () => {
	beforeEach(() => {
		setLocalInstanceId(null);
	});

	it('uses local files when no instance is supplied', () => {
		expect(avatarUrl('avatar-1')).toBe('/api/v1/files/avatar-1');
		expect(fileUrl('file-1')).toBe('/api/v1/files/file-1');
	});

	it('does not proxy media for the local instance id', () => {
		setLocalInstanceId('local-1');
		expect(avatarUrl('avatar-1', 'local-1')).toBe('/api/v1/files/avatar-1');
		expect(fileUrl('file-1', 'local-1')).toBe('/api/v1/files/file-1');
	});

	it('uses the federation proxy for remote instance ids', () => {
		setLocalInstanceId('local-1');
		expect(avatarUrl('avatar-1', 'remote-1')).toBe('/api/v1/federation/media/remote-1/avatar-1');
		expect(fileUrl('file-1', 'remote-1')).toBe('/api/v1/federation/media/remote-1/file-1');
	});
});
