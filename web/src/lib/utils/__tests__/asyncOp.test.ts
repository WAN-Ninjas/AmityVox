import { describe, it, expect, vi } from 'vitest';
import { createAsyncOp } from '$lib/utils/asyncOp';

describe('createAsyncOp', () => {
	it('initializes with loading=false and error=null', () => {
		const op = createAsyncOp();
		expect(op.loading).toBe(false);
		expect(op.error).toBeNull();
	});

	it('sets loading=true during execution', async () => {
		const op = createAsyncOp();
		let wasLoadingDuringRun = false;

		await op.run(async () => {
			wasLoadingDuringRun = op.loading;
		});

		expect(wasLoadingDuringRun).toBe(true);
		expect(op.loading).toBe(false);
	});

	it('returns the value from fn on success', async () => {
		const op = createAsyncOp();
		const result = await op.run(async () => 42);
		expect(result).toBe(42);
		expect(op.error).toBeNull();
	});

	it('returns undefined and sets error on failure', async () => {
		const op = createAsyncOp();
		const result = await op.run(async () => {
			throw new Error('something broke');
		});
		expect(result).toBeUndefined();
		expect(op.error).toBe('something broke');
		expect(op.loading).toBe(false);
	});

	it('calls onError callback on failure', async () => {
		const op = createAsyncOp();
		const onError = vi.fn();
		await op.run(async () => {
			throw new Error('oops');
		}, onError);
		expect(onError).toHaveBeenCalledWith('oops');
	});

	it('clears previous error on new run', async () => {
		const op = createAsyncOp();
		await op.run(async () => { throw new Error('first'); });
		expect(op.error).toBe('first');

		await op.run(async () => 'ok');
		expect(op.error).toBeNull();
	});

	it('handles non-Error throws with fallback message', async () => {
		const op = createAsyncOp();
		await op.run(async () => {
			throw 'string error';
		});
		expect(op.error).toBe('An error occurred');
	});

	it('handles errors with no message property', async () => {
		const op = createAsyncOp();
		await op.run(async () => {
			throw { code: 'ERR' };
		});
		expect(op.error).toBe('An error occurred');
	});

	it('resets loading on error', async () => {
		const op = createAsyncOp();
		await op.run(async () => { throw new Error('fail'); });
		expect(op.loading).toBe(false);
	});

	it('supports setting error manually', () => {
		const op = createAsyncOp();
		op.error = 'manual error';
		expect(op.error).toBe('manual error');
		op.error = null;
		expect(op.error).toBeNull();
	});

	it('supports multiple sequential runs', async () => {
		const op = createAsyncOp();
		const r1 = await op.run(async () => 1);
		const r2 = await op.run(async () => 2);
		const r3 = await op.run(async () => 3);
		expect(r1).toBe(1);
		expect(r2).toBe(2);
		expect(r3).toBe(3);
		expect(op.loading).toBe(false);
		expect(op.error).toBeNull();
	});
});
