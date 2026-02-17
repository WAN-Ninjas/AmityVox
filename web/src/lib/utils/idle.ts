// Auto-idle detection: sets user status to "idle" after 5 minutes of inactivity.
// Does not auto-idle if user manually set "busy" or "invisible".

import { getGatewayClient } from '$lib/stores/gateway';
import { updatePresence } from '$lib/stores/presence';
import { currentUser } from '$lib/stores/auth';
import { get } from 'svelte/store';

const IDLE_TIMEOUT_MS = 5 * 60 * 1000; // 5 minutes

const ACTIVITY_EVENTS = ['mousemove', 'keydown', 'mousedown', 'touchstart'] as const;

let timer: ReturnType<typeof setTimeout> | null = null;
let isIdle = false;
let previousStatus: string = 'online';
let manualStatus: string | null = null;
let running = false;

/** Track manually-selected status so idle detection can respect it. */
export function setManualStatus(status: string) {
	manualStatus = status;
	// If user just picked a non-online/idle status, cancel any pending idle timer
	if (status === 'busy' || status === 'invisible') {
		clearIdleTimer();
		isIdle = false;
	}
}

/** Get the current manual status (for testing). */
export function getManualStatus(): string | null {
	return manualStatus;
}

/** Check if the user is currently auto-idled (for testing). */
export function isAutoIdled(): boolean {
	return isIdle;
}

function clearIdleTimer() {
	if (timer !== null) {
		clearTimeout(timer);
		timer = null;
	}
}

function shouldAutoIdle(): boolean {
	return manualStatus !== 'busy' && manualStatus !== 'invisible';
}

function onActivity() {
	if (!running) return;

	// If we were idle, restore previous status
	if (isIdle && shouldAutoIdle()) {
		isIdle = false;
		const restoreStatus = previousStatus || 'online';
		const user = get(currentUser);
		if (user) {
			updatePresence(user.id, restoreStatus);
			const client = getGatewayClient();
			client?.updatePresence(restoreStatus);
		}
	}

	// Reset idle timer
	clearIdleTimer();
	if (shouldAutoIdle()) {
		timer = setTimeout(goIdle, IDLE_TIMEOUT_MS);
	}
}

function goIdle() {
	if (!running || !shouldAutoIdle()) return;

	const user = get(currentUser);
	if (!user) return;

	// Save current status before going idle
	previousStatus = manualStatus || user.status_presence || 'online';
	if (previousStatus === 'idle') return; // Already idle

	isIdle = true;
	updatePresence(user.id, 'idle');
	const client = getGatewayClient();
	client?.updatePresence('idle');
}

export function startIdleDetection() {
	if (running) return;
	running = true;
	isIdle = false;

	// Initialize previous status from current user
	const user = get(currentUser);
	previousStatus = manualStatus || user?.status_presence || 'online';

	for (const event of ACTIVITY_EVENTS) {
		document.addEventListener(event, onActivity, { passive: true });
	}

	// Start the initial idle timer
	if (shouldAutoIdle()) {
		timer = setTimeout(goIdle, IDLE_TIMEOUT_MS);
	}
}

export function stopIdleDetection() {
	running = false;
	isIdle = false;
	clearIdleTimer();

	for (const event of ACTIVITY_EVENTS) {
		document.removeEventListener(event, onActivity);
	}
}

/** Compute expiry time for custom status (exported for testing). */
export function computeExpiryTime(ms: number | null): string | null {
	if (ms === null) return null;
	if (ms === -1) {
		const endOfDay = new Date();
		endOfDay.setHours(23, 59, 59, 999);
		return endOfDay.toISOString();
	}
	return new Date(Date.now() + ms).toISOString();
}
