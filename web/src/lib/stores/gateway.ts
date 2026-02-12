// Gateway store — manages the WebSocket connection and dispatches events to stores.

import { writable } from 'svelte/store';
import { goto } from '$app/navigation';
import { GatewayClient } from '$lib/api/ws';
import { currentUser } from './auth';
import { loadGuilds, updateGuild, removeGuild } from './guilds';
import { updateChannel, removeChannel } from './channels';
import { appendMessage, updateMessage, removeMessage } from './messages';
import { updatePresence } from './presence';
import { addTypingUser, clearTypingUser } from './typing';
import { loadDMs, addDMChannel, updateDMChannel, removeDMChannel } from './dms';
import { incrementUnread, loadReadState } from './unreads';
import type { User, Guild, Channel, Message, ReadyEvent, TypingEvent } from '$lib/types';

export const gatewayConnected = writable(false);

let client: GatewayClient | null = null;

export function connectGateway(token: string) {
	if (client) client.disconnect();

	client = new GatewayClient(token);

	client.on((event, data) => {
		switch (event) {
			case 'READY': {
				const ready = data as ReadyEvent;
				currentUser.set(ready.user);
				gatewayConnected.set(true);
				loadGuilds();
				loadDMs();
				loadReadState();
				updatePresence(ready.user.id, 'online');
				client?.updatePresence('online');
				break;
			}

			// --- Gateway lifecycle ---
			case 'GATEWAY_AUTH_FAILED':
				// Token is invalid — redirect to login.
				disconnectGateway();
				goto('/login');
				break;
			case 'GATEWAY_EXHAUSTED':
				// Too many failed reconnects — mark disconnected.
				gatewayConnected.set(false);
				break;

			// --- Guild events ---
			case 'GUILD_CREATE':
			case 'GUILD_UPDATE':
				updateGuild(data as Guild);
				break;
			case 'GUILD_DELETE':
				removeGuild((data as { id: string }).id);
				break;

			// --- Channel events ---
			case 'CHANNEL_CREATE':
			case 'CHANNEL_UPDATE': {
				const ch = data as Channel;
				updateChannel(ch);
				// Also track DM channels.
				if (ch.channel_type === 'dm' || ch.channel_type === 'group') {
					addDMChannel(ch);
				}
				break;
			}
			case 'CHANNEL_DELETE': {
				const deleted = data as { id: string };
				removeChannel(deleted.id);
				removeDMChannel(deleted.id);
				break;
			}

			// --- Message events ---
			case 'MESSAGE_CREATE': {
				const msg = data as Message;
				appendMessage(msg);
				// Track unreads for messages not from self.
				let selfId: string | undefined;
				currentUser.subscribe((u) => (selfId = u?.id ?? undefined))();
				if (msg.author_id !== selfId) {
					const isMention = msg.mention_everyone ||
						(selfId ? msg.mention_user_ids?.includes(selfId) : false);
					incrementUnread(msg.channel_id, isMention);
				}
				break;
			}
			case 'MESSAGE_UPDATE':
				updateMessage(data as Message);
				break;
			case 'MESSAGE_DELETE':
				removeMessage(
					(data as { channel_id: string }).channel_id,
					(data as { id: string }).id
				);
				break;

			// --- Presence events ---
			case 'PRESENCE_UPDATE':
				updatePresence(
					(data as { user_id: string }).user_id,
					(data as { status: string }).status
				);
				break;

			// --- Typing events ---
			case 'TYPING_START': {
				const typing = data as TypingEvent;
				addTypingUser(typing.channel_id, typing.user_id);
				break;
			}

			// --- User events ---
			case 'USER_UPDATE':
				currentUser.set(data as User);
				break;
		}
	});

	client.connect();
}

export function disconnectGateway() {
	client?.disconnect();
	client = null;
	gatewayConnected.set(false);
}

export function getGatewayClient(): GatewayClient | null {
	return client;
}
