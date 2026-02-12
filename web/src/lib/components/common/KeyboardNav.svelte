<script lang="ts">
	/**
	 * KeyboardNav — Manages keyboard-only navigation, focus management, and
	 * tab order for all app features. Works alongside KeyboardShortcuts.svelte
	 * which handles specific keyboard shortcuts (Ctrl+K, etc.).
	 *
	 * This component provides:
	 * - Arrow key navigation within lists (channels, members, messages)
	 * - Focus trapping within modals and panels
	 * - Skip-to-content navigation
	 * - Focus-visible ring styling
	 * - Screen reader announcements for navigation changes
	 */

	interface Props {
		/** Whether keyboard navigation mode is active (toggled by Tab press). */
		active?: boolean;
	}

	let { active = $bindable(false) }: Props = $props();

	/** Track the current focus region for arrow key navigation. */
	let currentRegion = $state<string | null>(null);
	let announcement = $state('');

	/** All navigable regions in order. */
	const regions = ['guild-sidebar', 'channel-sidebar', 'message-list', 'member-list'];

	/**
	 * Enable keyboard navigation mode on first Tab press.
	 * Disable on mouse click.
	 */
	function handleKeydown(e: KeyboardEvent) {
		// Tab key enables keyboard navigation mode.
		if (e.key === 'Tab') {
			if (!active) {
				active = true;
				document.body.classList.add('keyboard-nav');
			}
			return;
		}

		// Only handle arrow keys when keyboard nav is active.
		if (!active) return;

		const target = e.target as HTMLElement;
		const isInput =
			target?.tagName === 'INPUT' ||
			target?.tagName === 'TEXTAREA' ||
			target?.isContentEditable;

		// Don't intercept arrow keys in text inputs.
		if (isInput) return;

		switch (e.key) {
			case 'ArrowUp':
			case 'ArrowDown':
				e.preventDefault();
				navigateWithinRegion(e.key === 'ArrowUp' ? -1 : 1);
				break;

			case 'ArrowLeft':
			case 'ArrowRight':
				e.preventDefault();
				navigateBetweenRegions(e.key === 'ArrowLeft' ? -1 : 1);
				break;

			case 'Home':
				e.preventDefault();
				focusFirstInRegion();
				break;

			case 'End':
				e.preventDefault();
				focusLastInRegion();
				break;

			case 'F6':
				// F6: cycle between regions (common accessibility pattern).
				e.preventDefault();
				navigateBetweenRegions(e.shiftKey ? -1 : 1);
				break;
		}
	}

	function handleMousedown() {
		if (active) {
			active = false;
			document.body.classList.remove('keyboard-nav');
		}
	}

	/**
	 * Navigate within the current focus region (up/down through items).
	 */
	function navigateWithinRegion(direction: number) {
		const region = getCurrentRegion();
		if (!region) return;

		const focusables = getFocusableElements(region);
		if (focusables.length === 0) return;

		const currentIndex = focusables.indexOf(document.activeElement as HTMLElement);
		let nextIndex: number;

		if (currentIndex === -1) {
			nextIndex = direction > 0 ? 0 : focusables.length - 1;
		} else {
			nextIndex = currentIndex + direction;
			if (nextIndex < 0) nextIndex = focusables.length - 1;
			if (nextIndex >= focusables.length) nextIndex = 0;
		}

		const target = focusables[nextIndex];
		target.focus();
		announceElement(target);
	}

	/**
	 * Navigate between major regions (left/right or F6).
	 */
	function navigateBetweenRegions(direction: number) {
		const currentIdx = currentRegion ? regions.indexOf(currentRegion) : -1;
		let nextIdx: number;

		if (currentIdx === -1) {
			nextIdx = direction > 0 ? 0 : regions.length - 1;
		} else {
			nextIdx = currentIdx + direction;
			if (nextIdx < 0) nextIdx = regions.length - 1;
			if (nextIdx >= regions.length) nextIdx = 0;
		}

		const regionEl = document.querySelector(`[data-nav-region="${regions[nextIdx]}"]`) as HTMLElement;
		if (!regionEl) return;

		currentRegion = regions[nextIdx];

		// Focus the first focusable element in the region.
		const focusables = getFocusableElements(regionEl);
		if (focusables.length > 0) {
			focusables[0].focus();
			announceElement(focusables[0]);
		} else {
			regionEl.focus();
		}

		announce(`Navigated to ${currentRegion.replace(/-/g, ' ')}`);
	}

	/**
	 * Focus the first focusable element in the current region.
	 */
	function focusFirstInRegion() {
		const region = getCurrentRegion();
		if (!region) return;

		const focusables = getFocusableElements(region);
		if (focusables.length > 0) {
			focusables[0].focus();
			announceElement(focusables[0]);
		}
	}

	/**
	 * Focus the last focusable element in the current region.
	 */
	function focusLastInRegion() {
		const region = getCurrentRegion();
		if (!region) return;

		const focusables = getFocusableElements(region);
		if (focusables.length > 0) {
			focusables[focusables.length - 1].focus();
			announceElement(focusables[focusables.length - 1]);
		}
	}

	/**
	 * Get the region element containing the currently focused element.
	 */
	function getCurrentRegion(): HTMLElement | null {
		if (currentRegion) {
			return document.querySelector(`[data-nav-region="${currentRegion}"]`);
		}

		// Detect region from active element.
		const active = document.activeElement;
		if (!active) return null;

		for (const regionName of regions) {
			const regionEl = document.querySelector(`[data-nav-region="${regionName}"]`);
			if (regionEl?.contains(active)) {
				currentRegion = regionName;
				return regionEl as HTMLElement;
			}
		}

		return null;
	}

	/**
	 * Get all focusable elements within a container, respecting tabindex and visibility.
	 */
	function getFocusableElements(container: HTMLElement): HTMLElement[] {
		const selector = [
			'a[href]:not([tabindex="-1"])',
			'button:not([disabled]):not([tabindex="-1"])',
			'input:not([disabled]):not([tabindex="-1"])',
			'select:not([disabled]):not([tabindex="-1"])',
			'textarea:not([disabled]):not([tabindex="-1"])',
			'[tabindex]:not([tabindex="-1"])',
			'[role="button"]:not([tabindex="-1"])',
			'[role="menuitem"]:not([tabindex="-1"])',
			'[role="option"]:not([tabindex="-1"])',
			'[data-focusable]'
		].join(',');

		const elements = Array.from(container.querySelectorAll(selector)) as HTMLElement[];

		// Filter out hidden elements.
		return elements.filter((el) => {
			if (el.offsetParent === null && el.style.position !== 'fixed') return false;
			if (el.getAttribute('aria-hidden') === 'true') return false;
			return true;
		});
	}

	/**
	 * Announce an element's label to screen readers.
	 */
	function announceElement(el: HTMLElement) {
		const label =
			el.getAttribute('aria-label') ||
			el.getAttribute('title') ||
			el.textContent?.trim().slice(0, 50) ||
			el.tagName.toLowerCase();
		announce(label);
	}

	/**
	 * Set a live region announcement for screen readers.
	 */
	function announce(text: string) {
		announcement = '';
		// Use a microtask to ensure the live region picks up the change.
		requestAnimationFrame(() => {
			announcement = text;
		});
	}
</script>

<svelte:document onkeydown={handleKeydown} onmousedown={handleMousedown} />

<!-- Skip to content link (visible on focus) -->
<a
	href="#main-content"
	class="fixed left-2 top-2 z-[100] -translate-y-full rounded-md bg-brand-500 px-4 py-2 text-sm font-medium text-white opacity-0 transition-all focus:translate-y-0 focus:opacity-100"
>
	Skip to content
</a>

<!-- Live region for screen reader announcements -->
<div class="sr-only" aria-live="polite" aria-atomic="true" role="status">
	{announcement}
</div>

<style>
	:global(body.keyboard-nav) :global(*:focus-visible) {
		outline: 2px solid var(--brand-500, #5865f2);
		outline-offset: 2px;
		border-radius: 2px;
	}

	:global(body:not(.keyboard-nav)) :global(*:focus) {
		outline: none;
	}

	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border-width: 0;
	}
</style>
