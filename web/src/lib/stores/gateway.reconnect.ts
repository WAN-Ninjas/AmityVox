// gateway.reconnect.ts — Enhanced WebSocket reconnection logic with exponential
// backoff, jitter, and connection quality monitoring.
//
// This module is a helper imported by the gateway store. It does NOT modify
// gateway.ts directly. Instead, it exports reactive stores and utility functions
// that the gateway and UI components can use.

import { writable, derived, get } from 'svelte/store';

// ============================================================
// Connection Quality Stores
// ============================================================

/** Connection quality classification. */
export type ConnectionQualityLevel = 'good' | 'degraded' | 'disconnected';

/** Current connection quality level. */
export const connectionQuality = writable<ConnectionQualityLevel>('disconnected');

/** Latest measured round-trip latency in milliseconds. */
export const connectionLatency = writable<number>(0);

/** Current reconnection attempt number (0 = connected). */
export const reconnectAttempt = writable<number>(0);

/** Whether the connection is fully established and healthy. */
export const isConnected = derived(connectionQuality, (q) => q === 'good');

/** Whether a reconnection is in progress. */
export const isReconnecting = derived(reconnectAttempt, (a) => a > 0);

// ============================================================
// Exponential Backoff with Jitter
// ============================================================

export interface BackoffConfig {
	/** Initial delay in milliseconds. Default: 1000. */
	initialDelay: number;

	/** Maximum delay in milliseconds. Default: 60000 (1 minute). */
	maxDelay: number;

	/** Exponential growth factor. Default: 2. */
	factor: number;

	/** Maximum jitter as a fraction of the computed delay (0-1). Default: 0.25. */
	jitterFraction: number;

	/** Maximum number of attempts before giving up. Default: 50. */
	maxAttempts: number;
}

const DEFAULT_BACKOFF_CONFIG: BackoffConfig = {
	initialDelay: 1000,
	maxDelay: 60000,
	factor: 2,
	jitterFraction: 0.25,
	maxAttempts: 50
};

/**
 * Calculates the next reconnection delay using exponential backoff with
 * decorrelated jitter. This prevents the "thundering herd" problem where
 * all clients reconnect simultaneously after a server restart.
 *
 * @param attempt - Current attempt number (0-indexed).
 * @param config - Backoff configuration.
 * @returns Delay in milliseconds, or -1 if max attempts exceeded.
 */
export function calculateBackoffDelay(
	attempt: number,
	config: BackoffConfig = DEFAULT_BACKOFF_CONFIG
): number {
	if (attempt >= config.maxAttempts) {
		return -1; // Signal to give up.
	}

	// Exponential delay: initialDelay * factor^attempt.
	const exponentialDelay = config.initialDelay * Math.pow(config.factor, attempt);

	// Cap at maxDelay.
	const cappedDelay = Math.min(exponentialDelay, config.maxDelay);

	// Add decorrelated jitter: random value between [0, jitterFraction * delay].
	const jitter = Math.random() * config.jitterFraction * cappedDelay;

	// Full jitter: pick randomly between 0 and the base delay, then add jitter.
	// This gives better spread than simple additive jitter.
	const fullJitterDelay = Math.random() * cappedDelay;
	const finalDelay = Math.max(config.initialDelay, (fullJitterDelay + cappedDelay) / 2 + jitter);

	return Math.floor(Math.min(finalDelay, config.maxDelay));
}

// ============================================================
// Latency Measurement
// ============================================================

/** Sliding window of recent latency measurements. */
const LATENCY_WINDOW_SIZE = 10;
let latencyHistory: number[] = [];

/**
 * Records a heartbeat round-trip time and updates the connection quality.
 * Call this when a heartbeat ACK is received.
 *
 * @param latencyMs - The round-trip time in milliseconds.
 */
export function recordLatency(latencyMs: number): void {
	latencyHistory.push(latencyMs);
	if (latencyHistory.length > LATENCY_WINDOW_SIZE) {
		latencyHistory = latencyHistory.slice(-LATENCY_WINDOW_SIZE);
	}

	connectionLatency.set(latencyMs);
	updateQuality();
}

/**
 * Resets all connection quality state. Call on disconnect.
 */
export function resetConnectionState(): void {
	latencyHistory = [];
	connectionQuality.set('disconnected');
	connectionLatency.set(0);
}

/**
 * Marks the connection as established and healthy.
 * Call after successful READY event.
 */
export function markConnected(): void {
	reconnectAttempt.set(0);
	connectionQuality.set('good');
}

/**
 * Marks the connection as disconnected and begins reconnection tracking.
 */
export function markDisconnected(): void {
	connectionQuality.set('disconnected');
}

/**
 * Increments the reconnect attempt counter and returns the new count.
 */
export function incrementReconnectAttempt(): number {
	const current = get(reconnectAttempt);
	const next = current + 1;
	reconnectAttempt.set(next);
	connectionQuality.set('degraded');
	return next;
}

// ============================================================
// Quality Assessment
// ============================================================

/** Latency thresholds in milliseconds. */
const LATENCY_THRESHOLD_GOOD = 300;
const LATENCY_THRESHOLD_DEGRADED = 1000;

/**
 * Updates the connection quality based on recent latency measurements.
 */
function updateQuality(): void {
	if (latencyHistory.length === 0) {
		return;
	}

	// Calculate average of recent measurements.
	const avg = latencyHistory.reduce((a, b) => a + b, 0) / latencyHistory.length;

	// Calculate jitter (standard deviation of latency).
	const variance = latencyHistory.reduce((sum, val) => sum + Math.pow(val - avg, 2), 0) / latencyHistory.length;
	const jitter = Math.sqrt(variance);

	// Quality assessment: average latency + jitter penalty.
	const effectiveLatency = avg + jitter * 0.5;

	if (effectiveLatency < LATENCY_THRESHOLD_GOOD) {
		connectionQuality.set('good');
	} else if (effectiveLatency < LATENCY_THRESHOLD_DEGRADED) {
		connectionQuality.set('degraded');
	} else {
		connectionQuality.set('degraded');
	}
}

// ============================================================
// Network Status Detection
// ============================================================

/** Whether the browser reports an active network connection. */
export const isOnline = writable<boolean>(
	typeof navigator !== 'undefined' ? navigator.onLine : true
);

// Listen for browser online/offline events.
if (typeof window !== 'undefined') {
	window.addEventListener('online', () => {
		isOnline.set(true);
	});

	window.addEventListener('offline', () => {
		isOnline.set(false);
		connectionQuality.set('disconnected');
	});

	// Detect connection quality via Network Information API when available.
	if ('connection' in navigator) {
		const conn = (navigator as unknown as { connection: { addEventListener: (type: string, fn: () => void) => void; effectiveType: string } }).connection;
		conn.addEventListener('change', () => {
			const effectiveType = conn.effectiveType;
			if (effectiveType === 'slow-2g' || effectiveType === '2g') {
				connectionQuality.set('degraded');
			}
		});
	}
}

// ============================================================
// Reconnection Scheduler
// ============================================================

/**
 * Creates a reconnection scheduler that manages backoff timing and exposes
 * the current state. This is designed to be used alongside the existing
 * GatewayClient reconnection logic.
 */
export function createReconnectScheduler(config?: Partial<BackoffConfig>) {
	const mergedConfig = { ...DEFAULT_BACKOFF_CONFIG, ...config };
	let timer: ReturnType<typeof setTimeout> | null = null;
	let attempt = 0;

	return {
		/**
		 * Schedules the next reconnection attempt. Returns the delay in ms,
		 * or -1 if max attempts reached.
		 */
		schedule(onReconnect: () => void): number {
			if (timer) clearTimeout(timer);

			const delay = calculateBackoffDelay(attempt, mergedConfig);
			if (delay === -1) {
				return -1;
			}

			attempt++;
			reconnectAttempt.set(attempt);
			connectionQuality.set('degraded');

			timer = setTimeout(() => {
				timer = null;
				// Only reconnect if we're online.
				if (get(isOnline)) {
					onReconnect();
				} else {
					// Wait for online event, then reconnect.
					const cleanup = isOnline.subscribe((online) => {
						if (online) {
							cleanup();
							onReconnect();
						}
					});
				}
			}, delay);

			return delay;
		},

		/** Resets the backoff state after a successful connection. */
		reset(): void {
			if (timer) clearTimeout(timer);
			timer = null;
			attempt = 0;
			markConnected();
		},

		/** Cancels any pending reconnection. */
		cancel(): void {
			if (timer) clearTimeout(timer);
			timer = null;
			attempt = 0;
			reconnectAttempt.set(0);
		},

		/** Returns the current attempt number. */
		getAttempt(): number {
			return attempt;
		}
	};
}
