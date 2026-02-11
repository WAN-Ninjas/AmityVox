// Gateway store — manages the WebSocket connection and dispatches events to stores.

import { writable } from 'svelte/store';
import { GatewayClient } from '$lib/api/ws';
import { currentUser } from './auth';
import { loadGuilds, updateGuild, removeGuild } from './guilds';
import { updateChannel, removeChannel } from './channels';
import { appendMessage, updateMessage, removeMessage } from './messages';
import { updatePresence } from './presence';
import type { User, Guild, Channel, Message, ReadyEvent } from '$lib/types';

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
				break;
			}

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
			case 'CHANNEL_UPDATE':
				updateChannel(data as Channel);
				break;
			case 'CHANNEL_DELETE':
				removeChannel((data as { id: string }).id);
				break;

			// --- Message events ---
			case 'MESSAGE_CREATE':
				appendMessage(data as Message);
				break;
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
