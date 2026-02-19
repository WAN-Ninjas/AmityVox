import { describe, it, expect } from 'vitest';
import { calculateInsertionIndex, calculateScrollSpeed } from '$lib/utils/dragDrop';

describe('calculateInsertionIndex', () => {
	it('returns 0 when cursor is above all items', () => {
		const rects = [
			{ top: 100, bottom: 140, height: 40 },
			{ top: 140, bottom: 180, height: 40 },
		];
		expect(calculateInsertionIndex(50, rects, -1)).toBe(0);
	});

	it('returns length when cursor is below all items', () => {
		const rects = [
			{ top: 100, bottom: 140, height: 40 },
			{ top: 140, bottom: 180, height: 40 },
		];
		expect(calculateInsertionIndex(200, rects, -1)).toBe(2);
	});

	it('returns index based on midpoint of item', () => {
		const rects = [
			{ top: 0, bottom: 40, height: 40 },
			{ top: 40, bottom: 80, height: 40 },
			{ top: 80, bottom: 120, height: 40 },
		];
		// Cursor at Y=50 is past midpoint of item 1 (midpoint=60), so before item 1
		expect(calculateInsertionIndex(50, rects, -1)).toBe(1);
		// Cursor at Y=70 is past midpoint of item 1, so after item 1
		expect(calculateInsertionIndex(70, rects, -1)).toBe(2);
	});

	it('skips the source item index', () => {
		const rects = [
			{ top: 0, bottom: 40, height: 40 },
			{ top: 40, bottom: 80, height: 40 },
			{ top: 80, bottom: 120, height: 40 },
		];
		// Without skipping source (midpoint=60), 50<60 would return 1.
		// Skipping source at index 1 means next check is index 2 (midpoint=100), 50<100 returns 2.
		expect(calculateInsertionIndex(50, rects, 1)).toBe(2);
	});

	it('returns 0 for empty list', () => {
		expect(calculateInsertionIndex(50, [], -1)).toBe(0);
	});
});

describe('calculateScrollSpeed', () => {
	it('returns 0 when cursor is in the middle of the container', () => {
		expect(calculateScrollSpeed(200, 100, 500, 40)).toBe(0);
	});

	it('returns negative speed when cursor is near the top edge', () => {
		// cursorY=120, containerTop=100, threshold=40 => 20px inside threshold
		const speed = calculateScrollSpeed(120, 100, 500, 40);
		expect(speed).toBeLessThan(0);
	});

	it('returns positive speed when cursor is near the bottom edge', () => {
		// cursorY=480, containerTop=100, containerBottom=500, threshold=40
		const speed = calculateScrollSpeed(480, 100, 500, 40);
		expect(speed).toBeGreaterThan(0);
	});

	it('returns 0 when cursor is outside the container', () => {
		expect(calculateScrollSpeed(50, 100, 500, 40)).toBe(0);
		expect(calculateScrollSpeed(550, 100, 500, 40)).toBe(0);
	});

	it('clamps speed to max value', () => {
		// Cursor right at the edge
		const speed = calculateScrollSpeed(101, 100, 500, 40);
		expect(Math.abs(speed)).toBeLessThanOrEqual(15);
	});
});
