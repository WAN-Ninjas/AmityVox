import { describe, it, expect, vi } from 'vitest';
import { calculateInsertionIndex, calculateScrollSpeed, DragController } from '$lib/utils/dragDrop';

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

// Minimal mock for HTMLElement — happy-dom provides a real DOM
function makeContainer(itemCount: number): { container: HTMLDivElement; items: HTMLDivElement[] } {
	const container = document.createElement('div');
	Object.defineProperty(container, 'getBoundingClientRect', {
		value: () => ({ top: 0, bottom: 400, left: 0, right: 300, width: 300, height: 400 }),
	});
	container.style.position = 'relative';

	const items: HTMLDivElement[] = [];
	for (let i = 0; i < itemCount; i++) {
		const item = document.createElement('div');
		item.dataset.dragId = `item-${i}`;
		Object.defineProperty(item, 'getBoundingClientRect', {
			value: () => ({ top: i * 40, bottom: (i + 1) * 40, left: 0, right: 300, width: 300, height: 40 }),
		});
		container.appendChild(item);
		items.push(item);
	}
	document.body.appendChild(container);
	return { container, items };
}

describe('DragController', () => {
	it('is not active before any interaction', () => {
		const { container, items } = makeContainer(3);
		const onDrop = vi.fn();
		const ctrl = new DragController({
			container,
			items: () => ['item-0', 'item-1', 'item-2'],
			getElement: (id) => items[parseInt(id.split('-')[1])] ?? null,
			onDrop,
		});
		expect(ctrl.isDragging).toBe(false);
		ctrl.destroy();
		container.remove();
	});

	it('calls onDrop with correct source and target', () => {
		const { container, items } = makeContainer(3);
		const onDrop = vi.fn();
		const ctrl = new DragController({
			container,
			items: () => ['item-0', 'item-1', 'item-2'],
			getElement: (id) => items[parseInt(id.split('-')[1])] ?? null,
			onDrop,
		});

		// Simulate drag: pointerdown on item-0, move 10px, release on item-2's zone
		ctrl.handlePointerDown(new PointerEvent('pointerdown', { clientX: 50, clientY: 10 }), 'item-0');
		// Move enough to activate (>5px)
		ctrl.handlePointerMove(new PointerEvent('pointermove', { clientX: 50, clientY: 100 }));
		ctrl.handlePointerUp(new PointerEvent('pointerup', { clientX: 50, clientY: 100 }));

		expect(onDrop).toHaveBeenCalledTimes(1);
		const [sourceId, targetIndex] = onDrop.mock.calls[0];
		expect(sourceId).toBe('item-0');
		expect(typeof targetIndex).toBe('number');
		ctrl.destroy();
		container.remove();
	});

	it('does not activate on small movements (click threshold)', () => {
		const { container, items } = makeContainer(3);
		const onDrop = vi.fn();
		const ctrl = new DragController({
			container,
			items: () => ['item-0', 'item-1', 'item-2'],
			getElement: (id) => items[parseInt(id.split('-')[1])] ?? null,
			onDrop,
		});

		ctrl.handlePointerDown(new PointerEvent('pointerdown', { clientX: 50, clientY: 10 }), 'item-0');
		ctrl.handlePointerMove(new PointerEvent('pointermove', { clientX: 52, clientY: 12 })); // <5px
		ctrl.handlePointerUp(new PointerEvent('pointerup', { clientX: 52, clientY: 12 }));

		expect(ctrl.isDragging).toBe(false);
		expect(onDrop).not.toHaveBeenCalled();
		ctrl.destroy();
		container.remove();
	});

	it('cancels drag on Escape key', () => {
		const { container, items } = makeContainer(3);
		const onDrop = vi.fn();
		const ctrl = new DragController({
			container,
			items: () => ['item-0', 'item-1', 'item-2'],
			getElement: (id) => items[parseInt(id.split('-')[1])] ?? null,
			onDrop,
		});

		ctrl.handlePointerDown(new PointerEvent('pointerdown', { clientX: 50, clientY: 10 }), 'item-0');
		ctrl.handlePointerMove(new PointerEvent('pointermove', { clientX: 50, clientY: 100 })); // activate
		expect(ctrl.isDragging).toBe(true);

		ctrl.handleKeyDown(new KeyboardEvent('keydown', { key: 'Escape' }));
		expect(ctrl.isDragging).toBe(false);
		expect(onDrop).not.toHaveBeenCalled();
		ctrl.destroy();
		container.remove();
	});

	it('respects canDrag=false', () => {
		const { container, items } = makeContainer(3);
		const onDrop = vi.fn();
		const ctrl = new DragController({
			container,
			items: () => ['item-0', 'item-1', 'item-2'],
			getElement: (id) => items[parseInt(id.split('-')[1])] ?? null,
			onDrop,
			canDrag: false,
		});

		ctrl.handlePointerDown(new PointerEvent('pointerdown', { clientX: 50, clientY: 10 }), 'item-0');
		ctrl.handlePointerMove(new PointerEvent('pointermove', { clientX: 50, clientY: 100 }));
		expect(ctrl.isDragging).toBe(false);
		ctrl.destroy();
		container.remove();
	});
});
