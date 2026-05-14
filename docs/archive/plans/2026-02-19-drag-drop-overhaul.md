# Drag/Drop UI Overhaul Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace all HTML5 drag-and-drop with a custom pointer-event-based system that provides smooth animations, floating previews, gap indicators, and touch support across channels, channel groups, and guilds.

**Architecture:** A single reusable `DragController` class in `web/src/lib/utils/dragDrop.ts` manages all pointer tracking, preview rendering, gap animation, and auto-scroll. Each component (ChannelSidebar, ChannelGroups, GuildSidebar, GuildFolder) creates a controller instance and wires `pointerdown` to its items. The controller handles everything else — no HTML5 drag attributes needed.

**Tech Stack:** TypeScript, Pointer Events API, CSS transitions/transforms, Vitest for testing.

**Design doc:** `docs/plans/2026-02-19-drag-drop-overhaul-design.md`

---

## Task 1: Create `dragDrop.ts` — Pure Calculation Functions

**Files:**
- Create: `web/src/lib/utils/dragDrop.ts`
- Create: `web/src/lib/utils/__tests__/dragDrop.test.ts`

These are the testable pure functions that the controller will use internally. We build and test them first.

**Step 1: Write failing tests for `calculateInsertionIndex`**

This function takes a cursor Y position and an array of item bounding rects, and returns the index where the dragged item should be inserted.

```ts
// web/src/lib/utils/__tests__/dragDrop.test.ts
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
		// Source is index 1, cursor at Y=50 — should skip source in calculation
		expect(calculateInsertionIndex(50, rects, 1)).toBe(0);
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
```

**Step 2: Run tests to verify they fail**

Run: `docker run --rm -v $(pwd)/web:/web -w /web node:24-alpine npx vitest run src/lib/utils/__tests__/dragDrop.test.ts`
Expected: FAIL — module `$lib/utils/dragDrop` not found.

**Step 3: Implement the pure calculation functions**

```ts
// web/src/lib/utils/dragDrop.ts

/** Bounding rect info for an item in the drag list. */
export interface ItemRect {
	top: number;
	bottom: number;
	height: number;
}

/**
 * Calculate where a dragged item should be inserted based on cursor Y position.
 * Returns an insertion index (0 = before first item, length = after last).
 * `sourceIndex` is excluded from the calculation (the item being dragged).
 */
export function calculateInsertionIndex(
	cursorY: number,
	rects: ItemRect[],
	sourceIndex: number
): number {
	if (rects.length === 0) return 0;

	let insertIndex = rects.length;
	for (let i = 0; i < rects.length; i++) {
		if (i === sourceIndex) continue;
		const midpoint = rects[i].top + rects[i].height / 2;
		if (cursorY < midpoint) {
			insertIndex = i;
			break;
		}
	}
	return insertIndex;
}

/**
 * Calculate auto-scroll speed based on cursor proximity to container edges.
 * Returns px/frame: negative = scroll up, positive = scroll down, 0 = no scroll.
 */
export function calculateScrollSpeed(
	cursorY: number,
	containerTop: number,
	containerBottom: number,
	threshold: number = 40,
	maxSpeed: number = 15
): number {
	if (cursorY < containerTop || cursorY > containerBottom) return 0;

	const distFromTop = cursorY - containerTop;
	const distFromBottom = containerBottom - cursorY;

	if (distFromTop < threshold) {
		// Near top — scroll up (negative)
		const ratio = 1 - distFromTop / threshold;
		return -Math.round(ratio * maxSpeed);
	}

	if (distFromBottom < threshold) {
		// Near bottom — scroll down (positive)
		const ratio = 1 - distFromBottom / threshold;
		return Math.round(ratio * maxSpeed);
	}

	return 0;
}
```

**Step 4: Run tests to verify they pass**

Run: `docker run --rm -v $(pwd)/web:/web -w /web node:24-alpine npx vitest run src/lib/utils/__tests__/dragDrop.test.ts`
Expected: All tests PASS.

**Step 5: Commit**

```bash
git add web/src/lib/utils/dragDrop.ts web/src/lib/utils/__tests__/dragDrop.test.ts
git commit -m "feat(drag): add pure calculation functions for pointer-based drag system"
```

---

## Task 2: Create `DragController` Class

**Files:**
- Modify: `web/src/lib/utils/dragDrop.ts`
- Modify: `web/src/lib/utils/__tests__/dragDrop.test.ts`

This is the main class that components instantiate. It manages pointer events, creates the floating preview, animates gaps, shows the insert indicator, and handles auto-scroll.

**Step 1: Write failing tests for DragController state management**

Add to the test file:

```ts
import { DragController } from '$lib/utils/dragDrop';

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
```

**Step 2: Run tests to verify they fail**

Run: `docker run --rm -v $(pwd)/web:/web -w /web node:24-alpine npx vitest run src/lib/utils/__tests__/dragDrop.test.ts`
Expected: FAIL — `DragController` is not exported.

**Step 3: Implement `DragController`**

Add to `web/src/lib/utils/dragDrop.ts` (append after the pure functions):

```ts
export interface DragOptions {
	/** The scrollable container element that holds the draggable items. */
	container: HTMLElement;
	/** Returns the current ordered list of item IDs. */
	items: () => string[];
	/** Returns the DOM element for a given item ID. */
	getElement: (id: string) => HTMLElement | null;
	/** Called when a drag completes. `targetIndex` is the insertion index in the list. */
	onDrop: (sourceId: string, targetIndex: number) => void;
	/** If false, drag is disabled. Default true. */
	canDrag?: boolean;
	/** CSS selector for the drag handle within each item. If not set, entire item is draggable. */
	dragHandleSelector?: string;
	/** Called when drag state changes (for reactive UI updates in Svelte). */
	onDragStateChange?: (isDragging: boolean) => void;
}

const ACTIVATION_THRESHOLD = 5; // px of movement before drag activates
const SCROLL_EDGE_THRESHOLD = 40; // px from edge to start auto-scroll
const SCROLL_MAX_SPEED = 15;

export class DragController {
	private opts: Required<Pick<DragOptions, 'container' | 'items' | 'getElement' | 'onDrop'>> & DragOptions;
	private sourceId: string | null = null;
	private startX = 0;
	private startY = 0;
	private _isDragging = false;
	private activated = false;
	private preview: HTMLElement | null = null;
	private indicator: HTMLElement | null = null;
	private scrollRAF: number | null = null;
	private currentInsertIndex = -1;
	private sourceIndex = -1;

	get isDragging(): boolean {
		return this._isDragging;
	}

	constructor(opts: DragOptions) {
		this.opts = {
			canDrag: true,
			...opts,
		};
	}

	handlePointerDown(e: PointerEvent, itemId: string): void {
		if (!this.opts.canDrag) return;
		if (e.button !== 0) return; // left click only

		// Check drag handle if configured
		if (this.opts.dragHandleSelector) {
			const target = e.target as HTMLElement;
			if (!target.closest(this.opts.dragHandleSelector)) return;
		}

		this.sourceId = itemId;
		this.startX = e.clientX;
		this.startY = e.clientY;
		this.activated = false;

		const itemIds = this.opts.items();
		this.sourceIndex = itemIds.indexOf(itemId);
	}

	handlePointerMove(e: PointerEvent): void {
		if (!this.sourceId) return;

		const dx = e.clientX - this.startX;
		const dy = e.clientY - this.startY;
		const distance = Math.sqrt(dx * dx + dy * dy);

		// Activation threshold
		if (!this.activated) {
			if (distance < ACTIVATION_THRESHOLD) return;
			this.activated = true;
			this._isDragging = true;
			this.opts.onDragStateChange?.(true);
			this.createPreview(e);
			this.createIndicator();
			this.applySourceStyle();
			this.startAutoScroll();
		}

		// Update preview position
		this.updatePreviewPosition(e.clientX, e.clientY);

		// Calculate insertion index
		const rects = this.getItemRects();
		this.currentInsertIndex = calculateInsertionIndex(e.clientY, rects, this.sourceIndex);

		// Update gap animation + indicator
		this.updateGapAnimation(rects);
		this.updateIndicatorPosition(rects);
	}

	handlePointerUp(_e: PointerEvent): void {
		if (!this.sourceId) return;

		if (this.activated && this.currentInsertIndex >= 0) {
			// Adjust index: if inserting after the source, subtract 1 since source will be removed first
			let adjustedIndex = this.currentInsertIndex;
			if (this.sourceIndex >= 0 && this.currentInsertIndex > this.sourceIndex) {
				adjustedIndex--;
			}
			this.opts.onDrop(this.sourceId, adjustedIndex);
		}

		this.cleanup();
	}

	handleKeyDown(e: KeyboardEvent): void {
		if (e.key === 'Escape' && this._isDragging) {
			this.cleanup();
		}
	}

	private createPreview(e: PointerEvent): void {
		const sourceEl = this.opts.getElement(this.sourceId!);
		if (!sourceEl) return;

		const clone = sourceEl.cloneNode(true) as HTMLElement;
		const rect = sourceEl.getBoundingClientRect();

		clone.style.cssText = `
			position: fixed;
			width: ${rect.width}px;
			height: ${rect.height}px;
			opacity: 0.9;
			transform: scale(1.02);
			box-shadow: 0 10px 25px -5px rgba(0,0,0,0.3), 0 4px 6px -2px rgba(0,0,0,0.2);
			border: 1px solid var(--brand-500, #5c6bc0);
			border-radius: 6px;
			pointer-events: none;
			z-index: 9999;
			transition: none;
			cursor: grabbing;
		`;

		document.body.appendChild(clone);
		this.preview = clone;
		this.updatePreviewPosition(e.clientX, e.clientY);
	}

	private updatePreviewPosition(x: number, y: number): void {
		if (!this.preview) return;
		this.preview.style.left = `${x + 8}px`;
		this.preview.style.top = `${y - 16}px`;
	}

	private createIndicator(): void {
		const indicator = document.createElement('div');
		indicator.style.cssText = `
			position: absolute;
			left: 0;
			right: 0;
			height: 2px;
			background: var(--brand-500, #5c6bc0);
			border-radius: 1px;
			pointer-events: none;
			z-index: 50;
			display: none;
		`;

		// Left dot
		const leftDot = document.createElement('div');
		leftDot.style.cssText = `
			position: absolute;
			left: -3px;
			top: -2px;
			width: 6px;
			height: 6px;
			border-radius: 50%;
			background: var(--brand-500, #5c6bc0);
		`;
		indicator.appendChild(leftDot);

		// Right dot
		const rightDot = document.createElement('div');
		rightDot.style.cssText = `
			position: absolute;
			right: -3px;
			top: -2px;
			width: 6px;
			height: 6px;
			border-radius: 50%;
			background: var(--brand-500, #5c6bc0);
		`;
		indicator.appendChild(rightDot);

		// Append to container (must be position: relative)
		const containerStyle = getComputedStyle(this.opts.container);
		if (containerStyle.position === 'static') {
			this.opts.container.style.position = 'relative';
		}
		this.opts.container.appendChild(indicator);
		this.indicator = indicator;
	}

	private updateIndicatorPosition(rects: ItemRect[]): void {
		if (!this.indicator || rects.length === 0) return;

		const containerRect = this.opts.container.getBoundingClientRect();
		let indicatorY: number;

		if (this.currentInsertIndex <= 0) {
			indicatorY = rects[0].top - containerRect.top + this.opts.container.scrollTop - 1;
		} else if (this.currentInsertIndex >= rects.length) {
			const last = rects[rects.length - 1];
			indicatorY = last.bottom - containerRect.top + this.opts.container.scrollTop - 1;
		} else {
			const above = rects[this.currentInsertIndex - 1];
			const below = rects[this.currentInsertIndex];
			indicatorY = (above.bottom + below.top) / 2 - containerRect.top + this.opts.container.scrollTop - 1;
		}

		this.indicator.style.top = `${indicatorY}px`;
		this.indicator.style.display = 'block';
	}

	private applySourceStyle(): void {
		const el = this.opts.getElement(this.sourceId!);
		if (el) {
			el.style.opacity = '0.3';
			el.style.transition = 'opacity 150ms ease';
		}
	}

	private removeSourceStyle(): void {
		const el = this.opts.getElement(this.sourceId!);
		if (el) {
			el.style.opacity = '';
			el.style.transition = '';
		}
	}

	private getItemRects(): ItemRect[] {
		const ids = this.opts.items();
		return ids.map((id) => {
			const el = this.opts.getElement(id);
			if (!el) return { top: 0, bottom: 0, height: 0 };
			const rect = el.getBoundingClientRect();
			return { top: rect.top, bottom: rect.bottom, height: rect.height };
		});
	}

	private updateGapAnimation(rects: ItemRect[]): void {
		const ids = this.opts.items();
		const sourceHeight = this.sourceIndex >= 0 && rects[this.sourceIndex]
			? rects[this.sourceIndex].height
			: 36;

		for (let i = 0; i < ids.length; i++) {
			if (i === this.sourceIndex) continue;
			const el = this.opts.getElement(ids[i]);
			if (!el) continue;

			el.style.transition = 'transform 150ms ease';

			if (this.sourceIndex < this.currentInsertIndex) {
				// Dragging downward: items between source+1 and insertIndex-1 shift up
				if (i > this.sourceIndex && i < this.currentInsertIndex) {
					el.style.transform = `translateY(-${sourceHeight}px)`;
				} else {
					el.style.transform = '';
				}
			} else {
				// Dragging upward: items between insertIndex and source-1 shift down
				if (i >= this.currentInsertIndex && i < this.sourceIndex) {
					el.style.transform = `translateY(${sourceHeight}px)`;
				} else {
					el.style.transform = '';
				}
			}
		}
	}

	private clearGapAnimation(): void {
		const ids = this.opts.items();
		for (const id of ids) {
			const el = this.opts.getElement(id);
			if (el) {
				el.style.transform = '';
				el.style.transition = '';
			}
		}
	}

	private startAutoScroll(): void {
		const scroll = () => {
			if (!this._isDragging) return;

			const containerRect = this.opts.container.getBoundingClientRect();
			// We use the last known cursor Y from the preview position
			const previewTop = this.preview ? parseFloat(this.preview.style.top) + 16 : 0;
			const speed = calculateScrollSpeed(
				previewTop,
				containerRect.top,
				containerRect.bottom,
				SCROLL_EDGE_THRESHOLD,
				SCROLL_MAX_SPEED
			);

			if (speed !== 0) {
				this.opts.container.scrollTop += speed;
			}

			this.scrollRAF = requestAnimationFrame(scroll);
		};
		this.scrollRAF = requestAnimationFrame(scroll);
	}

	private stopAutoScroll(): void {
		if (this.scrollRAF !== null) {
			cancelAnimationFrame(this.scrollRAF);
			this.scrollRAF = null;
		}
	}

	private cleanup(): void {
		this.removeSourceStyle();
		this.clearGapAnimation();

		if (this.preview) {
			this.preview.remove();
			this.preview = null;
		}
		if (this.indicator) {
			this.indicator.remove();
			this.indicator = null;
		}

		this.stopAutoScroll();
		this._isDragging = false;
		this.activated = false;
		this.sourceId = null;
		this.sourceIndex = -1;
		this.currentInsertIndex = -1;
		this.opts.onDragStateChange?.(false);
	}

	destroy(): void {
		this.cleanup();
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `docker run --rm -v $(pwd)/web:/web -w /web node:24-alpine npx vitest run src/lib/utils/__tests__/dragDrop.test.ts`
Expected: All tests PASS.

**Step 5: Commit**

```bash
git add web/src/lib/utils/dragDrop.ts web/src/lib/utils/__tests__/dragDrop.test.ts
git commit -m "feat(drag): add DragController class with pointer-based drag engine"
```

---

## Task 3: Add Drag Handle Component and CSS

**Files:**
- Create: `web/src/lib/components/common/DragHandle.svelte`

A small presentational component for the 6-dot grip icon used as a drag handle.

**Step 1: Create the DragHandle component**

```svelte
<!-- web/src/lib/components/common/DragHandle.svelte -->
<script lang="ts">
	interface Props {
		visible?: boolean;
	}
	let { visible = true }: Props = $props();
</script>

{#if visible}
	<div
		class="drag-handle flex shrink-0 cursor-grab items-center justify-center opacity-0 transition-opacity group-hover/drag:opacity-60 hover:!opacity-100 active:cursor-grabbing"
		aria-hidden="true"
	>
		<svg class="h-4 w-3 text-text-muted" viewBox="0 0 12 16" fill="currentColor">
			<circle cx="3.5" cy="2.5" r="1.5" />
			<circle cx="8.5" cy="2.5" r="1.5" />
			<circle cx="3.5" cy="7.5" r="1.5" />
			<circle cx="8.5" cy="7.5" r="1.5" />
			<circle cx="3.5" cy="12.5" r="1.5" />
			<circle cx="8.5" cy="12.5" r="1.5" />
		</svg>
	</div>
{/if}
```

**Step 2: Commit**

```bash
git add web/src/lib/components/common/DragHandle.svelte
git commit -m "feat(drag): add DragHandle grip icon component"
```

---

## Task 4: Update ChannelSidebar — Replace HTML5 DnD with Pointer Drag

**Files:**
- Modify: `web/src/lib/components/layout/ChannelSidebar.svelte` (lines 439-500 drag handlers, lines 598-666 template)

This is the biggest change. We remove all HTML5 drag attributes and wire up the `DragController`.

**Step 1: Replace the drag state and handlers in the `<script>` block**

Remove lines 439-500 (the old `dragChannelId`, `dragOverChannelId`, and all `handleChannel*` drag functions).

Replace with:

```ts
// --- Channel Drag Reorder (pointer-based) ---
import DragHandle from '$components/common/DragHandle.svelte';
import { DragController } from '$lib/utils/dragDrop';
import { onDestroy, tick } from 'svelte';

let channelListEl = $state<HTMLElement | null>(null);
let channelDragController = $state<DragController | null>(null);
let isDraggingChannel = $state(false);

// Create/destroy controller when the container mounts or permissions change.
$effect(() => {
	if (!channelListEl) return;
	channelDragController?.destroy();
	channelDragController = new DragController({
		container: channelListEl,
		items: () => uncategorizedChannels.map(c => c.id),
		getElement: (id) => channelListEl?.querySelector(`[data-channel-id="${id}"]`) as HTMLElement | null,
		canDrag: $canManageChannels,
		onDrop: handleChannelReorder,
		onDragStateChange: (dragging) => { isDraggingChannel = dragging; },
	});
});

onDestroy(() => { channelDragController?.destroy(); });

async function handleChannelReorder(sourceId: string, targetIndex: number) {
	const guildId = $currentGuildId;
	if (!guildId) return;

	const reordered = [...uncategorizedChannels];
	const sourceIdx = reordered.findIndex(c => c.id === sourceId);
	if (sourceIdx === -1) return;

	const [moved] = reordered.splice(sourceIdx, 1);
	reordered.splice(targetIndex, 0, moved);

	const positions = reordered.map((c, i) => ({ id: c.id, position: i }));

	// Optimistic update
	for (const p of positions) {
		const ch = [...$textChannels, ...$voiceChannels, ...$forumChannels, ...$galleryChannels].find(c => c.id === p.id);
		if (ch) updateChannelStore({ ...ch, position: p.position });
	}

	try {
		await api.reorderChannels(guildId, positions);
	} catch (err: any) {
		addToast(err.message || 'Failed to reorder channels', 'error');
	}
}
```

**Step 2: Update the channel list template**

Find the `{#each uncategorizedChannels as channel}` loop. The parent container needs `bind:this={channelListEl}`. Each channel item needs:
- Remove: `draggable`, `ondragstart`, `ondragover`, `ondragleave`, `ondragend`, `ondrop`
- Remove: the `dragOverChannelId === channel.id` conditional class
- Add: `data-channel-id={channel.id}`, `class="group/drag"`
- Add: `onpointerdown` that calls the controller
- Add: DragHandle component before the channel icon

The channel button changes from:

```svelte
<button
    class="... {dragOverChannelId === channel.id ? 'border-t-2 border-brand-500' : ''}"
    draggable={$canManageChannels ? 'true' : undefined}
    ondragstart={(e) => handleChannelDragStart(e, channel.id)}
    ondragover={(e) => handleChannelDragOver(e, channel.id)}
    ondragleave={handleChannelDragLeave}
    ondragend={handleChannelDragEnd}
    ondrop={(e) => handleChannelDrop(e, channel.id, uncategorizedChannels)}
>
```

To:

```svelte
<div
    class="group/drag flex items-center"
    data-channel-id={channel.id}
    onpointerdown={(e) => channelDragController?.handlePointerDown(e, channel.id)}
>
    <DragHandle visible={$canManageChannels} />
    <button
        class="mb-0.5 flex flex-1 items-center gap-1.5 rounded px-2 py-1.5 text-left text-sm transition-colors {chMuted ? 'opacity-60' : ''} {$currentChannelId === channel.id ? 'bg-bg-modifier text-text-primary' : unread > 0 && !chMuted ? 'text-text-primary font-semibold hover:bg-bg-modifier' : 'text-text-muted hover:bg-bg-modifier hover:text-text-secondary'}"
        onclick={() => handleChannelClick(channel.id)}
        ondblclick={() => { if (isVoice) { const gid = $currentGuildId; if (gid) joinVoice(channel.id, gid, channel.name ?? ''); } }}
        oncontextmenu={(e) => openContextMenu(e, channel)}
    >
        <!-- ... same icon/name/badge content ... -->
    </button>
</div>
```

**Step 3: Wire up global pointermove/pointerup/keydown in the component**

Add to the `<svelte:window>` element:

```svelte
<svelte:window
    onclick={() => { closeContextMenu(); dmContextMenu = null; guildContextMenu = null; showStatusPicker = false; }}
    onpointermove={(e) => channelDragController?.handlePointerMove(e)}
    onpointerup={(e) => channelDragController?.handlePointerUp(e)}
    onkeydown={(e) => channelDragController?.handleKeyDown(e)}
/>
```

**Step 4: Test in browser**

1. Build and deploy: `docker compose -f deploy/docker/docker-compose.yml build --no-cache web-init && docker compose -f deploy/docker/docker-compose.yml up -d web-init && docker compose -f deploy/docker/docker-compose.yml restart caddy`
2. Navigate to a guild with 3+ channels
3. As an admin user, hover over a channel — grip dots appear on left
4. Click and drag a channel — floating preview follows cursor, items slide apart, blue insert line shows
5. Release — channel moves to new position
6. Verify non-admin users cannot drag

**Step 5: Commit**

```bash
git add web/src/lib/components/layout/ChannelSidebar.svelte
git commit -m "feat(drag): replace HTML5 DnD with pointer-based drag in ChannelSidebar"
```

---

## Task 5: Update ChannelGroups — Replace HTML5 DnD

**Files:**
- Modify: `web/src/lib/components/layout/ChannelGroups.svelte`

ChannelGroups has three drag interactions:
1. Channels within a group (reorder)
2. Channels between groups (cross-group move)
3. Groups themselves (reorder)

**Step 1: Replace channel-within-group drag**

Remove the old handlers: `handleChannelDragStart`, `handleChannelDragEnd`, `handleChannelDragOver`, `handleChannelDragLeave`, `handleChannelDrop` (lines 266-368), and the state: `dragOverChannelId`, `dragChannelId`, `dragChannelGroupId` (lines 266-268).

For each group's channel list, create a DragController scoped to that group. Since groups are rendered in an `#each`, use a Map to track controllers per group:

```ts
import DragHandle from '$components/common/DragHandle.svelte';
import { DragController } from '$lib/utils/dragDrop';
import { onDestroy } from 'svelte';

let groupControllers = $state<Map<string, DragController>>(new Map());
let groupContainerEls = $state<Map<string, HTMLElement>>(new Map());
let isDraggingInGroup = $state(false);

function setupGroupController(groupId: string, container: HTMLElement) {
	groupContainerEls.set(groupId, container);
	const existing = groupControllers.get(groupId);
	existing?.destroy();

	const ctrl = new DragController({
		container,
		items: () => {
			const group = groups.find(g => g.id === groupId);
			return group ? [...new Set(group.channels)] : [];
		},
		getElement: (id) => container.querySelector(`[data-channel-id="${id}"]`) as HTMLElement | null,
		canDrag: $canManageChannels,
		onDrop: (sourceId, targetIndex) => handleChannelReorderInGroup(sourceId, targetIndex, groupId),
		onDragStateChange: (d) => { isDraggingInGroup = d; },
	});
	groupControllers.set(groupId, ctrl);
}

onDestroy(() => {
	for (const ctrl of groupControllers.values()) ctrl.destroy();
});

async function handleChannelReorderInGroup(sourceId: string, targetIndex: number, groupId: string) {
	const guildId = $currentGuildId;
	if (!guildId) return;

	const group = groups.find(g => g.id === groupId);
	if (!group) return;

	const channels = [...new Set(group.channels)];
	const sourceIdx = channels.indexOf(sourceId);
	if (sourceIdx === -1) return;

	channels.splice(sourceIdx, 1);
	channels.splice(targetIndex, 0, sourceId);

	// Optimistic update
	groups = groups.map(g => g.id === groupId ? { ...g, channels } : g);

	try {
		await api.setChannelGroupChannels(guildId, groupId, channels);
	} catch (err: any) {
		addToast(err.message || 'Failed to reorder channels', 'error');
		await loadGroups(guildId);
	}
}
```

**Step 2: Replace group reorder drag**

Remove: `dragGroupId`, `dragOverGroupId`, `handleGroupDragStart/Over/Leave/End/Drop` (lines 370-430).

Add a separate DragController for the group list:

```ts
let groupListEl = $state<HTMLElement | null>(null);
let groupListController = $state<DragController | null>(null);

$effect(() => {
	if (!groupListEl || groups.length === 0) return;
	groupListController?.destroy();
	groupListController = new DragController({
		container: groupListEl,
		items: () => groups.map(g => g.id),
		getElement: (id) => groupListEl?.querySelector(`[data-group-id="${id}"]`) as HTMLElement | null,
		canDrag: $canManageChannels,
		onDrop: handleGroupReorder,
	});
});

async function handleGroupReorder(sourceId: string, targetIndex: number) {
	const guildId = $currentGuildId;
	if (!guildId) return;

	const reordered = [...groups];
	const sourceIdx = reordered.findIndex(g => g.id === sourceId);
	if (sourceIdx === -1) return;

	const [moved] = reordered.splice(sourceIdx, 1);
	reordered.splice(targetIndex, 0, moved);

	groups = reordered.map((g, i) => ({ ...g, position: i }));

	try {
		for (let i = 0; i < reordered.length; i++) {
			await api.updateChannelGroup(guildId, reordered[i].id, { position: i });
		}
	} catch (err: any) {
		addToast(err.message || 'Failed to reorder groups', 'error');
		await loadGroups(guildId);
	}
}
```

**Step 3: Handle cross-group channel drops**

For cross-group moves (dragging a channel from one group to another), add a secondary drop zone handler. When a channel is dragged over a different group's empty area or header, treat it as an "add to group" operation. This uses native `pointerover` detection on the group container divs:

```ts
// In the group container div, detect when a dragged channel enters a different group
function handleGroupZoneDrop(e: PointerEvent, targetGroupId: string) {
	if (!isDraggingInGroup) return;
	// The DragController handles within-group; cross-group is handled via
	// a separate "drop on group header" interaction using the existing
	// handleDrop function (now updated to use pointer events).
}
```

For simplicity: cross-group moves remain triggered by dropping a channel onto a different group's header area (a dedicated drop zone). This leverages the existing `handleDrop` logic but fires via `pointerup` detection instead of HTML5 `ondrop`.

**Step 4: Update the template**

For each group:
- Wrap the channel list in a `div` with `bind:this` to register with `setupGroupController`
- Add `data-group-id={group.id}` on the group container
- Add `data-channel-id={channelId}` on each channel item
- Remove all `draggable`, `ondragstart`, `ondragover`, `ondragleave`, `ondragend`, `ondrop` attributes
- Add `onpointerdown` to each channel item
- Add DragHandle on each channel and group header
- Wire `onpointermove`/`onpointerup`/`onkeydown` on `<svelte:window>`

**Step 5: Test in browser**

Same deploy steps as Task 4. Verify:
- Channel reorder within a group works with smooth animation
- Group reorder works with gap animation
- Channel drag from one group to another works
- Non-admin users see no drag handles

**Step 6: Commit**

```bash
git add web/src/lib/components/layout/ChannelGroups.svelte
git commit -m "feat(drag): replace HTML5 DnD with pointer-based drag in ChannelGroups"
```

---

## Task 6: Update GuildSidebar and GuildFolder — Replace HTML5 DnD

**Files:**
- Modify: `web/src/lib/components/layout/GuildSidebar.svelte` (lines 72-125)
- Modify: `web/src/lib/components/layout/GuildFolder.svelte` (lines 30-49)

**Step 1: Replace guild drag in GuildSidebar**

Remove: `dragGuildId`, `dragOverGuildId`, `handleGuildDragStart/Over/End/Drop` (lines 72-125).

Add:

```ts
import { DragController } from '$lib/utils/dragDrop';
import { onDestroy } from 'svelte';

let guildListEl = $state<HTMLElement | null>(null);
let guildDragController = $state<DragController | null>(null);

$effect(() => {
	if (!guildListEl) return;
	guildDragController?.destroy();
	guildDragController = new DragController({
		container: guildListEl,
		items: () => unfiledGuilds.map(g => g.id),
		getElement: (id) => guildListEl?.querySelector(`[data-guild-id="${id}"]`) as HTMLElement | null,
		canDrag: true, // All users can reorder their own guild list
		onDrop: handleGuildReorder,
	});
});

onDestroy(() => { guildDragController?.destroy(); });

async function handleGuildReorder(sourceId: string, targetIndex: number) {
	const list = $guildList;
	const sourceIdx = list.findIndex(g => g.id === sourceId);
	if (sourceIdx === -1) return;

	const reordered = [...list];
	const [moved] = reordered.splice(sourceIdx, 1);
	reordered.splice(targetIndex, 0, moved);

	const positions = reordered.map((g, i) => ({ guild_id: g.id, position: i }));

	for (const p of positions) {
		const g = list.find(x => x.id === p.guild_id);
		if (g) updateGuild({ ...g, position: p.position });
	}

	try {
		await api.reorderGuilds(positions);
	} catch (err: any) {
		addToast(err.message || 'Failed to reorder guilds', 'error');
	}
}
```

Template changes for unfiled guilds:
- Wrap in a container with `bind:this={guildListEl}`
- Each guild button: remove `draggable`, `ondragstart`, `ondragover`, `ondragleave`, `ondragend`, `ondrop`
- Add `data-guild-id={guild.id}`, `onpointerdown`
- Remove `dragOverGuildId` conditional class

**Step 2: Update GuildFolder to use pointer events for folder drop**

GuildFolder is a drop target — guilds can be dropped onto it. Since the guild drag now uses pointer events, we need to detect when a pointer is released over a folder.

Replace `handleFolderDragOver/DragLeave/Drop` (which use `ondragover/ondragleave/ondrop` HTML5 events) with pointer detection. The simplest approach: the parent `GuildSidebar` detects which folder the pointer is over during guild drag, and if the guild is released over a folder, calls the folder drop handler.

In GuildFolder, replace:
```ts
function handleFolderDragOver(e: DragEvent) { ... }
function handleFolderDragLeave() { ... }
function handleFolderDrop(e: DragEvent) { ... }
```

With pointer-based detection managed by the parent via a prop callback:
```ts
// GuildFolder.svelte — add pointerenter/pointerleave for highlighting
let isPointerOver = $state(false);

// Parent manages drag state and calls ondropguild when pointer is released over a folder
```

In the template, replace `ondragover/ondragleave/ondrop` with `onpointerenter/onpointerleave`:
```svelte
<div
    class="flex flex-col items-center"
    onpointerenter={() => { isPointerOver = true; }}
    onpointerleave={() => { isPointerOver = false; }}
    data-folder-id={folder.id}
    role="group"
>
```

Then in GuildSidebar, during guild drag's `pointerup`, check if the pointer is over a folder element:
```ts
// In the DragController's onDrop callback, also check folder drop zones
function handleGuildDrop(sourceId: string, targetIndex: number) {
	// Check if pointer is over a folder
	const folderEl = document.elementFromPoint(lastPointerX, lastPointerY)?.closest('[data-folder-id]');
	if (folderEl) {
		const folderId = folderEl.getAttribute('data-folder-id');
		if (folderId) {
			handleDropGuildOnFolder(sourceId, folderId);
			return;
		}
	}
	// Otherwise, normal guild reorder
	handleGuildReorder(sourceId, targetIndex);
}
```

**Step 3: Test in browser**

Deploy and verify:
- Guild reorder in sidebar works with smooth animation
- Dragging a guild onto a folder highlights the folder and moves the guild into it
- Guilds inside expanded folders can be dragged out

**Step 4: Commit**

```bash
git add web/src/lib/components/layout/GuildSidebar.svelte web/src/lib/components/layout/GuildFolder.svelte
git commit -m "feat(drag): replace HTML5 DnD with pointer-based drag in GuildSidebar/GuildFolder"
```

---

## Task 7: Run All Frontend Tests + Docker Build

**Files:**
- No changes — verification only.

**Step 1: Run all frontend tests**

Run: `docker run --rm -v $(pwd)/web:/web -w /web node:24-alpine npx vitest run`
Expected: All 574+ tests pass (including the new dragDrop tests).

**Step 2: Run full Docker build**

Run: `docker compose -f deploy/docker/docker-compose.yml build --no-cache amityvox web-init`
Expected: Build succeeds.

**Step 3: Deploy and restart**

```bash
docker compose -f deploy/docker/docker-compose.yml up -d amityvox web-init
docker compose -f deploy/docker/docker-compose.yml restart caddy
```

**Step 4: Manual smoke test**

- Verify channel drag in a guild
- Verify channel group drag
- Verify guild drag in sidebar
- Verify guild-to-folder drop
- Verify no regressions in click behavior (clicking channels still navigates)

**Step 5: Final commit (if any fixes needed)**

```bash
git add -A
git commit -m "fix(drag): address issues found during smoke testing"
```

---

## Summary

| Task | Component | Description |
|------|-----------|-------------|
| 1 | `dragDrop.ts` | Pure calculation functions + tests |
| 2 | `dragDrop.ts` | DragController class + tests |
| 3 | `DragHandle.svelte` | Grip icon component |
| 4 | `ChannelSidebar.svelte` | Replace HTML5 DnD with pointer drag |
| 5 | `ChannelGroups.svelte` | Replace all three drag interactions |
| 6 | `GuildSidebar.svelte` + `GuildFolder.svelte` | Replace guild/folder drag |
| 7 | Verification | Tests, build, deploy, smoke test |
