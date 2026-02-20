// Gateway store — manages the WebSocket connection and dispatches events to stores.

import { writable, get } from 'svelte/store';
import { goto } from '$app/navigation';
import { GatewayClient } from '$lib/api/ws';
import { currentUser } from './auth';
import { loadGuilds, loadFederatedGuilds, updateGuild, removeGuild, guilds as guildsStore, currentGuildId, isFederatedGuild } from './guilds';
import { updateChannel, removeChannel, loadChannels, channels as channelsStore, currentChannelId } from './channels';
import { appendMessage, updateMessage, removeMessage, removeMessages, loadMessages } from './messages';
import { updatePresence } from './presence';
import { addTypingUser, clearTypingUser } from './typing';
import { loadDMs, addDMChannel, removeDMChannel, updateUserInDMs } from './dms';
import { incrementUnread, loadReadState, loadChannelGuildMap, registerChannelGuild } from './unreads';
import { handleNotificationCreate, handleNotificationUpdate, handleNotificationDelete, loadNotifications } from './notifications';
import { initPushNotifications } from '$lib/utils/pushNotifications';
import { handleVoiceStateUpdate, clearChannelVoiceUsers } from './voice';
import { loadRelationships, addOrUpdateRelationship, removeRelationship } from './relationships';
import { loadPermissions, invalidatePermissions } from './permissions';
import { loadChannelMutePrefs, isChannelMuted, isGuildMuted } from './muting';
import { updateGuildMember, updateUserInMembers, guildMembers } from './members';
import { startIdleDetection, stopIdleDetection, setManualStatus } from '$lib/utils/idle';
import { addToast } from './toast';
import { clearChannelMessages } from './messages';
import { addAnnouncement, updateAnnouncement, removeAnnouncement } from './announcements';
import { addIncomingCall, dismissIncomingCall, clearIncomingCalls } from './callRing';
import { clearChannelUnreads } from './unreads';
import type { User, Guild, Channel, Message, ReadyEvent, TypingEvent, Relationship, ServerNotification } from '$lib/types';

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
				if (ready.federated_guilds) {
					loadFederatedGuilds(ready.federated_guilds);
				}
				loadDMs();
				loadReadState();
				loadChannelGuildMap();
				loadRelationships();
				loadChannelMutePrefs();
				loadNotifications();
				initPushNotifications();
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
						const guildId = get(currentGuildId);
						const fedGuildId = guildId && isFederatedGuild(guildId) ? guildId : null;
						loadMessages(activeChannelId, undefined, fedGuildId);
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
			case 'GUILD_CREATE': {
				const guild = data as Guild;
				updateGuild(guild);
				loadPermissions(guild.id);
				loadChannels(guild.id);
				break;
			}
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
				// Track channel→guild mapping for unread indicators.
				if (ch.guild_id) registerChannelGuild(ch.id, ch.guild_id);
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
					// Check if this message mentions the current user (direct, @here, or role mention).
				let isMention = msg.mention_here ||
						(selfId ? msg.mention_user_ids?.includes(selfId) : false);
				if (!isMention && selfId && msg.mention_role_ids?.length > 0) {
					// Only check role mentions if the message's channel belongs to the
					// currently viewed guild, since guildMembers only holds members for
					// that guild.
					const activeGuildId = get(currentGuildId);
					if (activeGuildId && msgChannel?.guild_id === activeGuildId) {
						const member = get(guildMembers).get(selfId);
						if (member?.roles?.some(r => msg.mention_role_ids.includes(r))) {
							isMention = true;
						}
					}
				}
					incrementUnread(msg.channel_id, isMention);
					// Notifications are now server-generated via NOTIFICATION_CREATE events.
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
			case 'VOICE_STATE_UPDATE': {
				const vs = data as {
					channel_id: string;
					user_id: string;
					username?: string;
					display_name?: string | null;
					avatar_id?: string | null;
					muted?: boolean;
					deafened?: boolean;
					action?: 'join' | 'leave' | 'update';
				};
				handleVoiceStateUpdate(vs);
				// If the caller left, dismiss the incoming call ring.
				if (vs.action === 'leave' && vs.channel_id) {
					dismissIncomingCall(vs.channel_id);
				}
				break;
			}

			// --- Incoming call ring (DM/Group calls) ---
			case 'CALL_RING': {
				const ring = data as {
					channel_id: string;
					caller_id: string;
					caller_name: string;
					caller_display_name?: string | null;
					caller_avatar_id?: string | null;
					channel_type: 'dm' | 'group';
				};
				addIncomingCall({
					channelId: ring.channel_id,
					callerId: ring.caller_id,
					callerName: ring.caller_name,
					callerDisplayName: ring.caller_display_name,
					callerAvatarId: ring.caller_avatar_id,
					channelType: ring.channel_type,
					timestamp: Date.now()
				});
				break;
			}

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
				// Friend request notifications are now server-generated via NOTIFICATION_CREATE.
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

			// --- Guild member add/remove events ---
			case 'GUILD_MEMBER_ADD': {
				const member = data as { guild_id: string; user_id: string; username?: string };
				// Reload permissions when a new member joins (could affect role distribution).
				let selfId: string | undefined;
				currentUser.subscribe((u) => (selfId = u?.id))();
				if (member.user_id === selfId && member.guild_id) {
					// We joined a new guild — reload guilds and permissions.
					loadGuilds();
					loadPermissions(member.guild_id);
				}
				break;
			}
			case 'GUILD_MEMBER_REMOVE': {
				const removed = data as { guild_id: string; user_id: string };
				let selfId: string | undefined;
				currentUser.subscribe((u) => (selfId = u?.id))();
				if (removed.user_id === selfId) {
					// We were removed from the guild.
					removeGuild(removed.guild_id);
					invalidatePermissions(removed.guild_id);
				}
				break;
			}

			// --- Guild role create ---
			case 'GUILD_ROLE_CREATE': {
				const roleData = data as { guild_id: string };
				if (roleData.guild_id) {
					loadPermissions(roleData.guild_id);
				}
				break;
			}

			// --- Guild ban events ---
			case 'GUILD_BAN_ADD': {
				const ban = data as { guild_id: string; user_id: string };
				let selfId: string | undefined;
				currentUser.subscribe((u) => (selfId = u?.id))();
				if (ban.user_id === selfId) {
					removeGuild(ban.guild_id);
					invalidatePermissions(ban.guild_id);
					addToast('You have been banned from a server', 'error', 5000);
				}
				break;
			}
			case 'GUILD_BAN_REMOVE':
				// No-op for UI; user would need to re-join via invite.
				break;

			// --- Guild emoji update ---
			case 'GUILD_EMOJI_UPDATE':
			case 'GUILD_EMOJI_DELETE':
				// Emoji changes — the guild store will pick up changes on next load.
				break;

			// --- Guild scheduled events ---
			case 'GUILD_EVENT_CREATE':
			case 'GUILD_EVENT_UPDATE':
			case 'GUILD_EVENT_DELETE':
				// Scheduled event changes — currently no dedicated frontend store.
				break;

			// --- Guild onboarding ---
			case 'GUILD_ONBOARDING_UPDATE':
				// Onboarding config changed — no-op for non-admin users.
				break;

			// --- Channel pins update ---
			case 'CHANNEL_PINS_UPDATE':
				// Pin count changed — components that display pins will refetch on focus.
				break;

			// --- Channel ACK (read state, user-scoped) ---
			case 'CHANNEL_ACK': {
				const ack = data as { channel_id: string; message_id?: string };
				clearChannelUnreads(ack.channel_id);
				break;
			}

			// --- Channel widget events ---
			case 'CHANNEL_WIDGET_CREATE':
			case 'CHANNEL_WIDGET_UPDATE':
			case 'CHANNEL_WIDGET_DELETE':
				// Widget changes — no dedicated frontend store yet.
				break;

			// --- Message reaction events ---
			case 'MESSAGE_REACTION_ADD':
			case 'MESSAGE_REACTION_REMOVE': {
				// Reaction events include the full updated reactions array from the backend.
				// Update the message with the new reaction data.
				const reaction = data as { channel_id: string; message_id: string; reactions?: unknown[] };
				if (reaction.reactions !== undefined) {
					updateMessage({ id: reaction.message_id, channel_id: reaction.channel_id, reactions: reaction.reactions } as Message);
				}
				break;
			}

			// --- Message embed update (link unfurl) ---
			case 'MESSAGE_EMBED_UPDATE': {
				const embed = data as { channel_id: string; message_id: string; embeds?: unknown[] };
				if (embed.embeds !== undefined) {
					updateMessage({ id: embed.message_id, channel_id: embed.channel_id, embeds: embed.embeds } as Message);
				}
				break;
			}

			// --- Poll events ---
			case 'POLL_CREATE':
			case 'POLL_VOTE':
			case 'POLL_CLOSE': {
				const poll = data as { channel_id: string; poll_id?: string; message_id?: string };
				// Polls are embedded in messages — re-fetch the channel messages.
				if (poll.channel_id) {
					const activeChannelId = get(currentChannelId);
					if (activeChannelId === poll.channel_id) {
						// Refresh messages in active channel to get updated poll state.
						clearChannelMessages(poll.channel_id);
						loadMessages(poll.channel_id);
					}
				}
				break;
			}

			// --- Automod action ---
			case 'AUTOMOD_ACTION':
				// Automod notification — could show a toast for guild moderators.
				break;

			// --- Server notifications (persistent, server-backed) ---
			case 'NOTIFICATION_CREATE':
				handleNotificationCreate(data as ServerNotification);
				break;
			case 'NOTIFICATION_UPDATE':
				handleNotificationUpdate(data as { id: string; read: boolean });
				break;
			case 'NOTIFICATION_DELETE':
				handleNotificationDelete(data as { id: string });
				break;

			// --- Activity/game events ---
			case 'ACTIVITY_SESSION_START':
			case 'ACTIVITY_SESSION_END':
			case 'ACTIVITY_PARTICIPANT_JOIN':
			case 'ACTIVITY_PARTICIPANT_LEAVE':
			case 'ACTIVITY_STATE_UPDATE':
			case 'WATCH_TOGETHER_START':
			case 'WATCH_TOGETHER_SYNC':
			case 'MUSIC_PARTY_START':
			case 'MUSIC_PARTY_QUEUE_ADD':
			case 'GAME_SESSION_CREATE':
			case 'GAME_PLAYER_JOIN':
			case 'GAME_MOVE':
				// Activity/game events — handled by activity-specific components
				// via their own event subscriptions when mounted.
				break;

			// --- Soundboard events ---
			case 'SOUNDBOARD_PLAY':
				// Soundboard play — handled by voice panel component.
				break;

			// --- Voice broadcast events ---
			case 'VOICE_BROADCAST_START':
			case 'VOICE_BROADCAST_END':
				// Voice broadcast lifecycle — handled by voice components.
				break;

			// --- Screen share events ---
			case 'SCREEN_SHARE_START':
			case 'SCREEN_SHARE_END':
				// Screen share lifecycle — handled by voice components.
				break;

			// --- Location share events ---
			case 'LOCATION_SHARE_UPDATE':
			case 'LOCATION_SHARE_END':
				// Location share — handled by LocationShare component.
				break;

			// --- Bot presence ---
			case 'BOT_PRESENCE_UPDATE': {
				const bot = data as { bot_id: string; status: string };
				updatePresence(bot.bot_id, bot.status);
				break;
			}

			// --- Component interaction (for bots) ---
			case 'COMPONENT_INTERACTION':
				// Bot component interaction — handled by message components.
				break;

			// --- Announcement events (instance-wide) ---
			case 'ANNOUNCEMENT_CREATE':
				addAnnouncement(data as { id: string; title: string; content: string; severity: string; active?: boolean; expires_at?: string | null });
				break;
			case 'ANNOUNCEMENT_UPDATE':
				updateAnnouncement(data as { id: string; active?: boolean | null; title?: string | null; content?: string | null });
				break;
			case 'ANNOUNCEMENT_DELETE':
				removeAnnouncement((data as { id: string }).id);
				break;
		}
	});

	client.connect();
}

export function disconnectGateway() {
	stopIdleDetection();
	clearIncomingCalls();
	client?.disconnect();
	client = null;
	gatewayConnected.set(false);
	hasReceivedReady = false;
}

export function getGatewayClient(): GatewayClient | null {
	return client;
}
