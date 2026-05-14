# Drag/Drop UI Overhaul Design

**Date:** 2026-02-19
**Status:** Approved

## Problem

The current drag/drop across channels, channel groups, and guilds uses the HTML5 Drag and Drop API with minimal visual feedback. Issues:

1. Default browser drag ghost (ugly, varies by browser)
2. No opacity/scale change on the dragged source item
3. Only a thin `border-t-2` line as a drop indicator
4. No animation when items reorder (they snap instantly)
5. Drop indicator flickers due to constant `dragover` re-renders
6. No drag handle — whole button is draggable, conflicting with clicks
7. No touch/mobile support (HTML5 DnD doesn't work on touch)
8. No visual feedback when dragging over empty groups

## Approach

Replace HTML5 DnD with a custom pointer-event-based drag system. Build a reusable `DragController` utility that all three components share.

## Reusable Utility — `web/src/lib/utils/dragDrop.ts`

### Activation
- `pointerdown` starts tracking cursor movement.
- Drag activates after 5px of movement (prevents accidental drags on click).
- `setPointerCapture()` ensures events are received even if cursor leaves the element.

### Preview
- A floating clone of the dragged element, positioned under cursor with `position: fixed`.
- Styled with `opacity: 0.9`, `scale(1.02)`, `box-shadow`, `pointer-events: none`.
- Offset from cursor so it doesn't obscure the drop target.

### Source Styling
- The original item gets `opacity: 0.3` during drag.

### Gap Animation
- As cursor moves over the list, items above/below the insertion point get `transform: translateY(+/- itemHeight)` with a `150ms ease` CSS transition.
- Items visually slide apart to show where the drop will land.

### Insert Indicator
- A 2px `bg-brand-500` horizontal line spanning the container width.
- Small 6px circles on left and right ends.
- Absolutely positioned between items at the calculated insertion point.

### Auto-Scroll
- When cursor is within 40px of the scrollable container's top/bottom edge, auto-scroll.
- Scroll speed increases as cursor gets closer to the edge (max 15px/frame).

### Drop
- On `pointerup`, call the `onDrop(sourceId, targetId, position)` callback.
- Clean up all visual state (preview, source opacity, gap transforms, indicator).

### Cancel
- `Escape` key cancels the drag and restores all visual state.
- `pointerup` outside the valid drop area also cancels.

### API

```ts
interface DragOptions {
  container: HTMLElement;
  items: () => string[];
  getElement: (id: string) => HTMLElement | null;
  onDrop: (sourceId: string, targetId: string, position: 'before' | 'after') => void;
  canDrag?: boolean;
  dragHandleSelector?: string;
  axis?: 'vertical' | 'horizontal';
  groupId?: string;
}

function createDragController(options: DragOptions): {
  handlePointerDown: (e: PointerEvent, itemId: string) => void;
  destroy: () => void;
};
```

## Visual Design

### During Drag
- **Source item:** `opacity: 0.3`
- **Floating preview:** clone with `shadow-lg`, slight scale, `border border-brand-500/50`
- **Insert line:** 2px `bg-brand-500`, full-width, with 6px dot endpoints
- **Gap:** items slide apart by the dragged item's height

### Drag Handle
- 6-dot grip icon appears on the left side of each draggable item on hover
- Only visible for users with manage permissions
- Cursor: `grab` on hover, `grabbing` during drag

### Drop Zone Feedback (cross-group)
- Group container gets dashed `border-brand-500` and `bg-brand-500/5` when a channel is dragged over it

## Scope — Components to Update

### ChannelSidebar.svelte
- Replace all `ondragstart/ondragover/ondragleave/ondragend/ondrop` handlers
- Add drag handle icons on hover
- Remove `dragChannelId`/`dragOverChannelId` state

### ChannelGroups.svelte
- Replace three sets of drag handlers (within-group, between-group, group reorder)
- Cross-list detection: when dragged outside one group into another
- Groups get pointer-based reorder with gap animation

### GuildSidebar.svelte
- Replace guild drag handlers
- Same floating preview + insert indicator
- Folder drop zones: highlight on hover

## Testing

- Unit tests for `dragDrop.ts`: activation threshold, index calculation, cancel behavior, callback invocation
- Test as pure functions (Svelte 5 component testing limitation)

## Non-Goals

- Keyboard-driven reorder (out of scope for this change)
- Drag across different sidebars (e.g., dragging a guild into the channel sidebar)
