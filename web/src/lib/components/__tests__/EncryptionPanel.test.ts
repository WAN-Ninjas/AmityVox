import { describe, it, expect } from 'vitest';

// Test passphrase validation logic as pure functions since Svelte 5 components
// can't be rendered in happy-dom (SSR mode). These mirror the validation logic
// in EncryptionPanel.svelte.

function validateEnablePassphrase(passphrase: string, confirm: string): string | null {
	if (!passphrase.trim()) return 'Enter a passphrase';
	if (passphrase !== confirm) return 'Passphrases do not match';
	return null;
}

function validateChangePassphrase(newPassphrase: string, confirm: string): string | null {
	if (!newPassphrase.trim()) return 'Enter a new passphrase';
	if (newPassphrase !== confirm) return 'Passphrases do not match';
	return null;
}

function validateUnlockPassphrase(passphrase: string): string | null {
	if (!passphrase.trim()) return 'Enter the channel passphrase';
	return null;
}

describe('EncryptionPanel validation', () => {
	describe('enable passphrase', () => {
		it('rejects empty passphrase', () => {
			expect(validateEnablePassphrase('', '')).toBe('Enter a passphrase');
			expect(validateEnablePassphrase('   ', '   ')).toBe('Enter a passphrase');
		});

		it('rejects mismatched passphrases', () => {
			expect(validateEnablePassphrase('pass1', 'pass2')).toBe('Passphrases do not match');
		});

		it('accepts matching passphrases', () => {
			expect(validateEnablePassphrase('bobaganoosh', 'bobaganoosh')).toBeNull();
		});
	});

	describe('change passphrase', () => {
		it('rejects empty new passphrase', () => {
			expect(validateChangePassphrase('', '')).toBe('Enter a new passphrase');
		});

		it('rejects mismatched passphrases', () => {
			expect(validateChangePassphrase('new1', 'new2')).toBe('Passphrases do not match');
		});

		it('accepts matching passphrases', () => {
			expect(validateChangePassphrase('secret', 'secret')).toBeNull();
		});
	});

	describe('unlock passphrase', () => {
		it('rejects empty passphrase', () => {
			expect(validateUnlockPassphrase('')).toBe('Enter the channel passphrase');
			expect(validateUnlockPassphrase('  ')).toBe('Enter the channel passphrase');
		});

		it('accepts non-empty passphrase', () => {
			expect(validateUnlockPassphrase('any-phrase')).toBeNull();
		});
	});
});

describe('encryption state transitions', () => {
	// Model the state machine: not_encrypted -> enabling -> encrypted_locked -> encrypted_unlocked -> disabling -> not_encrypted
	type EncState = 'not_encrypted' | 'encrypted_locked' | 'encrypted_unlocked';

	function getState(encrypted: boolean, hasKey: boolean): EncState {
		if (!encrypted) return 'not_encrypted';
		return hasKey ? 'encrypted_unlocked' : 'encrypted_locked';
	}

	it('starts as not_encrypted', () => {
		expect(getState(false, false)).toBe('not_encrypted');
	});

	it('encrypted without key is locked', () => {
		expect(getState(true, false)).toBe('encrypted_locked');
	});

	it('encrypted with key is unlocked', () => {
		expect(getState(true, true)).toBe('encrypted_unlocked');
	});

	it('disable returns to not_encrypted regardless of key', () => {
		expect(getState(false, true)).toBe('not_encrypted');
		expect(getState(false, false)).toBe('not_encrypted');
	});
});
