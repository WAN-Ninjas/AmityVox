// Gateway store — manages the WebSocket connection and dispatches events to stores.

import { writable, get } from 'svelte/store';
import { goto } from '$app/navigation';
import { GatewayClient } from '$lib/api/ws';
import { currentUser } from './auth';
import { loadGuilds, updateGuild, removeGuild, guilds as guildsStore } from './guilds';
import { updateChannel, removeChannel, channels as channelsStore, currentChannelId } from './channels';
import { appendMessage, updateMessage, removeMessage, removeMessages, loadMessages } from './messages';
import { updatePresence } from './presence';
import { addTypingUser, clearTypingUser } from './typing';
import { loadDMs, addDMChannel, removeDMChannel, updateUserInDMs } from './dms';
import { incrementUnread, loadReadState } from './unreads';
import { addNotification } from './notifications';
import { handleVoiceStateUpdate, clearChannelVoiceUsers } from './voice';
import { loadRelationships, addOrUpdateRelationship, removeRelationship } from './relationships';
import { loadPermissions, invalidatePermissions } from './permissions';
import { loadChannelMutePrefs, isChannelMuted, isGuildMuted } from './muting';
import { updateGuildMember, updateUserInMembers } from './members';
import { startIdleDetection, stopIdleDetection, setManualStatus } from '$lib/utils/idle';
import { addToast } from './toast';
import { clearChannelMessages } from './messages';
import type { User, Guild, Channel, Message, ReadyEvent, TypingEvent, Relationship } from '$lib/types';

export const gatewayConnected = writable(false);

let client: GatewayClient | null = null;
let hasReceivedReady = false;

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
				loadRelationships();
				loadChannelMutePrefs();
				// Preserve the user's chosen status. The DB defaults status_presence
				// to 'offline', which just means "never explicitly set" — treat as online.
				const raw = ready.user.status_presence;
				const savedStatus = (!raw || raw === 'offline') ? 'online' : raw;
				const displayStatus = savedStatus === 'invisible' ? 'offline' : savedStatus;
				updatePresence(ready.user.id, displayStatus);
				// Load initial presence for all online guild members.
				if (ready.presences) {
					for (const [uid, status] of Object.entries(ready.presences)) {
						updatePresence(uid, status as string);
					}
				}
				client?.updatePresence(savedStatus);
				setManualStatus(savedStatus);
				startIdleDetection();

				// Clear stale voice state then repopulate from READY payload.
				clearChannelVoiceUsers();
				if (ready.voice_states) {
					for (const vs of ready.voice_states) {
						handleVoiceStateUpdate({
							channel_id: vs.channel_id,
							user_id: vs.user_id,
							username: vs.username,
							display_name: vs.display_name,
							avatar_id: vs.avatar_id,
							muted: vs.self_mute,
							deafened: vs.self_deaf,
							action: 'join'
						});
					}
				}

				// E2EE is now passphrase-based — no init or welcome processing needed

				// Detect reconnection and refresh active channel data.
				const isReconnect = hasReceivedReady;
				hasReceivedReady = true;
				if (isReconnect) {
					const activeChannelId = get(currentChannelId);
					if (activeChannelId) {
						clearChannelMessages(activeChannelId);
						loadMessages(activeChannelId);
					}
					addToast('Reconnected to server', 'success', 3000);
				}
				break;
			}

			// --- Gateway lifecycle ---
			case 'GATEWAY_DISCONNECTED':
				// Connection dropped — mark disconnected immediately for UI feedback.
				gatewayConnected.set(false);
				addToast('Connection lost. Reconnecting...', 'warning', 5000);
				break;
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
			case 'GUILD_UPDATE': {
				const guild = data as Guild;
				updateGuild(guild);
				loadPermissions(guild.id);
				break;
			}
			case 'GUILD_DELETE': {
				const deletedGuild = data as { id: string };
				removeGuild(deletedGuild.id);
				invalidatePermissions(deletedGuild.id);
				break;
			}

			// --- Channel events ---
			case 'CHANNEL_CREATE':
			case 'CHANNEL_UPDATE':
			case 'THREAD_CREATE': {
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
				// Update last_activity_at for thread channels.
				const msgChannel = get(channelsStore).get(msg.channel_id);
				if (msgChannel?.parent_channel_id) {
					updateChannel({ ...msgChannel, last_activity_at: msg.created_at });
				}
				// Track unreads for messages not from self.
				let selfId: string | undefined;
				currentUser.subscribe((u) => (selfId = u?.id ?? undefined))();
				if (msg.author_id !== selfId) {
					const isMention = msg.mention_everyone ||
						(selfId ? msg.mention_user_ids?.includes(selfId) : false);
					incrementUnread(msg.channel_id, isMention);

					// Build notification for mentions, replies, and DMs.
					// Skip notification if channel or guild is muted (unreads still tracked above).
					const channel = get(channelsStore).get(msg.channel_id);
					const channelMuted = isChannelMuted(msg.channel_id);
					const guildMuted = channel?.guild_id ? isGuildMuted(channel.guild_id) : false;

					if (!channelMuted && !guildMuted) {
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
			case 'USER_UPDATE': {
				const updatedUser = data as User;
				let selfId: string | undefined;
				currentUser.subscribe((u) => (selfId = u?.id))();
				if (updatedUser.id === selfId) {
					// Own profile update
					currentUser.set(updatedUser);
				}
				// Update user data across all stores (works for self and others)
				updateUserInMembers(updatedUser);
				updateUserInDMs(updatedUser);
				break;
			}

			// --- Voice state events ---
			case 'VOICE_STATE_UPDATE':
				handleVoiceStateUpdate(data as {
					channel_id: string;
					user_id: string;
					username?: string;
					display_name?: string | null;
					avatar_id?: string | null;
					muted?: boolean;
					deafened?: boolean;
					action?: 'join' | 'leave' | 'update';
				});
				break;

			// --- Relationship events (friend requests) ---
			// --- Guild member events ---
			case 'GUILD_MEMBER_UPDATE': {
				const memberData = data as { guild_id: string; user_id: string; action?: string; roles?: string[] };
				// When the current user's roles/member data changes, reload permissions.
				let selfId: string | undefined;
				currentUser.subscribe((u) => (selfId = u?.id))();
				if (memberData.user_id === selfId && memberData.guild_id) {
					loadPermissions(memberData.guild_id);
				}
				// Update member store for real-time role display.
				if (memberData.roles !== undefined) {
					updateGuildMember(memberData.user_id, { roles: memberData.roles });
				}
				break;
			}

			case 'GUILD_ROLE_UPDATE':
			case 'GUILD_ROLE_DELETE': {
				// When a role is updated or deleted, reload permissions for the guild
				// since the role's permission bits may have changed.
				const roleData = data as { guild_id: string };
				if (roleData.guild_id) {
					loadPermissions(roleData.guild_id);
				}
				break;
			}

			case 'RELATIONSHIP_ADD': {
				const rel = data as Relationship;
				addOrUpdateRelationship(rel);
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
			case 'RELATIONSHIP_UPDATE': {
				const rel = data as Relationship;
				addOrUpdateRelationship(rel);
				break;
			}
			case 'RELATIONSHIP_REMOVE': {
				const rel = data as { target_id: string };
				removeRelationship(rel.target_id);
				break;
			}
		}
	});

	client.connect();
}

export function disconnectGateway() {
	stopIdleDetection();
	client?.disconnect();
	client = null;
	gatewayConnected.set(false);
	hasReceivedReady = false;
}

export function getGatewayClient(): GatewayClient | null {
	return client;
}
