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
