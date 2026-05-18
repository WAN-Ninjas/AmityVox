<script lang="ts">
	import { api } from '$lib/api/client';
	import { channels as channelsStore } from '$lib/stores/channels';
	import { guilds as guildsStore } from '$lib/stores/guilds';
	import { getMutedChannels, getMutedGuilds, unmuteChannel, unmuteGuild } from '$lib/stores/muting';
	import {
		dndSchedule,
		dndManualOverride,
		isDndActive,
		notificationSoundsEnabled,
		notificationSoundPreset,
		notificationVolume,
		saveDndSchedule,
		saveDndManualOverride,
		saveNotificationSoundsEnabled,
		saveNotificationSoundPreset,
		saveNotificationVolume,
		syncSettingsToApi
	} from '$lib/stores/settings';
	import { SOUND_PRESETS, playNotificationSound } from '$lib/utils/sounds';
	import { ALL_NOTIFICATION_TYPES, NOTIFICATION_TYPE_LABELS, NOTIFICATION_CATEGORIES } from '$lib/utils/notificationHelpers';
	import { createAsyncOp } from '$lib/utils/asyncOp';
	import { getErrorMessage } from '$lib/utils/apiError';
	import type { NotificationTypePreference, ServerNotificationType } from '$lib/types';

	let desktopNotifications = $state(true);
	let notificationSounds = $state(true);
	let soundPreset = $state('default');
	let soundVolume = $state(80);
	let notifSuccess = $state('');
	let notifError = $state('');
	let dndEnabled = $state(false);
	let dndStartHour = $state(23);
	let dndStartMinute = $state(0);
	let dndEndHour = $state(7);
	let dndEndMinute = $state(0);
	let dndSuccess = $state('');
	let typePrefs = $state<Map<string, NotificationTypePreference>>(new Map());
	let typePrefsLoaded = $state(false);
	let typePrefsLoadFailed = $state(false);
	let typePrefsSuccess = $state('');
	let notifOp = $state(createAsyncOp());
	let dndOp = $state(createAsyncOp());
	let typePrefsOp = $state(createAsyncOp());
	let typePrefsSaveOp = $state(createAsyncOp());
	let loaded = false;

	$effect(() => {
		if (!loaded) {
			loaded = true;
			const schedule = $dndSchedule;
			dndEnabled = schedule.enabled;
			dndStartHour = schedule.startHour;
			dndStartMinute = schedule.startMinute;
			dndEndHour = schedule.endHour;
			dndEndMinute = schedule.endMinute;
			loadUserSettings();
			loadTypePrefs();
		}
	});

	function getTypePref(type: string): NotificationTypePreference {
		return typePrefs.get(type) ?? { type: type as ServerNotificationType, in_app: true, push: true, sound: true };
	}

	function setTypePref(type: string, field: 'in_app' | 'push' | 'sound', value: boolean) {
		const current = getTypePref(type);
		typePrefs.set(type, { ...current, [field]: value });
		typePrefs = new Map(typePrefs);
	}

	async function loadTypePrefs() {
		if (typePrefsLoaded || typePrefsOp.loading) return;
		typePrefsLoadFailed = false;
		notifError = '';
		await typePrefsOp.run(async () => {
			const prefs = await api.getNotificationTypePreferences();
			const map = new Map<string, NotificationTypePreference>();
			for (const pref of prefs) map.set(pref.type, pref);
			typePrefs = map;
			typePrefsLoaded = true;
		}, msg => {
			notifError = msg;
			typePrefsLoadFailed = true;
		}, 'Failed to load notification type preferences. Please try again.');
	}

	async function saveTypePrefs() {
		if (!typePrefsLoaded) {
			notifError = 'Type preferences have not loaded yet.';
			return;
		}
		typePrefsSuccess = '';
		notifError = '';
		await typePrefsSaveOp.run(async () => {
			const prefs = ALL_NOTIFICATION_TYPES.map((type) => getTypePref(type));
			await api.updateNotificationTypePreferences(prefs);
			typePrefsSuccess = 'Notification type preferences saved!';
			setTimeout(() => (typePrefsSuccess = ''), 3000);
		}, msg => {
			notifError = msg;
		}, 'Failed to save type preferences');
	}

	async function loadUserSettings() {
		try {
			const settings = await api.getUserSettings();
			desktopNotifications = settings.desktop_notifications ?? true;
			notificationSounds = settings.notification_sounds ?? true;
			soundPreset = settings.notification_sound_preset ?? 'default';
			soundVolume = settings.notification_volume ?? 80;
		} catch {
			// Use defaults if settings do not exist yet.
		}
	}

	async function saveNotifications() {
		notifSuccess = '';
		notifError = '';
		await notifOp.run(async () => {
			await api.updateUserSettings({
				desktop_notifications: desktopNotifications,
				notification_sounds: notificationSounds,
				notification_sound_preset: soundPreset,
				notification_volume: soundVolume
			});

			notificationSoundsEnabled.set(notificationSounds);
			notificationSoundPreset.set(soundPreset);
			notificationVolume.set(soundVolume);
			saveNotificationSoundsEnabled();
			saveNotificationSoundPreset();
			saveNotificationVolume();

			notifSuccess = 'Notification preferences saved!';
			setTimeout(() => (notifSuccess = ''), 3000);

			if (desktopNotifications && 'Notification' in window && Notification.permission === 'default') {
				await Notification.requestPermission();
			}
		}, msg => {
			notifError = msg;
		}, 'Failed to save notification preferences');
	}

	async function saveDnd() {
		dndSuccess = '';
		notifError = '';
		await dndOp.run(async () => {
			const schedule = {
				enabled: dndEnabled,
				startHour: dndStartHour,
				startMinute: dndStartMinute,
				endHour: dndEndHour,
				endMinute: dndEndMinute
			};
			dndSchedule.set(schedule);
			saveDndSchedule();
			await syncSettingsToApi();
			dndSuccess = 'Do Not Disturb schedule saved!';
			setTimeout(() => (dndSuccess = ''), 3000);
		}, msg => {
			notifError = msg;
		}, 'Failed to save DND schedule');
	}

	function toggleManualDnd() {
		dndManualOverride.update((value) => !value);
		saveDndManualOverride();
	}

	function formatTime(hour: number, minute: number): string {
		const h = hour.toString().padStart(2, '0');
		const m = minute.toString().padStart(2, '0');
		return `${h}:${m}`;
	}

	function parseTimeInput(value: string): { hour: number; minute: number } {
		const [h, m] = value.split(':').map(Number);
		return { hour: h ?? 0, minute: m ?? 0 };
	}
</script>

			<h1 class="mb-6 text-xl font-bold text-text-primary">Notifications</h1>

			{#if notifError}
				<div class="mb-4 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{notifError}</div>
			{/if}
			{#if notifSuccess}
				<div class="mb-4 rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{notifSuccess}</div>
			{/if}

			<div class="space-y-6">
				<div class="rounded-lg bg-bg-secondary p-4">
					<h3 class="mb-1 text-sm font-semibold text-text-primary">Desktop Notifications</h3>
					<p class="mb-3 text-xs text-text-muted">Show browser notifications for new messages and mentions.</p>
					<label class="flex items-center gap-2">
						<input type="checkbox" bind:checked={desktopNotifications} class="accent-brand-500" />
						<span class="text-sm text-text-secondary">Enable desktop notifications</span>
					</label>
					{#if 'Notification' in globalThis && Notification.permission === 'denied'}
						<p class="mt-2 text-xs text-red-400">
							Browser notifications are blocked. Please allow notifications in your browser settings.
						</p>
					{/if}
				</div>

				<div class="rounded-lg bg-bg-secondary p-4">
					<h3 class="mb-1 text-sm font-semibold text-text-primary">Notification Sounds</h3>
					<p class="mb-3 text-xs text-text-muted">Play a sound when you receive a notification.</p>
					<label class="flex items-center gap-2">
						<input type="checkbox" bind:checked={notificationSounds} class="accent-brand-500" />
						<span class="text-sm text-text-secondary">Enable notification sounds</span>
					</label>

					{#if notificationSounds}
						<div class="mt-4 space-y-4 border-t border-bg-modifier pt-4">
							<div>
								<label for="sound-preset" class="mb-1 block text-xs font-medium text-text-secondary">Sound Preset</label>
								<select
									id="sound-preset"
									bind:value={soundPreset}
									class="w-full rounded bg-bg-tertiary px-3 py-2 text-sm text-text-primary outline-none focus:ring-1 focus:ring-brand-500"
								>
									{#each SOUND_PRESETS as preset}
										<option value={preset.id}>{preset.name}</option>
									{/each}
								</select>
							</div>

							<div>
								<label for="sound-volume" class="mb-1 block text-xs font-medium text-text-secondary">
									Volume: {soundVolume}%
								</label>
								<input
									id="sound-volume"
									type="range"
									min="0"
									max="100"
									bind:value={soundVolume}
									class="w-full accent-brand-500"
								/>
							</div>

							<button
								class="rounded bg-bg-tertiary px-3 py-1.5 text-xs font-medium text-text-secondary transition-colors hover:bg-bg-modifier hover:text-text-primary"
								onclick={() => playNotificationSound(soundPreset, soundVolume)}
							>
								Preview Sound
							</button>
						</div>
					{/if}
				</div>

				<button class="btn-primary" onclick={saveNotifications} disabled={notifOp.loading}>
					{notifOp.loading ? 'Saving...' : 'Save Notification Preferences'}
				</button>

				<!-- ==================== PER-TYPE PREFERENCES ==================== -->
				<div class="border-t border-bg-modifier pt-6">
					<div class="flex items-center gap-3 mb-4">
						<h2 class="text-lg font-bold text-text-primary">Notification Types</h2>
					</div>
					<p class="mb-4 text-xs text-text-muted">
						Choose which notification types you want to receive in-app, as push notifications, or with sound.
					</p>

					{#if typePrefsSuccess}
						<div class="mb-4 rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{typePrefsSuccess}</div>
					{/if}

					{#if typePrefsOp.loading}
						<div class="flex items-center justify-center py-8">
							<div class="h-6 w-6 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
						</div>
					{:else if typePrefsLoadFailed}
						<div class="rounded-lg bg-bg-secondary p-4 text-sm text-text-muted">
							Failed to load notification type preferences.
							<button class="ml-2 text-brand-400 hover:text-brand-300" onclick={loadTypePrefs}>
								Retry
							</button>
						</div>
					{:else}
						{#each NOTIFICATION_CATEGORIES as category}
							<div class="mb-4 rounded-lg bg-bg-secondary p-4">
								<h3 class="mb-3 text-sm font-semibold text-text-primary">{category.label}</h3>
								<div class="space-y-2">
									<!-- Header row -->
									<div class="grid grid-cols-[1fr_60px_60px_60px] items-center gap-2 pb-1 text-2xs font-medium uppercase tracking-wide text-text-muted">
										<span>Type</span>
										<span class="text-center">In-App</span>
										<span class="text-center">Push</span>
										<span class="text-center">Sound</span>
									</div>
									{#each category.types as type}
										{@const pref = getTypePref(type)}
										<div class="grid grid-cols-[1fr_60px_60px_60px] items-center gap-2 rounded py-1.5 hover:bg-bg-modifier/50">
											<span class="text-sm text-text-secondary">{NOTIFICATION_TYPE_LABELS[type]}</span>
											<label class="flex justify-center">
												<input
													type="checkbox"
													checked={pref.in_app}
													onchange={() => setTypePref(type, 'in_app', !pref.in_app)}
													class="accent-brand-500"
												/>
											</label>
											<label class="flex justify-center">
												<input
													type="checkbox"
													checked={pref.push}
													onchange={() => setTypePref(type, 'push', !pref.push)}
													class="accent-brand-500"
												/>
											</label>
											<label class="flex justify-center">
												<input
													type="checkbox"
													checked={pref.sound}
													onchange={() => setTypePref(type, 'sound', !pref.sound)}
													class="accent-brand-500"
												/>
											</label>
										</div>
									{/each}
								</div>
							</div>
						{/each}

						<button class="btn-primary" onclick={saveTypePrefs} disabled={typePrefsSaveOp.loading || !typePrefsLoaded}>
							{typePrefsSaveOp.loading ? 'Saving...' : 'Save Type Preferences'}
						</button>
					{/if}
				</div>

				<!-- ==================== DO NOT DISTURB ==================== -->
				<div class="border-t border-bg-modifier pt-6">
					<div class="flex items-center gap-3 mb-4">
						<h2 class="text-lg font-bold text-text-primary">Do Not Disturb</h2>
						{#if $isDndActive}
							<span class="flex items-center gap-1.5 rounded-full bg-status-dnd/15 px-2.5 py-1 text-xs font-semibold text-status-dnd">
								<span class="h-2 w-2 rounded-full bg-status-dnd"></span>
								Active
							</span>
						{/if}
					</div>

					{#if dndSuccess}
						<div class="mb-4 rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{dndSuccess}</div>
					{/if}

					<!-- Manual DND toggle -->
					<div class="mb-6 rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">Manual Do Not Disturb</h3>
						<p class="mb-3 text-xs text-text-muted">
							Immediately enable DND mode. When active, all notifications are silently stored without alerts.
						</p>
						<label class="flex items-center gap-2">
							<input
								type="checkbox"
								checked={$dndManualOverride}
								onchange={toggleManualDnd}
								class="accent-brand-500"
							/>
							<span class="text-sm text-text-secondary">Enable Do Not Disturb now</span>
						</label>
					</div>

					<!-- Scheduled DND -->
					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">Scheduled Do Not Disturb</h3>
						<p class="mb-3 text-xs text-text-muted">
							Automatically enable DND during a recurring time window. Notifications during this period are silenced and marked as read.
						</p>

						<label class="mb-4 flex items-center gap-2">
							<input type="checkbox" bind:checked={dndEnabled} class="accent-brand-500" />
							<span class="text-sm text-text-secondary">Enable DND schedule</span>
						</label>

						{#if dndEnabled}
							<div class="space-y-4">
								<div class="flex items-center gap-4">
									<div class="flex-1">
										<label for="dnd-start" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
											Start Time
										</label>
										<input
											id="dnd-start"
											type="time"
											value={formatTime(dndStartHour, dndStartMinute)}
											onchange={(e) => {
												const t = parseTimeInput((e.target as HTMLInputElement).value);
												dndStartHour = t.hour;
												dndStartMinute = t.minute;
											}}
											class="input w-full"
										/>
									</div>
									<div class="mt-5 text-text-muted">to</div>
									<div class="flex-1">
										<label for="dnd-end" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
											End Time
										</label>
										<input
											id="dnd-end"
											type="time"
											value={formatTime(dndEndHour, dndEndMinute)}
											onchange={(e) => {
												const t = parseTimeInput((e.target as HTMLInputElement).value);
												dndEndHour = t.hour;
												dndEndMinute = t.minute;
											}}
											class="input w-full"
										/>
									</div>
								</div>

								<!-- Visual schedule bar -->
								<div class="rounded bg-bg-primary p-3">
									<p class="mb-2 text-xs text-text-muted">Schedule preview:</p>
									{#snippet scheduleBar()}
										{@const startPct = ((dndStartHour * 60 + dndStartMinute) / 1440) * 100}
										{@const endPct = ((dndEndHour * 60 + dndEndMinute) / 1440) * 100}
										{#if startPct <= endPct}
											<div
												class="absolute inset-y-0 bg-status-dnd/40"
												style="left: {startPct}%; width: {endPct - startPct}%"
											></div>
										{:else}
											<div
												class="absolute inset-y-0 bg-status-dnd/40"
												style="left: {startPct}%; right: 0"
											></div>
											<div
												class="absolute inset-y-0 bg-status-dnd/40"
												style="left: 0; width: {endPct}%"
											></div>
										{/if}
									{/snippet}
									<div class="relative h-6 overflow-hidden rounded-full bg-bg-modifier">
										{@render scheduleBar()}
										<!-- Time markers -->
										<div class="absolute inset-0 flex items-center justify-between px-2 text-2xs text-text-muted">
											<span>00:00</span>
											<span>06:00</span>
											<span>12:00</span>
											<span>18:00</span>
											<span>24:00</span>
										</div>
									</div>
									<p class="mt-1.5 text-xs text-text-secondary">
										DND active from {formatTime(dndStartHour, dndStartMinute)} to {formatTime(dndEndHour, dndEndMinute)}
										{#if dndStartHour * 60 + dndStartMinute > dndEndHour * 60 + dndEndMinute}
											(overnight)
										{/if}
									</p>
								</div>
							</div>
						{/if}

						<button class="btn-primary mt-4" onclick={saveDnd} disabled={dndOp.loading}>
							{dndOp.loading ? 'Saving...' : 'Save DND Schedule'}
						</button>
					</div>
				</div>

				<!-- ==================== MUTED ITEMS ==================== -->
				<div class="border-t border-bg-modifier pt-6">
					<h2 class="mb-4 text-lg font-bold text-text-primary">Muted Channels & Servers</h2>

					{#if getMutedChannels().length === 0 && getMutedGuilds().length === 0}
						<p class="text-sm text-text-muted">No muted channels or servers.</p>
					{:else}
						<div class="space-y-2">
							{#each getMutedGuilds() as guildPref}
								{@const guild = $guildsStore.get(guildPref.guild_id ?? '')}
								<div class="flex items-center justify-between rounded-lg bg-bg-secondary p-3">
									<div class="min-w-0 flex-1">
										<p class="truncate text-sm font-medium text-text-primary">
											{guild?.name ?? guildPref.guild_id ?? 'Unknown Server'}
										</p>
										<p class="text-xs text-text-muted">
											Server
											{#if guildPref.muted_until}
												&mdash; until {new Date(guildPref.muted_until).toLocaleString()}
											{:else}
												&mdash; indefinitely
											{/if}
										</p>
									</div>
									<button
										class="rounded bg-bg-tertiary px-3 py-1.5 text-xs font-medium text-text-secondary transition-colors hover:bg-bg-modifier hover:text-text-primary"
										onclick={() => unmuteGuild(guildPref.guild_id ?? '')}
									>
										Unmute
									</button>
								</div>
							{/each}
							{#each getMutedChannels() as chPref}
								{@const ch = $channelsStore.get(chPref.channel_id)}
								{@const chGuild = ch?.guild_id ? $guildsStore.get(ch.guild_id) : null}
								<div class="flex items-center justify-between rounded-lg bg-bg-secondary p-3">
									<div class="min-w-0 flex-1">
										<p class="truncate text-sm font-medium text-text-primary">
											{#if ch?.channel_type === 'dm' || ch?.channel_type === 'group'}
												DM: {ch?.name ?? 'Direct Message'}
											{:else}
												#{ch?.name ?? chPref.channel_id}
											{/if}
											{#if chGuild}
												<span class="text-text-muted"> in {chGuild.name}</span>
											{/if}
										</p>
										<p class="text-xs text-text-muted">
											Channel
											{#if chPref.muted_until}
												&mdash; until {new Date(chPref.muted_until).toLocaleString()}
											{:else}
												&mdash; indefinitely
											{/if}
										</p>
									</div>
									<button
										class="rounded bg-bg-tertiary px-3 py-1.5 text-xs font-medium text-text-secondary transition-colors hover:bg-bg-modifier hover:text-text-primary"
										onclick={() => unmuteChannel(chPref.channel_id)}
									>
										Unmute
									</button>
								</div>
							{/each}
						</div>
					{/if}
				</div>
			</div>
