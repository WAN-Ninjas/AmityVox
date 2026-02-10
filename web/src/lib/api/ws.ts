// WebSocket gateway client for AmityVox real-time events.
// Handles connection, heartbeating, identify, resume, and event dispatch.

import { GatewayOp, type GatewayMessage, type ReadyEvent } from '$lib/types';

export type EventHandler = (eventType: string, data: unknown) => void;

const WS_URL = `${location?.protocol === 'https:' ? 'wss:' : 'ws:'}//${location?.host}/ws`;
const RECONNECT_DELAYS = [1000, 2000, 5000, 10000, 30000];

export class GatewayClient {
	private ws: WebSocket | null = null;
	private heartbeatInterval: ReturnType<typeof setInterval> | null = null;
	private heartbeatAcked = true;
	private sequence = 0;
	private sessionId: string | null = null;
	private token: string;
	private reconnectAttempt = 0;
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

		try {
			this.ws = new WebSocket(WS_URL);
		} catch {
			this.scheduleReconnect();
			return;
		}

		this.ws.onopen = () => {
			this.reconnectAttempt = 0;
		};

		this.ws.onmessage = (event) => {
			const msg: GatewayMessage = JSON.parse(event.data);
			this.handleMessage(msg);
		};

		this.ws.onclose = () => {
			this.cleanup();
			if (!this.closed) this.scheduleReconnect();
		};

		this.ws.onerror = () => {
			this.ws?.close();
		};
	}

	disconnect() {
		this.closed = true;
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
			// Resume existing session.
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

		this.heartbeatInterval = setInterval(() => {
			if (!this.heartbeatAcked) {
				// Missed ACK — connection is dead.
				this.ws?.close();
				return;
			}
			this.heartbeatAcked = false;
			this.send({ op: GatewayOp.Heartbeat });
		}, intervalMs);
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
		const delay = RECONNECT_DELAYS[Math.min(this.reconnectAttempt, RECONNECT_DELAYS.length - 1)];
		this.reconnectAttempt++;
		setTimeout(() => {
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
