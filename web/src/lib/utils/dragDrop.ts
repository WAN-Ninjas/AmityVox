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

	handlePointerCancel(_e: PointerEvent): void {
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
		if (!this.sourceId) return;
		const el = this.opts.getElement(this.sourceId);
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
