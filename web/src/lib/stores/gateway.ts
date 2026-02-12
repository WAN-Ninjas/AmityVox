// Gateway store — manages the WebSocket connection and dispatches events to stores.

import { writable, get } from 'svelte/store';
import { goto } from '$app/navigation';
import { GatewayClient } from '$lib/api/ws';
import { currentUser } from './auth';
import { loadGuilds, updateGuild, removeGuild, guilds as guildsStore } from './guilds';
import { updateChannel, removeChannel, channels as channelsStore } from './channels';
import { appendMessage, updateMessage, removeMessage, removeMessages } from './messages';
import { updatePresence } from './presence';
import { addTypingUser, clearTypingUser } from './typing';
import { loadDMs, addDMChannel, updateDMChannel, removeDMChannel } from './dms';
import { incrementUnread, loadReadState } from './unreads';
import { addNotification } from './notifications';
import type { User, Guild, Channel, Message, ReadyEvent, TypingEvent, Relationship } from '$lib/types';

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
				// Load initial presence for all online guild members.
				if (ready.presences) {
					for (const [uid, status] of Object.entries(ready.presences)) {
						updatePresence(uid, status as string);
					}
				}
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

					// Build notification for mentions, replies, and DMs.
					const channel = get(channelsStore).get(msg.channel_id);
					const isDM = channel?.channel_type === 'dm' || channel?.channel_type === 'group';
					const isReply = msg.message_type === 'reply' || (msg.reply_to_ids && msg.reply_to_ids.length > 0);
					const senderName = msg.author?.display_name ?? msg.author?.username ?? 'Unknown';

					if (isMention || isDM || isReply) {
						const guildId = channel?.guild_id ?? null;
						const guild = guildId ? get(guildsStore).get(guildId) ?? null : null;

						addNotification({
							type: isDM ? 'dm' : isReply ? 'reply' : 'mention',
							guild_id: guildId,
							guild_name: guild?.name ?? null,
							channel_id: msg.channel_id,
							channel_name: channel?.name ?? null,
							message_id: msg.id,
							sender_id: msg.author_id,
							sender_name: senderName,
							content: msg.content ? msg.content.slice(0, 200) : null
						});
					}
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
			case 'MESSAGE_DELETE_BULK':
				removeMessages(
					(data as { channel_id: string }).channel_id,
					(data as { message_ids: string[] }).message_ids
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

			// --- Relationship events (friend requests) ---
			case 'RELATIONSHIP_ADD': {
				const rel = data as Relationship;
				if (rel.type === 'pending_incoming') {
					const senderName = rel.user?.display_name ?? rel.user?.username ?? 'Someone';
					addNotification({
						type: 'friend_request',
						guild_id: null,
						guild_name: null,
						channel_id: null,
						channel_name: null,
						message_id: null,
						sender_id: rel.target_id,
						sender_name: senderName,
						content: `${senderName} sent you a friend request`
					});
				}
				break;
			}
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
