// WebSocket gateway client for AmityVox real-time events.
// Handles connection, heartbeating, identify, resume, and event dispatch.

import { GatewayOp, type GatewayMessage, type ReadyEvent } from '$lib/types';

export type EventHandler = (eventType: string, data: unknown) => void;

function getWsUrl(): string {
	if (typeof location === 'undefined') return 'ws://localhost/ws';
	return `${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/ws`;
}

const RECONNECT_DELAYS = [1000, 2000, 5000, 10000, 30000];
const MAX_RECONNECT_ATTEMPTS = 50;

export class GatewayClient {
	private ws: WebSocket | null = null;
	private heartbeatInterval: ReturnType<typeof setInterval> | null = null;
	private heartbeatAcked = true;
	private sequence = 0;
	private sessionId: string | null = null;
	private token: string;
	private reconnectAttempt = 0;
	private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	private handlers: EventHandler[] = [];
	private closed = false;

	constructor(token: string) {
		this.token = token;
	}

	on(handler: EventHandler) {
		this.handlers.push(handler);
		return () => {
			this.handlers = this.handlers.filter((h) => h !== handler);
		};
	}

	private emit(eventType: string, data: unknown) {
		for (const handler of this.handlers) {
			try {
				handler(eventType, data);
			} catch (e) {
				console.error('[GW] Handler error:', e);
			}
		}
	}

	connect() {
		if (this.ws) return;
		this.closed = false;

		const url = getWsUrl();
		try {
			this.ws = new WebSocket(url);
		} catch {
			this.scheduleReconnect();
			return;
		}

		this.ws.onopen = () => {
			// Don't reset reconnectAttempt here — only reset after successful READY.
		};

		this.ws.onmessage = (event) => {
			try {
				const msg: GatewayMessage = JSON.parse(event.data);
				this.handleMessage(msg);
			} catch (e) {
				console.error('[GW] Failed to parse message:', e);
			}
		};

		this.ws.onclose = (event) => {
			this.cleanup();
			if (this.closed) return;

			// Auth failure — don't reconnect, token is invalid.
			if (event.code === 4001 || event.code === 4004) {
				console.error('[GW] Auth rejected, not reconnecting');
				this.emit('GATEWAY_AUTH_FAILED', null);
				return;
			}

			this.scheduleReconnect();
		};

		this.ws.onerror = () => {
			// onerror is always followed by onclose, so just let onclose handle reconnection.
		};
	}

	disconnect() {
		this.closed = true;
		if (this.reconnectTimer) {
			clearTimeout(this.reconnectTimer);
			this.reconnectTimer = null;
		}
		this.ws?.close();
		this.cleanup();
	}

	private handleMessage(msg: GatewayMessage) {
		switch (msg.op) {
			case GatewayOp.Hello:
				this.startHeartbeat((msg.d as { heartbeat_interval: number }).heartbeat_interval);
				this.identify();
				break;

			case GatewayOp.HeartbeatAck:
				this.heartbeatAcked = true;
				break;

			case GatewayOp.Dispatch:
				if (msg.s) this.sequence = msg.s;
				if (msg.t === 'READY') {
					const ready = msg.d as ReadyEvent;
					this.sessionId = ready.session_id;
					// Only reset backoff after a fully successful handshake.
					this.reconnectAttempt = 0;
				}
				if (msg.t) this.emit(msg.t, msg.d);
				break;

			case GatewayOp.Reconnect:
				this.ws?.close();
				break;
		}
	}

	private identify() {
		if (this.sessionId && this.sequence > 0) {
			this.send({
				op: GatewayOp.Resume,
				d: { token: this.token, session_id: this.sessionId, seq: this.sequence }
			});
		} else {
			this.send({
				op: GatewayOp.Identify,
				d: { token: this.token }
			});
		}
	}

	private startHeartbeat(intervalMs: number) {
		this.stopHeartbeat();
		this.heartbeatAcked = true;

		// Validate server-provided interval is within safe bounds (15s–120s).
		// Use a hardcoded default for out-of-range values to prevent resource
		// exhaustion from a malicious or misconfigured server.
		const DEFAULT_HEARTBEAT_MS = 41_250;
		const safeInterval = (Number.isFinite(intervalMs) && intervalMs >= 15_000 && intervalMs <= 120_000)
			? intervalMs
			: DEFAULT_HEARTBEAT_MS;

		this.heartbeatInterval = setInterval(() => {
			if (!this.heartbeatAcked) {
				this.ws?.close();
				return;
			}
			this.heartbeatAcked = false;
			this.send({ op: GatewayOp.Heartbeat });
		}, safeInterval);
	}

	private stopHeartbeat() {
		if (this.heartbeatInterval) {
			clearInterval(this.heartbeatInterval);
			this.heartbeatInterval = null;
		}
	}

	private send(msg: GatewayMessage) {
		if (this.ws?.readyState === WebSocket.OPEN) {
			this.ws.send(JSON.stringify(msg));
		}
	}

	private cleanup() {
		this.stopHeartbeat();
		this.ws = null;
	}

	private scheduleReconnect() {
		if (this.reconnectAttempt >= MAX_RECONNECT_ATTEMPTS) {
			console.error('[GW] Max reconnect attempts reached, giving up');
			this.emit('GATEWAY_EXHAUSTED', null);
			return;
		}

		const base = RECONNECT_DELAYS[Math.min(this.reconnectAttempt, RECONNECT_DELAYS.length - 1)];
		// Add jitter (0-25%) to prevent thundering herd.
		const jitter = Math.random() * base * 0.25;
		const delay = base + jitter;
		this.reconnectAttempt++;

		this.reconnectTimer = setTimeout(() => {
			this.reconnectTimer = null;
			if (!this.closed) this.connect();
		}, delay);
	}

	// --- Client actions ---

	sendTyping(channelId: string) {
		this.send({ op: GatewayOp.Typing, d: { channel_id: channelId } });
	}

	updatePresence(status: string) {
		this.send({ op: GatewayOp.PresenceUpdate, d: { status } });
	}
}
