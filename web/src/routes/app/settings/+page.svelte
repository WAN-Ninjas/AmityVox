<script lang="ts">
	import { onMount } from 'svelte';
	import { currentUser, logout } from '$lib/stores/auth';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import Avatar from '$components/common/Avatar.svelte';
	import { e2ee } from '$lib/encryption/e2eeManager';
	import { getMutedChannels, getMutedGuilds, unmuteChannel, unmuteGuild } from '$lib/stores/muting';
	import { channels as channelsStore } from '$lib/stores/channels';
	import { guilds as guildsStore } from '$lib/stores/guilds';
	import ProfileLinkEditor from '$components/common/ProfileLinkEditor.svelte';
	import ImageCropper from '$components/common/ImageCropper.svelte';
	import Modal from '$components/common/Modal.svelte';
	import type { Session } from '$lib/types';
	import {
		customThemes,
		activeCustomThemeName,
		customCss,
		dndSchedule,
		dndManualOverride,
		isDndActive,
		notificationSoundsEnabled,
		notificationSoundPreset,
		notificationVolume,
		addCustomTheme,
		deleteCustomTheme,
		activateCustomTheme,
		deactivateCustomTheme,
		exportTheme,
		importTheme,
		getCurrentThemeColors,
		applyCustomThemeColors,
		clearCustomThemeColors,
		saveDndSchedule,
		saveDndManualOverride,
		saveNotificationSoundsEnabled,
		saveNotificationSoundPreset,
		saveNotificationVolume,
		saveCustomCss,
		clearCustomCss,
		syncSettingsToApi,
		DEFAULT_THEME_COLORS,
		THEME_COLOR_LABELS,
		THEME_COLOR_GROUPS,
		type CustomThemeColors,
		type CustomTheme
	} from '$lib/stores/settings';
	import { SOUND_PRESETS, playNotificationSound } from '$lib/utils/sounds';

	import type { User, BotToken, SlashCommand, VoicePreferences } from '$lib/types';

	type Tab = 'account' | 'security' | 'notifications' | 'privacy' | 'appearance' | 'voice' | 'encryption' | 'bots' | 'data';
	let currentTab = $state<Tab>('account');

	// --- Account tab state ---
	let displayName = $state('');
	let bio = $state('');
	let statusText = $state('');
	let saving = $state(false);
	let error = $state('');
	let success = $state('');
	let avatarFile = $state<File | null>(null);
	let avatarPreview = $state<string | null>(null);
	let accentColor = $state('#5865f2');
	let bannerFile = $state<File | null>(null);
	let bannerPreview = $state<string | null>(null);
	let bannerRemoved = $state(false);

	// Image cropper state
	let cropperFile = $state<File | null>(null);
	let cropperTarget = $state<'avatar' | 'banner'>('avatar');

	// --- Security tab state ---
	let currentPassword = $state('');
	let newPassword = $state('');
	let confirmPassword = $state('');
	let changingPassword = $state(false);
	let passwordError = $state('');
	let passwordSuccess = $state('');

	// --- 2FA state ---
	let totpSecret = $state('');
	let totpQrUrl = $state('');
	let totpCode = $state('');
	let backupCodes = $state<string[]>([]);
	let enablingTotp = $state(false);
	let verifyingTotp = $state(false);
	let totpError = $state('');
	let totpStep = $state<'idle' | 'setup' | 'verify' | 'done'>('idle');

	// --- Sessions state ---
	let sessions = $state<Session[]>([]);
	let loadingSessions = $state(false);
	let revokingSession = $state<string | null>(null);

	// --- Notifications tab state ---
	let desktopNotifications = $state(true);
	let notificationSounds = $state(true);
	let soundPreset = $state('default');
	let soundVolume = $state(80);
	let notifLoading = $state(false);
	let notifSuccess = $state('');

	// --- DND Schedule state ---
	let dndEnabled = $state(false);
	let dndStartHour = $state(23);
	let dndStartMinute = $state(0);
	let dndEndHour = $state(7);
	let dndEndMinute = $state(0);
	let dndSaving = $state(false);
	let dndSuccess = $state('');

	// --- Privacy tab state ---
	let dmPrivacy = $state<'everyone' | 'friends' | 'nobody'>('everyone');
	let friendRequestPrivacy = $state<'everyone' | 'mutual_guilds' | 'nobody'>('everyone');
	let nsfwContentFilter = $state<'blur_all' | 'blur_suspicious' | 'show_all'>('blur_all');
	let privacyLoading = $state(false);
	let privacySuccess = $state('');

	// --- Appearance tab state ---
	type ThemeName = 'dark' | 'light' | 'amoled' | 'nord' | 'dracula' | 'catppuccin' | 'solarized' | 'high-contrast';
	let theme = $state<ThemeName>('dark');
	let fontSize = $state(16);
	let compactMode = $state(false);
	let reducedMotion = $state(false);
	let dyslexicFont = $state(false);

	// --- Theme Editor state ---
	let showThemeEditor = $state(false);
	let editorColors = $state<CustomThemeColors>({ ...DEFAULT_THEME_COLORS });
	let editorThemeName = $state('');
	let editorBasePreset = $state<ThemeName | ''>('');
	let editingExistingTheme = $state(false);
	let editorError = $state('');
	let editorSuccess = $state('');
	let showImportModal = $state(false);
	let importJson = $state('');
	let importError = $state('');
	let importFileInput: HTMLInputElement;

	// --- Custom CSS state ---
	let customCssText = $state('');
	let customCssSuccess = $state('');
	const customCssMaxLength = 10000;

	// --- Connected Accounts state ---
	interface ConnectedAccounts {
		github: string;
		twitter: string;
		website: string;
	}
	let connectedAccounts = $state<ConnectedAccounts>({ github: '', twitter: '', website: '' });
	let connectedAccountsSuccess = $state('');

	// --- Bots tab state ---
	let myBots = $state<User[]>([]);
	let loadingBots = $state(false);
	let newBotName = $state('');
	let newBotDescription = $state('');
	let creatingBot = $state(false);
	let botError = $state('');
	let botSuccess = $state('');
	let expandedBotId = $state<string | null>(null);
	let botTokens = $state<Record<string, BotToken[]>>({});
	let botCommands = $state<Record<string, SlashCommand[]>>({});
	let loadingBotTokens = $state<string | null>(null);
	let loadingBotCommands = $state<string | null>(null);
	let newTokenName = $state('');
	let createdTokenRaw = $state<string | null>(null);
	let creatingToken = $state(false);
	let editingBotId = $state<string | null>(null);
	let editBotName = $state('');
	let editBotDescription = $state('');
	let savingBot = $state(false);
	let newCommandName = $state('');
	let newCommandDescription = $state('');
	let creatingCommand = $state(false);

	// --- Voice & Video tab state ---
	let voicePrefs = $state<VoicePreferences | null>(null);
	let voiceLoading = $state(false);
	let voiceSaving = $state(false);
	let voiceSuccess = $state('');
	let voiceError = $state('');
	let inputDeviceId = $state('');
	let outputDeviceId = $state('');
	let availableInputDevices = $state<MediaDeviceInfo[]>([]);
	let availableOutputDevices = $state<MediaDeviceInfo[]>([]);
	let recordingVoicePTTKey = $state(false);

	// --- Data & Privacy tab state ---
	let exportingData = $state(false);
	let exportDataSuccess = $state('');
	let exportDataError = $state('');
	let exportingAccount = $state(false);
	let exportAccountSuccess = $state('');
	let exportAccountError = $state('');
	let importingAccount = $state(false);
	let importAccountSuccess = $state('');
	let importAccountError = $state('');
	let accountImportFileInput: HTMLInputElement | undefined;

	function handleBannerSelect(e: Event) {
		const target = e.target as HTMLInputElement;
		const file = target.files?.[0];
		if (!file) return;
		cropperTarget = 'banner';
		cropperFile = file;
		target.value = '';
	}

	onMount(() => {
		if ($currentUser) {
			displayName = $currentUser.display_name ?? '';
			bio = $currentUser.bio ?? '';
			statusText = $currentUser.status_text ?? '';
			accentColor = $currentUser.accent_color ?? '#5865f2';
		}

		theme = (localStorage.getItem('av-theme') as ThemeName) ?? 'dark';
		fontSize = parseInt(localStorage.getItem('av-font-size') ?? '16', 10);
		compactMode = localStorage.getItem('av-compact') === 'true';
		reducedMotion = localStorage.getItem('av-reduced-motion') === 'true';
		dyslexicFont = localStorage.getItem('av-dyslexic-font') === 'true';

		// Load NSFW filter from localStorage.
		const storedNsfwFilter = localStorage.getItem('av-nsfw-filter');
		if (storedNsfwFilter === 'blur_all' || storedNsfwFilter === 'blur_suspicious' || storedNsfwFilter === 'show_all') {
			nsfwContentFilter = storedNsfwFilter;
		}

		// Load DND schedule from store.
		const schedule = $dndSchedule;
		dndEnabled = schedule.enabled;
		dndStartHour = schedule.startHour;
		dndStartMinute = schedule.startMinute;
		dndEndHour = schedule.endHour;
		dndEndMinute = schedule.endMinute;

		// Load custom CSS from store.
		customCssText = $customCss;

		// Load connected accounts from localStorage.
		try {
			const stored = localStorage.getItem('av-connected-accounts');
			if (stored) {
				const parsed = JSON.parse(stored);
				connectedAccounts = { github: parsed.github ?? '', twitter: parsed.twitter ?? '', website: parsed.website ?? '' };
			}
		} catch {
			// Ignore malformed JSON.
		}
	});

	// --- Account actions ---

	async function handleSaveProfile() {
		saving = true;
		error = '';
		success = '';

		try {
			// Upload avatar if selected.
			let avatarId: string | undefined;
			if (avatarFile) {
				const uploaded = await api.uploadFile(avatarFile);
				avatarId = uploaded.id;
			}

			// Upload banner if selected.
			let bannerId: string | undefined;
			if (bannerFile) {
				const uploaded = await api.uploadFile(bannerFile);
				bannerId = uploaded.id;
			}

			const payload: Record<string, unknown> = {
				display_name: displayName || undefined,
				bio: bio || undefined,
				status_text: statusText || undefined,
				accent_color: accentColor || undefined
			};
			if (avatarId) payload.avatar_id = avatarId;
			if (bannerId) payload.banner_id = bannerId;
			if (bannerRemoved && !bannerId) payload.banner_id = null;

			const updated = await api.updateMe(payload as any);
			currentUser.set(updated);
			avatarFile = null;
			avatarPreview = null;
			bannerFile = null;
			bannerPreview = null;
			bannerRemoved = false;
			success = 'Profile updated!';
			setTimeout(() => (success = ''), 3000);
		} catch (err: any) {
			error = err.message || 'Failed to save';
		} finally {
			saving = false;
		}
	}

	function handleAvatarSelect(e: Event) {
		const target = e.target as HTMLInputElement;
		const file = target.files?.[0];
		if (!file) return;
		if (!file.type.startsWith('image/')) {
			error = 'Please select an image file.';
			return;
		}
		cropperTarget = 'avatar';
		cropperFile = file;
		// Reset input so re-selecting the same file triggers the event
		target.value = '';
	}

	function handleCropComplete(blob: Blob) {
		const file = new File([blob], `${cropperTarget}.png`, { type: 'image/png' });
		if (cropperTarget === 'avatar') {
			avatarFile = file;
			avatarPreview = URL.createObjectURL(blob);
		} else {
			bannerFile = file;
			bannerPreview = URL.createObjectURL(blob);
		}
		cropperFile = null;
	}

	// --- Security actions ---

	async function handleChangePassword() {
		if (newPassword !== confirmPassword) {
			passwordError = 'Passwords do not match.';
			return;
		}
		if (newPassword.length < 8) {
			passwordError = 'Password must be at least 8 characters.';
			return;
		}

		changingPassword = true;
		passwordError = '';
		passwordSuccess = '';

		try {
			await api.changePassword(currentPassword, newPassword);
			currentPassword = '';
			newPassword = '';
			confirmPassword = '';
			passwordSuccess = 'Password changed successfully!';
			setTimeout(() => (passwordSuccess = ''), 3000);
		} catch (err: any) {
			passwordError = err.message || 'Failed to change password';
		} finally {
			changingPassword = false;
		}
	}

	async function handleEnableTotp() {
		enablingTotp = true;
		totpError = '';
		try {
			const result = await api.enableTOTP();
			totpSecret = result.secret;
			totpQrUrl = result.qr_url;
			totpStep = 'setup';
		} catch (err: any) {
			totpError = err.message || 'Failed to enable 2FA';
		} finally {
			enablingTotp = false;
		}
	}

	async function handleVerifyTotp() {
		if (totpCode.length !== 6) {
			totpError = 'Enter a 6-digit code.';
			return;
		}
		verifyingTotp = true;
		totpError = '';
		try {
			const result = await api.verifyTOTP(totpCode);
			backupCodes = result.backup_codes;
			totpStep = 'done';
			totpCode = '';
		} catch (err: any) {
			totpError = err.message || 'Invalid code';
		} finally {
			verifyingTotp = false;
		}
	}

	function resetTotpFlow() {
		totpStep = 'idle';
		totpSecret = '';
		totpQrUrl = '';
		totpCode = '';
		backupCodes = [];
		totpError = '';
	}

	// --- Sessions ---

	async function loadSessions() {
		loadingSessions = true;
		try {
			sessions = await api.getSessions();
		} catch {
			sessions = [];
		} finally {
			loadingSessions = false;
		}
	}

	async function revokeSession(sessionId: string) {
		revokingSession = sessionId;
		try {
			await api.deleteSession(sessionId);
			sessions = sessions.filter((s) => s.id !== sessionId);
		} catch (err: any) {
			alert(err.message || 'Failed to revoke session');
		} finally {
			revokingSession = null;
		}
	}

	// Load sessions when switching to security tab.
	$effect(() => {
		if (currentTab === 'security') {
			loadSessions();
		}
	});

	// Load settings when switching to notifications/privacy tabs.
	$effect(() => {
		if (currentTab === 'notifications' || currentTab === 'privacy') {
			loadUserSettings();
		}
	});

	async function loadUserSettings() {
		try {
			const settings = await api.getUserSettings();
			desktopNotifications = settings.desktop_notifications ?? true;
			notificationSounds = settings.notification_sounds ?? true;
			soundPreset = settings.notification_sound_preset ?? 'default';
			soundVolume = settings.notification_volume ?? 80;
			dmPrivacy = settings.dm_privacy ?? 'everyone';
			friendRequestPrivacy = settings.friend_request_privacy ?? 'everyone';
			nsfwContentFilter = settings.nsfw_content_filter ?? 'blur_all';
		} catch {
			// Use defaults if settings don't exist yet.
		}
	}

	async function saveNotifications() {
		notifLoading = true;
		notifSuccess = '';
		try {
			await api.updateUserSettings({
				desktop_notifications: desktopNotifications,
				notification_sounds: notificationSounds,
				notification_sound_preset: soundPreset,
				notification_volume: soundVolume
			});

			// Update the global stores so the notifications store uses the new values.
			notificationSoundsEnabled.set(notificationSounds);
			notificationSoundPreset.set(soundPreset);
			notificationVolume.set(soundVolume);
			saveNotificationSoundsEnabled();
			saveNotificationSoundPreset();
			saveNotificationVolume();

			notifSuccess = 'Notification preferences saved!';
			setTimeout(() => (notifSuccess = ''), 3000);

			// Request browser notification permission if enabling desktop notifications.
			if (desktopNotifications && 'Notification' in window && Notification.permission === 'default') {
				await Notification.requestPermission();
			}
		} catch (err: any) {
			error = err.message || 'Failed to save';
		} finally {
			notifLoading = false;
		}
	}

	// --- DND Schedule ---

	async function saveDnd() {
		dndSaving = true;
		dndSuccess = '';
		try {
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
		} catch (err: any) {
			error = err.message || 'Failed to save DND schedule';
		} finally {
			dndSaving = false;
		}
	}

	function toggleManualDnd() {
		dndManualOverride.update((v) => !v);
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

	async function savePrivacy() {
		privacyLoading = true;
		privacySuccess = '';
		try {
			await api.updateUserSettings({
				dm_privacy: dmPrivacy,
				friend_request_privacy: friendRequestPrivacy,
				nsfw_content_filter: nsfwContentFilter
			});
			// Persist NSFW filter to localStorage for MessageItem to read.
			localStorage.setItem('av-nsfw-filter', nsfwContentFilter);
			privacySuccess = 'Privacy settings saved!';
			setTimeout(() => (privacySuccess = ''), 3000);
		} catch (err: any) {
			error = err.message || 'Failed to save';
		} finally {
			privacyLoading = false;
		}
	}

	// --- Appearance ---

	function saveAppearance() {
		localStorage.setItem('av-theme', theme);
		localStorage.setItem('av-font-size', String(fontSize));
		localStorage.setItem('av-compact', String(compactMode));
		localStorage.setItem('av-reduced-motion', String(reducedMotion));
		localStorage.setItem('av-dyslexic-font', String(dyslexicFont));
		document.documentElement.setAttribute('data-theme', theme);
		document.documentElement.style.fontSize = `${fontSize}px`;
		if (compactMode) {
			document.documentElement.setAttribute('data-compact', 'true');
		} else {
			document.documentElement.removeAttribute('data-compact');
		}
		if (reducedMotion) {
			document.documentElement.setAttribute('data-reduced-motion', 'true');
		} else {
			document.documentElement.removeAttribute('data-reduced-motion');
		}
		if (dyslexicFont) {
			document.documentElement.setAttribute('data-dyslexic-font', 'true');
		} else {
			document.documentElement.removeAttribute('data-dyslexic-font');
		}
		success = 'Appearance settings saved!';
		setTimeout(() => (success = ''), 3000);
	}

	// --- Theme Editor ---

	function openThemeEditor() {
		// Initialize editor with the current CSS variable values.
		editorColors = getCurrentThemeColors();
		editorThemeName = '';
		editorBasePreset = theme;
		editingExistingTheme = false;
		editorError = '';
		editorSuccess = '';
		showThemeEditor = true;
	}

	function handleBasePresetChange(preset: ThemeName) {
		editorBasePreset = preset;
		// Temporarily set the data-theme to read computed values, then restore.
		const root = document.documentElement;
		const currentThemeAttr = root.getAttribute('data-theme') || 'dark';
		// Clear any custom inline styles first so we get the pure preset values.
		clearCustomThemeColors();
		root.setAttribute('data-theme', preset);
		// Force style recalculation.
		editorColors = getCurrentThemeColors();
		// Restore original theme attribute.
		root.setAttribute('data-theme', currentThemeAttr);
		// Apply the new editor colors as live preview.
		applyCustomThemeColors(editorColors);
	}

	function handleHexInput(key: keyof CustomThemeColors, value: string) {
		// Normalize the hex input: add # if missing, validate format.
		let hex = value.trim();
		if (hex && !hex.startsWith('#')) {
			hex = '#' + hex;
		}
		if (/^#[0-9a-fA-F]{6}$/.test(hex)) {
			handleEditorColorChange(key, hex);
		}
	}

	function handleEditorColorChange(key: keyof CustomThemeColors, value: string) {
		editorColors = { ...editorColors, [key]: value };
		// Live preview: apply the color immediately.
		document.documentElement.style.setProperty(`--${key}`, value);
	}

	function saveCustomThemeFromEditor() {
		editorError = '';
		editorSuccess = '';

		const name = editorThemeName.trim();
		if (!name) {
			editorError = 'Please enter a theme name.';
			return;
		}
		if (name.length > 50) {
			editorError = 'Theme name must be 50 characters or fewer.';
			return;
		}

		const newTheme: CustomTheme = {
			name,
			colors: { ...editorColors },
			createdAt: new Date().toISOString()
		};

		addCustomTheme(newTheme);
		activateCustomTheme(name);
		syncSettingsToApi();

		showThemeEditor = false;
		editingExistingTheme = false;
		editorSuccess = `Theme "${name}" saved and activated!`;
		setTimeout(() => (editorSuccess = ''), 3000);
	}

	function cancelThemeEditor() {
		showThemeEditor = false;
		editingExistingTheme = false;
		// Revert live preview: reapply the active custom theme or clear overrides.
		if ($activeCustomThemeName) {
			const active = $customThemes.find((t) => t.name === $activeCustomThemeName);
			if (active) {
				applyCustomThemeColors(active.colors);
			} else {
				clearCustomThemeColors();
			}
		} else {
			clearCustomThemeColors();
		}
	}

	function handleActivateTheme(name: string) {
		activateCustomTheme(name);
		syncSettingsToApi();
	}

	function handleDeactivateTheme() {
		deactivateCustomTheme();
		syncSettingsToApi();
	}

	function handleDeleteTheme(name: string) {
		deleteCustomTheme(name);
		syncSettingsToApi();
	}

	function handleExportTheme(themeObj: CustomTheme) {
		const json = exportTheme(themeObj);
		const blob = new Blob([json], { type: 'application/json' });
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = `${themeObj.name.replace(/[^a-zA-Z0-9_-]/g, '_')}.json`;
		a.click();
		URL.revokeObjectURL(url);
	}

	function openImportModal() {
		importJson = '';
		importError = '';
		showImportModal = true;
	}

	function handleImportTheme() {
		importError = '';
		try {
			const imported = importTheme(importJson);
			addCustomTheme(imported);
			syncSettingsToApi();
			showImportModal = false;
			editorSuccess = `Theme "${imported.name}" imported!`;
			setTimeout(() => (editorSuccess = ''), 3000);
		} catch (err: any) {
			importError = err.message || 'Failed to import theme.';
		}
	}

	function handleImportFile(e: Event) {
		const target = e.target as HTMLInputElement;
		const file = target.files?.[0];
		if (!file) return;
		if (!file.name.endsWith('.json') && file.type !== 'application/json') {
			importError = 'Please select a JSON file.';
			return;
		}
		importError = '';
		const reader = new FileReader();
		reader.onload = () => {
			const text = reader.result as string;
			try {
				const imported = importTheme(text);
				addCustomTheme(imported);
				syncSettingsToApi();
				showImportModal = false;
				editorSuccess = `Theme "${imported.name}" imported from file!`;
				setTimeout(() => (editorSuccess = ''), 3000);
			} catch (err: any) {
				importError = err.message || 'Failed to import theme from file.';
			}
		};
		reader.onerror = () => {
			importError = 'Failed to read file.';
		};
		reader.readAsText(file);
		target.value = '';
	}

	function triggerImportFilePicker() {
		importFileInput?.click();
	}

	function editExistingTheme(themeObj: CustomTheme) {
		editorColors = { ...themeObj.colors };
		editorThemeName = themeObj.name;
		editorBasePreset = '';
		editingExistingTheme = true;
		editorError = '';
		editorSuccess = '';
		showThemeEditor = true;
		// Apply live preview.
		applyCustomThemeColors(themeObj.colors);
	}

	function saveConnectedAccounts() {
		localStorage.setItem('av-connected-accounts', JSON.stringify(connectedAccounts));
		connectedAccountsSuccess = 'Connected accounts saved!';
		setTimeout(() => (connectedAccountsSuccess = ''), 3000);
	}

	// --- Custom CSS ---

	function handleSaveCustomCss() {
		saveCustomCss(customCssText);
		syncSettingsToApi();
		customCssSuccess = 'Custom CSS applied!';
		setTimeout(() => (customCssSuccess = ''), 3000);
	}

	function handleClearCustomCss() {
		customCssText = '';
		clearCustomCss();
		syncSettingsToApi();
		customCssSuccess = 'Custom CSS cleared.';
		setTimeout(() => (customCssSuccess = ''), 3000);
	}

	// --- Bots actions ---

	async function loadBots() {
		loadingBots = true;
		botError = '';
		try {
			myBots = await api.getMyBots();
		} catch (err: any) {
			botError = err.message || 'Failed to load bots';
			myBots = [];
		} finally {
			loadingBots = false;
		}
	}

	async function handleCreateBot() {
		if (!newBotName.trim()) return;
		creatingBot = true;
		botError = '';
		botSuccess = '';
		try {
			const bot = await api.createBot(newBotName.trim(), newBotDescription.trim() || undefined);
			myBots = [bot, ...myBots];
			newBotName = '';
			newBotDescription = '';
			botSuccess = `Bot "${bot.username}" created!`;
			setTimeout(() => (botSuccess = ''), 3000);
		} catch (err: any) {
			botError = err.message || 'Failed to create bot';
		} finally {
			creatingBot = false;
		}
	}

	async function handleDeleteBot(botId: string) {
		if (!confirm('Are you sure you want to delete this bot? This cannot be undone.')) return;
		try {
			await api.deleteBot(botId);
			myBots = myBots.filter(b => b.id !== botId);
			if (expandedBotId === botId) expandedBotId = null;
			botSuccess = 'Bot deleted.';
			setTimeout(() => (botSuccess = ''), 3000);
		} catch (err: any) {
			botError = err.message || 'Failed to delete bot';
		}
	}

	function startEditBot(bot: User) {
		editingBotId = bot.id;
		editBotName = bot.username;
		editBotDescription = bot.display_name ?? '';
	}

	function cancelEditBot() {
		editingBotId = null;
		editBotName = '';
		editBotDescription = '';
	}

	async function handleSaveBot() {
		if (!editingBotId || !editBotName.trim()) return;
		savingBot = true;
		botError = '';
		try {
			const updated = await api.updateBot(editingBotId, {
				name: editBotName.trim(),
				description: editBotDescription.trim() || undefined
			});
			myBots = myBots.map(b => b.id === updated.id ? updated : b);
			editingBotId = null;
			botSuccess = 'Bot updated.';
			setTimeout(() => (botSuccess = ''), 3000);
		} catch (err: any) {
			botError = err.message || 'Failed to update bot';
		} finally {
			savingBot = false;
		}
	}

	async function toggleBotExpand(botId: string) {
		if (expandedBotId === botId) {
			expandedBotId = null;
			return;
		}
		expandedBotId = botId;
		createdTokenRaw = null;
		// Load tokens and commands for this bot.
		if (!botTokens[botId]) {
			loadingBotTokens = botId;
			try {
				const tokens = await api.getBotTokens(botId);
				botTokens = { ...botTokens, [botId]: tokens };
			} catch { botTokens = { ...botTokens, [botId]: [] }; }
			finally { loadingBotTokens = null; }
		}
		if (!botCommands[botId]) {
			loadingBotCommands = botId;
			try {
				const cmds = await api.getBotCommands(botId);
				botCommands = { ...botCommands, [botId]: cmds };
			} catch { botCommands = { ...botCommands, [botId]: [] }; }
			finally { loadingBotCommands = null; }
		}
	}

	async function handleCreateToken(botId: string) {
		creatingToken = true;
		botError = '';
		try {
			const token = await api.createBotToken(botId, newTokenName.trim() || undefined);
			createdTokenRaw = token.token ?? null;
			botTokens = { ...botTokens, [botId]: [token, ...(botTokens[botId] ?? [])] };
			newTokenName = '';
		} catch (err: any) {
			botError = err.message || 'Failed to create token';
		} finally {
			creatingToken = false;
		}
	}

	async function handleDeleteToken(botId: string, tokenId: string) {
		try {
			await api.deleteBotToken(botId, tokenId);
			botTokens = { ...botTokens, [botId]: (botTokens[botId] ?? []).filter(t => t.id !== tokenId) };
		} catch (err: any) {
			botError = err.message || 'Failed to delete token';
		}
	}

	async function handleRegisterCommand(botId: string) {
		if (!newCommandName.trim() || !newCommandDescription.trim()) return;
		creatingCommand = true;
		botError = '';
		try {
			const cmd = await api.registerBotCommand(botId, {
				name: newCommandName.trim().toLowerCase(),
				description: newCommandDescription.trim()
			});
			botCommands = { ...botCommands, [botId]: [...(botCommands[botId] ?? []), cmd] };
			newCommandName = '';
			newCommandDescription = '';
			botSuccess = `Command "/${cmd.name}" registered.`;
			setTimeout(() => (botSuccess = ''), 3000);
		} catch (err: any) {
			botError = err.message || 'Failed to register command';
		} finally {
			creatingCommand = false;
		}
	}

	async function handleDeleteCommand(botId: string, commandId: string) {
		try {
			await api.deleteBotCommand(botId, commandId);
			botCommands = { ...botCommands, [botId]: (botCommands[botId] ?? []).filter(c => c.id !== commandId) };
		} catch (err: any) {
			botError = err.message || 'Failed to delete command';
		}
	}

	function copyToClipboard(text: string) {
		navigator.clipboard.writeText(text).catch(() => {});
	}

	// Load bots when switching to the bots tab.
	$effect(() => {
		if (currentTab === 'bots') {
			loadBots();
		}
	});

	// Load voice preferences when switching to the voice tab.
	$effect(() => {
		if (currentTab === 'voice') {
			loadVoicePreferences();
		}
	});

	// PTT key recording for voice settings page.
	$effect(() => {
		if (recordingVoicePTTKey && voicePrefs) {
			const handleKeyDown = (e: KeyboardEvent) => {
				e.preventDefault();
				if (voicePrefs) {
					voicePrefs.ptt_key = e.code;
				}
				recordingVoicePTTKey = false;
			};
			window.addEventListener('keydown', handleKeyDown);
			return () => window.removeEventListener('keydown', handleKeyDown);
		}
	});

	// --- Data Export/Import ---

	async function handleExportData() {
		exportingData = true;
		exportDataSuccess = '';
		exportDataError = '';
		try {
			const token = api.getToken();
			const res = await fetch('/api/v1/users/@me/export', {
				headers: { Authorization: `Bearer ${token}` }
			});
			if (!res.ok) {
				const body = await res.json();
				throw new Error(body?.error?.message || 'Export failed');
			}
			const data = await res.json();
			const exportData = data.data ?? data;
			const blob = new Blob([JSON.stringify(exportData, null, 2)], { type: 'application/json' });
			const url = URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = `amityvox-data-export-${new Date().toISOString().slice(0, 10)}.json`;
			a.click();
			URL.revokeObjectURL(url);
			exportDataSuccess = 'Data exported successfully! Check your downloads.';
			setTimeout(() => (exportDataSuccess = ''), 5000);
		} catch (err: any) {
			exportDataError = err.message || 'Failed to export data';
		} finally {
			exportingData = false;
		}
	}

	async function handleExportAccount() {
		exportingAccount = true;
		exportAccountSuccess = '';
		exportAccountError = '';
		try {
			const token = api.getToken();
			const res = await fetch('/api/v1/users/@me/export-account', {
				headers: { Authorization: `Bearer ${token}` }
			});
			if (!res.ok) {
				const body = await res.json();
				throw new Error(body?.error?.message || 'Export failed');
			}
			const data = await res.json();
			const exportData = data.data ?? data;
			const blob = new Blob([JSON.stringify(exportData, null, 2)], { type: 'application/json' });
			const url = URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = `amityvox-account-export-${new Date().toISOString().slice(0, 10)}.json`;
			a.click();
			URL.revokeObjectURL(url);
			exportAccountSuccess = 'Account exported successfully! Check your downloads.';
			setTimeout(() => (exportAccountSuccess = ''), 5000);
		} catch (err: any) {
			exportAccountError = err.message || 'Failed to export account';
		} finally {
			exportingAccount = false;
		}
	}

	async function handleImportAccount() {
		if (!accountImportFileInput?.files?.length) {
			importAccountError = 'Please select a file to import.';
			return;
		}
		const file = accountImportFileInput.files[0];
		if (file.size > 5 * 1024 * 1024) {
			importAccountError = 'File too large (max 5 MB).';
			return;
		}

		importingAccount = true;
		importAccountSuccess = '';
		importAccountError = '';
		try {
			const text = await file.text();
			const imported = JSON.parse(text);

			// Extract profile and settings from the exported format.
			const payload: Record<string, unknown> = {};
			if (imported.profile) {
				payload.profile = imported.profile;
			}
			if (imported.settings) {
				payload.settings = imported.settings;
			}

			const token = api.getToken();
			const res = await fetch('/api/v1/users/@me/import-account', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
					Authorization: `Bearer ${token}`
				},
				body: JSON.stringify(payload)
			});
			if (!res.ok) {
				const body = await res.json();
				throw new Error(body?.error?.message || 'Import failed');
			}
			const data = await res.json();
			const user = data.data ?? data;
			currentUser.set(user);
			displayName = user.display_name ?? '';
			bio = user.bio ?? '';
			statusText = user.status_text ?? '';
			accentColor = user.accent_color ?? '#5865f2';
			importAccountSuccess = 'Account data imported successfully! Profile updated.';
			setTimeout(() => (importAccountSuccess = ''), 5000);
		} catch (err: any) {
			if (err instanceof SyntaxError) {
				importAccountError = 'Invalid JSON file. Please select a valid AmityVox account export.';
			} else {
				importAccountError = err.message || 'Failed to import account';
			}
		} finally {
			importingAccount = false;
			if (accountImportFileInput) accountImportFileInput.value = '';
		}
	}

	// --- Voice & Video actions ---

	async function loadVoicePreferences() {
		voiceLoading = true;
		voiceError = '';
		try {
			voicePrefs = await api.getVoicePreferences();
			// Load device IDs from localStorage (browser-specific).
			inputDeviceId = localStorage.getItem('av-voice-input-device') ?? '';
			outputDeviceId = localStorage.getItem('av-voice-output-device') ?? '';
			// Enumerate available devices.
			if (navigator.mediaDevices?.enumerateDevices) {
				const devices = await navigator.mediaDevices.enumerateDevices();
				availableInputDevices = devices.filter(d => d.kind === 'audioinput');
				availableOutputDevices = devices.filter(d => d.kind === 'audiooutput');
			}
		} catch (err: any) {
			voiceError = err.message || 'Failed to load voice preferences';
		} finally {
			voiceLoading = false;
		}
	}

	async function saveVoicePreferences() {
		if (!voicePrefs) return;
		voiceSaving = true;
		voiceError = '';
		voiceSuccess = '';
		try {
			voicePrefs = await api.updateVoicePreferences({
				input_mode: voicePrefs.input_mode,
				ptt_key: voicePrefs.ptt_key,
				vad_threshold: voicePrefs.vad_threshold,
				noise_suppression: voicePrefs.noise_suppression,
				echo_cancellation: voicePrefs.echo_cancellation,
				auto_gain_control: voicePrefs.auto_gain_control,
				input_volume: voicePrefs.input_volume,
				output_volume: voicePrefs.output_volume,
				camera_resolution: voicePrefs.camera_resolution,
				camera_framerate: voicePrefs.camera_framerate,
				screenshare_resolution: voicePrefs.screenshare_resolution,
				screenshare_framerate: voicePrefs.screenshare_framerate,
				screenshare_audio: voicePrefs.screenshare_audio
			});
			// Save device IDs to localStorage.
			localStorage.setItem('av-voice-input-device', inputDeviceId);
			localStorage.setItem('av-voice-output-device', outputDeviceId);
			voiceSuccess = 'Voice preferences saved!';
			setTimeout(() => (voiceSuccess = ''), 3000);
		} catch (err: any) {
			voiceError = err.message || 'Failed to save voice preferences';
		} finally {
			voiceSaving = false;
		}
	}

	function formatVoiceKeyName(code: string): string {
		return code
			.replace('Key', '')
			.replace('Digit', '')
			.replace('Arrow', '')
			.replace('Numpad', 'Num ')
			.replace('Control', 'Ctrl')
			.replace('Semicolon', ';')
			.replace('Quote', "'")
			.replace('BracketLeft', '[')
			.replace('BracketRight', ']')
			.replace('Backslash', '\\')
			.replace('Slash', '/')
			.replace('Period', '.')
			.replace('Comma', ',')
			.replace('Minus', '-')
			.replace('Equal', '=')
			.replace('Backquote', '`');
	}

	async function handleLogout() {
		await logout();
		goto('/login');
	}

	const tabs: { id: Tab; label: string }[] = [
		{ id: 'account', label: 'My Account' },
		{ id: 'security', label: 'Security' },
		{ id: 'notifications', label: 'Notifications' },
		{ id: 'privacy', label: 'Privacy' },
		{ id: 'appearance', label: 'Appearance' },
		{ id: 'voice', label: 'Voice & Video' },
		{ id: 'encryption', label: 'Encryption' },
		{ id: 'bots', label: 'Bots' },
		{ id: 'data', label: 'Data & Privacy' }
	];

	const themeOptions: { id: ThemeName; label: string; preview: string }[] = [
		{ id: 'dark', label: 'Dark', preview: '#1e1f22' },
		{ id: 'light', label: 'Light', preview: '#ffffff' },
		{ id: 'amoled', label: 'AMOLED', preview: '#000000' },
		{ id: 'nord', label: 'Nord', preview: '#2e3440' },
		{ id: 'dracula', label: 'Dracula', preview: '#282a36' },
		{ id: 'catppuccin', label: 'Catppuccin', preview: '#1e1e2e' },
		{ id: 'solarized', label: 'Solarized', preview: '#002b36' },
		{ id: 'high-contrast', label: 'High Contrast', preview: '#000000' }
	];

	function themeButtonClass(t: ThemeName): string {
		const base = 'rounded-lg border-2 px-4 py-3 text-sm transition-colors flex items-center gap-3';
		if (theme === t) return `${base} border-brand-500 bg-brand-500/10 text-text-primary`;
		return `${base} border-bg-modifier text-text-muted hover:border-bg-tertiary`;
	}

	function formatSessionTime(iso: string): string {
		return new Date(iso).toLocaleString();
	}

	function parseUserAgent(ua: string): string {
		if (ua.includes('Firefox')) return 'Firefox';
		if (ua.includes('Chrome')) return 'Chrome';
		if (ua.includes('Safari')) return 'Safari';
		if (ua.includes('Edge')) return 'Edge';
		return 'Unknown Browser';
	}

	// Generate hour options for the time selectors.
	const hourOptions = Array.from({ length: 24 }, (_, i) => i);
	const minuteOptions = [0, 15, 30, 45];
</script>

<svelte:head>
	<title>Settings — AmityVox</title>
</svelte:head>

<div class="flex h-full">
	<!-- Settings sidebar -->
	<nav class="w-52 shrink-0 overflow-y-auto bg-bg-secondary p-4">
		<h3 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">User Settings</h3>
		<ul class="space-y-0.5">
			{#each tabs as tab (tab.id)}
				<li>
					<button
						class="w-full rounded px-2 py-1.5 text-left text-sm transition-colors {currentTab === tab.id ? 'bg-bg-modifier text-text-primary' : 'text-text-muted hover:bg-bg-modifier hover:text-text-secondary'}"
						onclick={() => (currentTab = tab.id)}
					>
						{tab.label}
						{#if tab.id === 'notifications' && $isDndActive}
							<span class="ml-1 inline-block h-2 w-2 rounded-full bg-status-dnd" title="DND active"></span>
						{/if}
					</button>
				</li>
			{/each}
		</ul>

		<div class="my-2 border-t border-bg-modifier"></div>
		<button
			class="w-full rounded px-2 py-1.5 text-left text-sm text-text-muted hover:bg-bg-modifier hover:text-text-secondary"
			onclick={() => goto('/app')}
		>
			Back
		</button>

		<div class="my-2 border-t border-bg-modifier"></div>
		<button
			class="w-full rounded px-2 py-1.5 text-left text-sm text-red-400 hover:bg-bg-modifier"
			onclick={handleLogout}
		>
			Log Out
		</button>
	</nav>

	<!-- Settings content -->
	<div class="flex-1 overflow-y-auto bg-bg-tertiary p-8">
		<div class="max-w-xl">
			{#if error}
				<div class="mb-4 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{error}</div>
			{/if}
			{#if success}
				<div class="mb-4 rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{success}</div>
			{/if}

			<!-- ==================== MY ACCOUNT ==================== -->
			{#if currentTab === 'account'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">My Account</h1>

				{#if $currentUser}
					<!-- Profile card with banner -->
					<div class="mb-8 overflow-hidden rounded-lg bg-bg-secondary">
						<!-- Banner area -->
						<div class="group relative h-28">
							{#if bannerPreview}
								<img class="h-full w-full object-cover" src={bannerPreview} alt="Banner preview" />
							{:else if $currentUser.banner_id}
								<img class="h-full w-full object-cover" src="/api/v1/files/{$currentUser.banner_id}" alt="Profile banner" />
							{:else}
								<div class="h-full w-full" style="background-color: {accentColor}"></div>
							{/if}
							<label class="absolute inset-0 flex cursor-pointer items-center justify-center bg-black/40 opacity-0 transition-opacity group-hover:opacity-100">
								<div class="flex flex-col items-center gap-1 text-white">
									<svg class="h-6 w-6" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
										<path d="M3 9a2 2 0 012-2h.93a2 2 0 001.664-.89l.812-1.22A2 2 0 0110.07 4h3.86a2 2 0 011.664.89l.812 1.22A2 2 0 0018.07 7H19a2 2 0 012 2v9a2 2 0 01-2 2H5a2 2 0 01-2-2V9z" />
										<circle cx="12" cy="13" r="3" />
									</svg>
									<span class="text-xs font-medium">Change Banner</span>
								</div>
								<input type="file" accept="image/*" class="hidden" onchange={handleBannerSelect} />
							</label>
						</div>

						<!-- Avatar + info, overlapping banner -->
						<div class="relative px-6 pb-4">
							<div class="flex items-end gap-4">
								<div class="relative -mt-10">
									<div class="rounded-xl bg-bg-secondary p-1">
										<Avatar
											name={$currentUser.display_name ?? $currentUser.username}
											src={avatarPreview ?? ($currentUser.avatar_id ? `/api/v1/files/${$currentUser.avatar_id}` : null)}
											size="lg"
											status={$currentUser.status_presence}
										/>
									</div>
									<label class="absolute inset-1 flex cursor-pointer items-center justify-center rounded-md bg-black/50 opacity-0 transition-opacity hover:opacity-100">
										<svg class="h-5 w-5 text-white" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
											<path d="M3 9a2 2 0 012-2h.93a2 2 0 001.664-.89l.812-1.22A2 2 0 0110.07 4h3.86a2 2 0 011.664.89l.812 1.22A2 2 0 0018.07 7H19a2 2 0 012 2v9a2 2 0 01-2 2H5a2 2 0 01-2-2V9z" />
											<circle cx="12" cy="13" r="3" />
										</svg>
										<input type="file" accept="image/*" class="hidden" onchange={handleAvatarSelect} />
									</label>
								</div>
								<div class="mb-1">
									<h2 class="text-lg font-semibold text-text-primary">
										{$currentUser.display_name ?? $currentUser.username}
									</h2>
									<p class="text-sm text-text-muted">{$currentUser.username}</p>
									{#if $currentUser.email}
										<p class="text-sm text-text-muted">{$currentUser.email}</p>
									{/if}
								</div>
							</div>
						</div>
					</div>

					<!-- Banner color -->
					<div class="mb-4">
						<label class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
							Profile Color
						</label>
						<p class="mb-2 text-xs text-text-muted">Used as your profile banner background when no banner image is set.</p>
						<div class="flex items-center gap-3">
							<input type="color" class="h-9 w-9 cursor-pointer rounded border border-border-primary bg-bg-secondary" bind:value={accentColor} />
							<input type="text" class="input w-28 font-mono text-xs" bind:value={accentColor} maxlength="7" />
							{#if $currentUser.banner_id || bannerPreview}
								<button
									class="text-xs text-red-400 hover:text-red-300"
									onclick={() => { bannerFile = null; bannerPreview = null; bannerRemoved = true; }}
									title="Remove banner image to use color instead"
								>
									Remove Banner
								</button>
							{/if}
						</div>
					</div>

					<div class="mb-4">
						<label for="displayName" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
							Display Name
						</label>
						<input id="displayName" type="text" bind:value={displayName} class="input w-full" maxlength="32" />
					</div>

					<div class="mb-4">
						<label for="statusText" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
							Custom Status
						</label>
						<input id="statusText" type="text" bind:value={statusText} class="input w-full" maxlength="128" />
					</div>

					<div class="mb-6">
						<label for="bio" class="mb-2 block text-xs font-bold uppercase tracking-wide text-text-muted">
							About Me
						</label>
						<textarea id="bio" bind:value={bio} class="input w-full" rows="3" maxlength="190"></textarea>
					</div>

					<button class="btn-primary" onclick={handleSaveProfile} disabled={saving}>
						{saving ? 'Saving...' : 'Save Changes'}
					</button>

					<!-- Profile Links -->
					<div class="mt-8 rounded-lg bg-bg-secondary p-6">
						<ProfileLinkEditor />
					</div>
				{/if}

			<!-- ==================== SECURITY ==================== -->
			{:else if currentTab === 'security'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Security</h1>

				<!-- Password Change -->
				<div class="mb-8 rounded-lg bg-bg-secondary p-6">
					<h3 class="mb-4 text-sm font-semibold text-text-primary">Change Password</h3>

					{#if passwordError}
						<div class="mb-3 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{passwordError}</div>
					{/if}
					{#if passwordSuccess}
						<div class="mb-3 rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{passwordSuccess}</div>
					{/if}

					<div class="mb-3">
						<label for="curPw" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
							Current Password
						</label>
						<input id="curPw" type="password" bind:value={currentPassword} class="input w-full" />
					</div>
					<div class="mb-3">
						<label for="newPw" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
							New Password
						</label>
						<input id="newPw" type="password" bind:value={newPassword} class="input w-full" />
					</div>
					<div class="mb-4">
						<label for="confirmPw" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
							Confirm New Password
						</label>
						<input id="confirmPw" type="password" bind:value={confirmPassword} class="input w-full" />
					</div>
					<button
						class="btn-primary"
						onclick={handleChangePassword}
						disabled={changingPassword || !currentPassword || !newPassword || !confirmPassword}
					>
						{changingPassword ? 'Changing...' : 'Change Password'}
					</button>
				</div>

				<!-- Two-Factor Authentication -->
				<div class="mb-8 rounded-lg bg-bg-secondary p-6">
					<h3 class="mb-2 text-sm font-semibold text-text-primary">Two-Factor Authentication</h3>
					<p class="mb-4 text-xs text-text-muted">
						Add an extra layer of security with a TOTP authenticator app.
					</p>

					{#if totpError}
						<div class="mb-3 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{totpError}</div>
					{/if}

					{#if totpStep === 'idle'}
						<button class="btn-primary" onclick={handleEnableTotp} disabled={enablingTotp}>
							{enablingTotp ? 'Setting up...' : 'Enable 2FA'}
						</button>
					{:else if totpStep === 'setup'}
						<div class="space-y-4">
							<p class="text-sm text-text-secondary">
								Scan this QR code with your authenticator app (Google Authenticator, Authy, etc.):
							</p>
							{#if totpQrUrl}
								<div class="flex justify-center rounded bg-white p-4">
									<img src={totpQrUrl} alt="2FA QR Code" class="h-48 w-48" />
								</div>
							{/if}
							<div class="rounded bg-bg-primary p-3">
								<p class="mb-1 text-xs font-bold uppercase tracking-wide text-text-muted">Manual entry code:</p>
								<code class="break-all text-sm text-text-primary">{totpSecret}</code>
							</div>
							<div>
								<label for="totpCode" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
									Enter 6-digit verification code
								</label>
								<input
									id="totpCode"
									type="text"
									inputmode="numeric"
									maxlength="6"
									bind:value={totpCode}
									class="input w-40"
									placeholder="000000"
									onkeydown={(e) => e.key === 'Enter' && handleVerifyTotp()}
								/>
							</div>
							<div class="flex gap-2">
								<button class="btn-primary" onclick={handleVerifyTotp} disabled={verifyingTotp || totpCode.length !== 6}>
									{verifyingTotp ? 'Verifying...' : 'Verify & Enable'}
								</button>
								<button class="btn-secondary" onclick={resetTotpFlow}>Cancel</button>
							</div>
						</div>
					{:else if totpStep === 'done'}
						<div class="space-y-4">
							<div class="rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">
								Two-factor authentication has been enabled!
							</div>
							{#if backupCodes.length > 0}
								<div class="rounded bg-bg-primary p-4">
									<p class="mb-2 text-sm font-semibold text-text-primary">Backup Codes</p>
									<p class="mb-3 text-xs text-text-muted">
										Save these codes in a safe place. Each can be used once if you lose access to your authenticator.
									</p>
									<div class="grid grid-cols-2 gap-2">
										{#each backupCodes as code}
											<code class="rounded bg-bg-secondary px-2 py-1 text-center text-sm text-text-primary">{code}</code>
										{/each}
									</div>
								</div>
							{/if}
							<button class="btn-secondary" onclick={resetTotpFlow}>Done</button>
						</div>
					{/if}
				</div>

				<!-- Active Sessions -->
				<div class="rounded-lg bg-bg-secondary p-6">
					<h3 class="mb-4 text-sm font-semibold text-text-primary">Active Sessions</h3>

					{#if loadingSessions}
						<div class="flex items-center gap-2 py-4">
							<div class="h-4 w-4 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
							<span class="text-sm text-text-muted">Loading sessions...</span>
						</div>
					{:else if sessions.length === 0}
						<p class="text-sm text-text-muted">No sessions found.</p>
					{:else}
						<div class="space-y-3">
							{#each sessions as session (session.id)}
								<div class="flex items-center justify-between rounded bg-bg-primary p-3">
									<div>
										<div class="flex items-center gap-2">
											<span class="text-sm font-medium text-text-primary">
												{parseUserAgent(session.user_agent)}
											</span>
											{#if session.current}
												<span class="rounded bg-green-500/10 px-1.5 py-0.5 text-2xs font-bold text-green-400">Current</span>
											{/if}
										</div>
										<p class="text-xs text-text-muted">
											{session.ip_address} &middot; Last active {formatSessionTime(session.last_active_at)}
										</p>
									</div>
									{#if !session.current}
										<button
											class="text-xs text-red-400 hover:text-red-300"
											onclick={() => revokeSession(session.id)}
											disabled={revokingSession === session.id}
										>
											{revokingSession === session.id ? 'Revoking...' : 'Revoke'}
										</button>
									{/if}
								</div>
							{/each}
						</div>
					{/if}
				</div>

			<!-- ==================== NOTIFICATIONS ==================== -->
			{:else if currentTab === 'notifications'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Notifications</h1>

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

					<button class="btn-primary" onclick={saveNotifications} disabled={notifLoading}>
						{notifLoading ? 'Saving...' : 'Save Notification Preferences'}
					</button>

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

							<button class="btn-primary mt-4" onclick={saveDnd} disabled={dndSaving}>
								{dndSaving ? 'Saving...' : 'Save DND Schedule'}
							</button>
						</div>
					</div>

					<!-- ==================== MUTED ITEMS ==================== -->
					<div class="border-t border-bg-modifier pt-6">
						<h2 class="mb-4 text-lg font-bold text-text-primary">Muted Channels & Guilds</h2>

						{#if getMutedChannels().length === 0 && getMutedGuilds().length === 0}
							<p class="text-sm text-text-muted">No muted channels or guilds.</p>
						{:else}
							<div class="space-y-2">
								{#each getMutedGuilds() as guildPref}
									{@const guild = $guildsStore.get(guildPref.guild_id ?? '')}
									<div class="flex items-center justify-between rounded-lg bg-bg-secondary p-3">
										<div class="min-w-0 flex-1">
											<p class="truncate text-sm font-medium text-text-primary">
												{guild?.name ?? guildPref.guild_id ?? 'Unknown Guild'}
											</p>
											<p class="text-xs text-text-muted">
												Guild
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

			<!-- ==================== PRIVACY ==================== -->
			{:else if currentTab === 'privacy'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Privacy</h1>

				{#if privacySuccess}
					<div class="mb-4 rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{privacySuccess}</div>
				{/if}

				<div class="space-y-6">
					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">Direct Messages</h3>
						<p class="mb-3 text-xs text-text-muted">Control who can send you direct messages.</p>
						<div class="space-y-2">
							<label class="flex items-center gap-2">
								<input type="radio" name="dmPrivacy" value="everyone" bind:group={dmPrivacy} class="accent-brand-500" />
								<span class="text-sm text-text-secondary">Everyone</span>
							</label>
							<label class="flex items-center gap-2">
								<input type="radio" name="dmPrivacy" value="friends" bind:group={dmPrivacy} class="accent-brand-500" />
								<span class="text-sm text-text-secondary">Friends only</span>
							</label>
							<label class="flex items-center gap-2">
								<input type="radio" name="dmPrivacy" value="nobody" bind:group={dmPrivacy} class="accent-brand-500" />
								<span class="text-sm text-text-secondary">Nobody</span>
							</label>
						</div>
					</div>

					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">Friend Requests</h3>
						<p class="mb-3 text-xs text-text-muted">Control who can send you friend requests.</p>
						<div class="space-y-2">
							<label class="flex items-center gap-2">
								<input type="radio" name="friendPrivacy" value="everyone" bind:group={friendRequestPrivacy} class="accent-brand-500" />
								<span class="text-sm text-text-secondary">Everyone</span>
							</label>
							<label class="flex items-center gap-2">
								<input type="radio" name="friendPrivacy" value="mutual_guilds" bind:group={friendRequestPrivacy} class="accent-brand-500" />
								<span class="text-sm text-text-secondary">People in mutual guilds</span>
							</label>
							<label class="flex items-center gap-2">
								<input type="radio" name="friendPrivacy" value="nobody" bind:group={friendRequestPrivacy} class="accent-brand-500" />
								<span class="text-sm text-text-secondary">Nobody</span>
							</label>
						</div>
					</div>

					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">NSFW Content Filter</h3>
						<p class="mb-3 text-xs text-text-muted">Control how images are displayed in NSFW-marked channels.</p>
						<div class="space-y-2">
							<label class="flex items-center gap-2">
								<input type="radio" name="nsfwFilter" value="blur_all" bind:group={nsfwContentFilter} class="accent-brand-500" />
								<span class="text-sm text-text-secondary">Blur all media in NSFW channels</span>
							</label>
							<label class="flex items-center gap-2">
								<input type="radio" name="nsfwFilter" value="show_all" bind:group={nsfwContentFilter} class="accent-brand-500" />
								<span class="text-sm text-text-secondary">Show all media</span>
							</label>
						</div>
					</div>

					<button class="btn-primary" onclick={savePrivacy} disabled={privacyLoading}>
						{privacyLoading ? 'Saving...' : 'Save Privacy Settings'}
					</button>
				</div>

			<!-- ==================== APPEARANCE ==================== -->
			{:else if currentTab === 'appearance'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Appearance</h1>

				<div class="space-y-6">
					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">Theme</h3>
						<p class="mb-3 text-xs text-text-muted">Choose your interface theme. Changes apply when saved.</p>
						<div class="grid grid-cols-2 gap-2">
							{#each themeOptions as opt (opt.id)}
								<button class={themeButtonClass(opt.id)} onclick={() => (theme = opt.id)}>
									<div class="h-5 w-5 shrink-0 rounded-full border border-bg-modifier" style="background-color: {opt.preview}"></div>
									{opt.label}
								</button>
							{/each}
						</div>
					</div>

					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">Font Size</h3>
						<p class="mb-3 text-xs text-text-muted">Adjust the base font size ({fontSize}px).</p>
						<input
							type="range"
							min="12"
							max="20"
							bind:value={fontSize}
							class="w-full accent-brand-500"
						/>
						<div class="mt-1 flex justify-between text-xs text-text-muted">
							<span>12px</span>
							<span>16px</span>
							<span>20px</span>
						</div>
						<div class="mt-3 rounded-md border border-bg-modifier bg-bg-primary p-3">
							<p class="mb-1 text-2xs font-bold uppercase tracking-wide text-text-muted">Preview</p>
							<p style="font-size: {fontSize}px; line-height: 1.5" class="text-text-primary">
								The quick brown fox jumps over the lazy dog.
							</p>
							<p style="font-size: {Math.round(fontSize * 0.85)}px; line-height: 1.4" class="mt-1 text-text-muted">
								This is how smaller text like timestamps will look.
							</p>
						</div>
					</div>

					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">Compact Mode</h3>
						<label class="flex items-center gap-2">
							<input type="checkbox" bind:checked={compactMode} class="accent-brand-500" />
							<span class="text-sm text-text-secondary">Use compact message layout</span>
						</label>
					</div>

					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">Reduced Motion</h3>
						<p class="mb-2 text-xs text-text-muted">Disable animations and transitions for accessibility.</p>
						<label class="flex items-center gap-2">
							<input type="checkbox" bind:checked={reducedMotion} class="accent-brand-500" />
							<span class="text-sm text-text-secondary">Reduce motion and animations</span>
						</label>
					</div>

					<div class="rounded-lg bg-bg-secondary p-4">
						<h3 class="mb-1 text-sm font-semibold text-text-primary">Dyslexia-Friendly Font</h3>
						<p class="mb-2 text-xs text-text-muted">Use OpenDyslexic font for improved readability.</p>
						<label class="flex items-center gap-2">
							<input type="checkbox" bind:checked={dyslexicFont} class="accent-brand-500" />
							<span class="text-sm text-text-secondary">Enable dyslexia-friendly font</span>
						</label>
					</div>

					<button class="btn-primary" onclick={saveAppearance}>Save Appearance</button>

					<!-- ==================== THEME EDITOR ==================== -->
					<div class="border-t border-bg-modifier pt-6">
						<div class="flex items-center justify-between mb-4">
							<div>
								<h2 class="text-lg font-bold text-text-primary">Custom Themes</h2>
								<p class="text-xs text-text-muted">Create, edit, import and export custom color themes.</p>
							</div>
							<div class="flex gap-2">
								<button
									class="btn-secondary text-xs"
									onclick={() => {
										const colors = getCurrentThemeColors();
										const exported = exportTheme({ name: theme + '-exported', colors, createdAt: new Date().toISOString() });
										const blob = new Blob([exported], { type: 'application/json' });
										const url = URL.createObjectURL(blob);
										const a = document.createElement('a');
										a.href = url;
										a.download = `${theme}-theme.json`;
										a.click();
										URL.revokeObjectURL(url);
									}}
									title="Export the current active theme colors as a JSON file"
								>
									Export Current
								</button>
								<button class="btn-secondary text-xs" onclick={openImportModal}>
									Import
								</button>
								<button class="btn-primary text-xs" onclick={openThemeEditor}>
									Create Theme
								</button>
							</div>
						</div>

						{#if editorSuccess}
							<div class="mb-4 rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{editorSuccess}</div>
						{/if}

						<!-- Active custom theme indicator -->
						{#if $activeCustomThemeName}
							<div class="mb-4 flex items-center gap-2 rounded-lg bg-brand-500/10 p-3">
								<svg class="h-4 w-4 shrink-0 text-brand-400" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
									<path d="M5 13l4 4L19 7" />
								</svg>
								<span class="text-sm text-text-primary">
									Active theme: <strong>{$activeCustomThemeName}</strong>
								</span>
								<button
									class="ml-auto text-xs text-text-muted hover:text-text-primary"
									onclick={handleDeactivateTheme}
								>
									Deactivate
								</button>
							</div>
						{/if}

						<!-- Saved custom themes list -->
						{#if $customThemes.length > 0}
							<div class="space-y-2 mb-4">
								{#each $customThemes as themeObj (themeObj.name)}
									<div class="flex items-center gap-3 rounded-lg bg-bg-secondary p-3">
										<!-- Color preview swatches -->
										<div class="flex -space-x-1">
											<div class="h-6 w-6 rounded-full border-2 border-bg-primary" style="background-color: {themeObj.colors['bg-primary']}"></div>
											<div class="h-6 w-6 rounded-full border-2 border-bg-primary" style="background-color: {themeObj.colors['brand-500']}"></div>
											<div class="h-6 w-6 rounded-full border-2 border-bg-primary" style="background-color: {themeObj.colors['text-primary']}"></div>
										</div>

										<div class="flex-1 min-w-0">
											<p class="text-sm font-medium text-text-primary truncate">{themeObj.name}</p>
											<p class="text-2xs text-text-muted">
												{new Date(themeObj.createdAt).toLocaleDateString()}
											</p>
										</div>

										<div class="flex items-center gap-1">
											{#if $activeCustomThemeName === themeObj.name}
												<span class="rounded bg-brand-500/15 px-2 py-0.5 text-2xs font-bold text-brand-400">Active</span>
											{:else}
												<button
													class="rounded px-2 py-1 text-xs text-text-muted hover:bg-bg-modifier hover:text-text-primary"
													onclick={() => handleActivateTheme(themeObj.name)}
												>
													Activate
												</button>
											{/if}
											<button
												class="rounded p-1 text-text-muted hover:bg-bg-modifier hover:text-text-primary"
												onclick={() => editExistingTheme(themeObj)}
												title="Edit theme"
											>
												<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
													<path d="M11 4H4a2 2 0 00-2 2v14a2 2 0 002 2h14a2 2 0 002-2v-7" />
													<path d="M18.5 2.5a2.121 2.121 0 013 3L12 15l-4 1 1-4 9.5-9.5z" />
												</svg>
											</button>
											<button
												class="rounded p-1 text-text-muted hover:bg-bg-modifier hover:text-text-primary"
												onclick={() => handleExportTheme(themeObj)}
												title="Export theme"
											>
												<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
													<path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4" />
													<polyline points="7 10 12 15 17 10" />
													<line x1="12" y1="15" x2="12" y2="3" />
												</svg>
											</button>
											<button
												class="rounded p-1 text-text-muted hover:bg-bg-modifier hover:text-red-400"
												onclick={() => handleDeleteTheme(themeObj.name)}
												title="Delete theme"
											>
												<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
													<polyline points="3 6 5 6 21 6" />
													<path d="M19 6v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6m3 0V4a2 2 0 012-2h4a2 2 0 012 2v2" />
												</svg>
											</button>
										</div>
									</div>
								{/each}
							</div>
						{:else}
							<div class="mb-4 rounded-lg bg-bg-secondary p-6 text-center">
								<svg class="mx-auto mb-2 h-10 w-10 text-text-muted/30" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
									<path d="M7 21a4 4 0 01-4-4V5a2 2 0 012-2h4a2 2 0 012 2v12a4 4 0 01-4 4zm0 0h12a2 2 0 002-2v-4a2 2 0 00-2-2h-2.343M11 7.343l1.657-1.657a2 2 0 012.828 0l2.829 2.829a2 2 0 010 2.828l-8.486 8.485M7 17h.01" />
								</svg>
								<p class="text-sm text-text-muted">No custom themes yet.</p>
								<p class="text-xs text-text-muted">Click "Create Theme" to design your own color scheme.</p>
							</div>
						{/if}

						<!-- Theme Editor Panel -->
						{#if showThemeEditor}
							<div class="rounded-lg border-2 border-brand-500/30 bg-bg-secondary p-4">
								<div class="flex items-center justify-between mb-4">
									<h3 class="text-sm font-semibold text-text-primary">
										{editingExistingTheme ? 'Edit Theme' : 'Create Theme'}
									</h3>
									<button
										class="rounded p-1 text-text-muted hover:bg-bg-modifier hover:text-text-primary"
										onclick={cancelThemeEditor}
										title="Close editor"
									>
										<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
											<path d="M6 18L18 6M6 6l12 12" />
										</svg>
									</button>
								</div>

								{#if editorError}
									<div class="mb-3 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{editorError}</div>
								{/if}

								<!-- Theme name -->
								<div class="mb-4">
									<label for="themeName" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
										Theme Name
									</label>
									<input
										id="themeName"
										type="text"
										bind:value={editorThemeName}
										class="input w-full"
										placeholder="My Custom Theme"
										maxlength="50"
									/>
								</div>

								<!-- Start from preset -->
								{#if !editingExistingTheme}
									<div class="mb-4">
										<label for="basePreset" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
											Start From Preset
										</label>
										<select
											id="basePreset"
											class="input w-full"
											value={editorBasePreset}
											onchange={(e) => handleBasePresetChange((e.target as HTMLSelectElement).value as ThemeName)}
										>
											{#each themeOptions as opt (opt.id)}
												<option value={opt.id}>{opt.label}</option>
											{/each}
										</select>
										<p class="mt-1 text-2xs text-text-muted">
											Pick a built-in theme to use as a starting point. You can change any color below.
										</p>
									</div>
								{/if}

								<!-- Color pickers grouped by category -->
								{#each THEME_COLOR_GROUPS as group}
									<div class="mb-4">
										<h4 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">{group.label}</h4>
										<div class="grid grid-cols-1 gap-2">
											{#each group.keys as key}
												<div class="flex items-center gap-2 rounded bg-bg-primary p-2">
													<!-- Color swatch preview -->
													<div
														class="h-8 w-8 shrink-0 rounded border border-bg-modifier"
														style="background-color: {editorColors[key]}"
													></div>
													<!-- Label -->
													<div class="flex-1 min-w-0">
														<p class="text-xs font-medium text-text-primary">{THEME_COLOR_LABELS[key]}</p>
													</div>
													<!-- Hex input field -->
													<input
														type="text"
														value={editorColors[key]}
														oninput={(e) => handleHexInput(key, (e.target as HTMLInputElement).value)}
														class="w-[5.5rem] shrink-0 rounded bg-bg-floating px-2 py-1 font-mono text-xs text-text-primary outline-none ring-1 ring-bg-modifier focus:ring-brand-500"
														placeholder="#000000"
														maxlength="7"
													/>
													<!-- Native color picker -->
													<input
														type="color"
														value={editorColors[key]}
														oninput={(e) => handleEditorColorChange(key, (e.target as HTMLInputElement).value)}
														class="h-8 w-8 shrink-0 cursor-pointer rounded border border-bg-modifier bg-transparent"
														title="Pick color for {THEME_COLOR_LABELS[key]}"
													/>
												</div>
											{/each}
										</div>
									</div>
								{/each}

								<!-- Live preview card -->
								<div class="mb-4">
									<h4 class="mb-2 text-xs font-bold uppercase tracking-wide text-text-muted">Live Preview</h4>
									<div class="overflow-hidden rounded-lg" style="border: 1px solid {editorColors['border-primary']}">
										<div class="p-3" style="background-color: {editorColors['bg-tertiary']}">
											<div class="flex items-center gap-2 mb-2">
												<div class="h-4 w-4 rounded-full" style="background-color: {editorColors['brand-500']}"></div>
												<span class="text-xs font-semibold" style="color: {editorColors['text-primary']}">Preview Channel</span>
											</div>
											<div class="rounded p-2" style="background-color: {editorColors['bg-secondary']}">
												<div class="flex items-start gap-2">
													<div class="relative h-8 w-8 shrink-0">
														<div class="h-8 w-8 rounded-full" style="background-color: {editorColors['brand-500']}"></div>
														<div class="absolute -bottom-0.5 -right-0.5 h-3 w-3 rounded-full border-2" style="background-color: {editorColors['status-online']}; border-color: {editorColors['bg-secondary']}"></div>
													</div>
													<div>
														<span class="text-xs font-semibold" style="color: {editorColors['text-primary']}">User</span>
														<span class="ml-1 text-2xs" style="color: {editorColors['text-muted']}">Today at 12:00</span>
														<p class="text-xs mt-0.5" style="color: {editorColors['text-secondary']}">
															This is a preview of how your theme looks. The colors update in real time.
														</p>
													</div>
												</div>
											</div>
											<div class="mt-2 rounded p-2" style="background-color: {editorColors['bg-primary']}; border: 1px solid {editorColors['border-primary']}">
												<div class="flex items-center gap-2">
													<span class="text-xs" style="color: {editorColors['text-muted']}">Type a message...</span>
												</div>
											</div>
											<!-- Status indicators row -->
											<div class="mt-2 flex items-center gap-3">
												<div class="flex items-center gap-1">
													<div class="h-2.5 w-2.5 rounded-full" style="background-color: {editorColors['status-online']}"></div>
													<span class="text-2xs" style="color: {editorColors['text-muted']}">Online</span>
												</div>
												<div class="flex items-center gap-1">
													<div class="h-2.5 w-2.5 rounded-full" style="background-color: {editorColors['status-idle']}"></div>
													<span class="text-2xs" style="color: {editorColors['text-muted']}">Idle</span>
												</div>
												<div class="flex items-center gap-1">
													<div class="h-2.5 w-2.5 rounded-full" style="background-color: {editorColors['status-dnd']}"></div>
													<span class="text-2xs" style="color: {editorColors['text-muted']}">DND</span>
												</div>
												<div class="flex items-center gap-1">
													<div class="h-2.5 w-2.5 rounded-full" style="background-color: {editorColors['status-offline']}"></div>
													<span class="text-2xs" style="color: {editorColors['text-muted']}">Offline</span>
												</div>
											</div>
											<div class="mt-2 flex items-center gap-2">
												<button class="rounded px-3 py-1 text-xs text-white" style="background-color: {editorColors['brand-500']}">
													Button
												</button>
												<a href="#link" class="text-xs underline" style="color: {editorColors['text-link']}" onclick={(e) => e.preventDefault()}>
													Link text
												</a>
											</div>
										</div>
									</div>
								</div>

								<div class="flex gap-2">
									<button class="btn-primary" onclick={saveCustomThemeFromEditor}>
										{editingExistingTheme ? 'Save Changes' : 'Save Theme'}
									</button>
									<button class="btn-secondary" onclick={cancelThemeEditor}>
										Cancel
									</button>
								</div>
							</div>
						{/if}

						<!-- Import Modal -->
						{#if showImportModal}
							<div class="mt-4 rounded-lg border border-bg-modifier bg-bg-secondary p-4">
								<h3 class="mb-2 text-sm font-semibold text-text-primary">Import Theme</h3>

								{#if importError}
									<div class="mb-3 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{importError}</div>
								{/if}

								<!-- File upload option -->
								<div class="mb-4 rounded-lg bg-bg-primary p-4 text-center">
									<p class="mb-2 text-xs text-text-muted">Select a .json theme file to import</p>
									<input
										bind:this={importFileInput}
										type="file"
										accept=".json,application/json"
										class="hidden"
										onchange={handleImportFile}
									/>
									<button class="btn-primary text-sm" onclick={triggerImportFilePicker}>
										<span class="inline-flex items-center gap-1.5">
											<svg class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
												<path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4" />
												<polyline points="17 8 12 3 7 8" />
												<line x1="12" y1="3" x2="12" y2="15" />
											</svg>
											Choose File
										</span>
									</button>
								</div>

								<!-- Divider -->
								<div class="mb-4 flex items-center gap-3">
									<div class="flex-1 border-t border-bg-modifier"></div>
									<span class="text-xs text-text-muted">or paste JSON</span>
									<div class="flex-1 border-t border-bg-modifier"></div>
								</div>

								<!-- Paste JSON option -->
								<textarea
									bind:value={importJson}
									class="input mb-3 w-full font-mono text-xs"
									rows="6"
									placeholder={'{"name": "My Theme", "version": 1, "colors": {...}}'}
								></textarea>

								<div class="flex gap-2">
									<button class="btn-primary" onclick={handleImportTheme} disabled={!importJson.trim()}>
										Import from Paste
									</button>
									<button class="btn-secondary" onclick={() => (showImportModal = false)}>
										Cancel
									</button>
								</div>
							</div>
						{/if}
					</div>

					<!-- Connected Accounts -->
					<div class="mt-8 border-t border-bg-modifier pt-6">
						<h2 class="mb-4 text-lg font-bold text-text-primary">Connected Accounts</h2>
						<p class="mb-4 text-xs text-text-muted">
							Add your social handles so others can find you elsewhere. These are stored locally.
						</p>

						{#if connectedAccountsSuccess}
							<div class="mb-4 rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{connectedAccountsSuccess}</div>
						{/if}

						<div class="space-y-4">
							<!-- GitHub -->
							<div class="rounded-lg bg-bg-secondary p-4">
								<div class="flex items-center gap-3">
									<svg class="h-5 w-5 shrink-0 text-text-muted" viewBox="0 0 24 24" fill="currentColor">
										<path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z" />
									</svg>
									<div class="flex-1">
										<label for="github-handle" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
											GitHub
										</label>
										<input
											id="github-handle"
											type="text"
											class="input w-full"
											placeholder="username"
											bind:value={connectedAccounts.github}
										/>
									</div>
								</div>
							</div>

							<!-- Twitter/X -->
							<div class="rounded-lg bg-bg-secondary p-4">
								<div class="flex items-center gap-3">
									<svg class="h-5 w-5 shrink-0 text-text-muted" viewBox="0 0 24 24" fill="currentColor">
										<path d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z" />
									</svg>
									<div class="flex-1">
										<label for="twitter-handle" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
											Twitter / X
										</label>
										<input
											id="twitter-handle"
											type="text"
											class="input w-full"
											placeholder="@handle"
											bind:value={connectedAccounts.twitter}
										/>
									</div>
								</div>
							</div>

							<!-- Website -->
							<div class="rounded-lg bg-bg-secondary p-4">
								<div class="flex items-center gap-3">
									<svg class="h-5 w-5 shrink-0 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
										<path d="M12 21a9 9 0 100-18 9 9 0 000 18z" />
										<path d="M3.6 9h16.8M3.6 15h16.8" />
										<path d="M12 3a15.3 15.3 0 014 9 15.3 15.3 0 01-4 9 15.3 15.3 0 01-4-9 15.3 15.3 0 014-9z" />
									</svg>
									<div class="flex-1">
										<label for="website-url" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
											Website
										</label>
										<input
											id="website-url"
											type="url"
											class="input w-full"
											placeholder="https://example.com"
											bind:value={connectedAccounts.website}
										/>
									</div>
								</div>
							</div>
						</div>

						<button class="btn-primary mt-4" onclick={saveConnectedAccounts}>
							Save Connected Accounts
						</button>
					</div>

					<!-- ==================== CUSTOM CSS ==================== -->
					<div class="mt-8 border-t border-bg-modifier pt-6">
						<h2 class="mb-2 text-lg font-bold text-text-primary">Custom CSS</h2>
						<p class="mb-1 text-xs text-text-muted">
							Advanced feature. Incorrect CSS may break the interface. Use at your own risk.
						</p>
						<p class="mb-4 text-xs text-text-muted">
							Paste custom CSS below to customize the app beyond the built-in theme options.
						</p>

						{#if customCssSuccess}
							<div class="mb-4 rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{customCssSuccess}</div>
						{/if}

						<div class="rounded-lg bg-bg-secondary p-4">
							<textarea
								bind:value={customCssText}
								class="w-full rounded border border-bg-modifier bg-bg-floating px-3 py-2 font-mono text-xs text-text-primary placeholder-text-muted outline-none focus:border-brand-500"
								rows="10"
								placeholder={".my-class {\n  color: red;\n}"}
								maxlength={customCssMaxLength}
							></textarea>
							<div class="mt-2 flex items-center justify-between">
								<span class="text-2xs text-text-muted">
									{customCssText.length} / {customCssMaxLength} characters
								</span>
								<div class="flex gap-2">
									<button
										class="btn-secondary text-xs"
										onclick={handleClearCustomCss}
										disabled={!customCssText && !$customCss}
									>
										Clear
									</button>
									<button
										class="btn-primary text-xs"
										onclick={handleSaveCustomCss}
									>
										Save Custom CSS
									</button>
								</div>
							</div>
						</div>
					</div>
				</div>

			<!-- ==================== BOTS ==================== -->
			{:else if currentTab === 'bots'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Bot Management</h1>

				{#if botError}
					<div class="mb-4 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{botError}</div>
				{/if}
				{#if botSuccess}
					<div class="mb-4 rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{botSuccess}</div>
				{/if}

				<!-- Create Bot -->
				<div class="mb-6 rounded-lg bg-bg-secondary p-4">
					<h3 class="mb-2 text-sm font-semibold text-text-primary">Create a Bot</h3>
					<p class="mb-3 text-xs text-text-muted">Bots can interact with the API using generated tokens.</p>
					<div class="mb-3">
						<label for="newBotName" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
							Bot Name
						</label>
						<input
							id="newBotName"
							type="text"
							class="input w-full"
							placeholder="my-bot"
							maxlength="32"
							bind:value={newBotName}
						/>
					</div>
					<div class="mb-3">
						<label for="newBotDesc" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
							Description (optional)
						</label>
						<input
							id="newBotDesc"
							type="text"
							class="input w-full"
							placeholder="What does this bot do?"
							maxlength="128"
							bind:value={newBotDescription}
						/>
					</div>
					<button
						class="btn-primary"
						onclick={handleCreateBot}
						disabled={creatingBot || !newBotName.trim()}
					>
						{creatingBot ? 'Creating...' : 'Create Bot'}
					</button>
				</div>

				<!-- Bot List -->
				{#if loadingBots}
					<div class="flex items-center gap-2 py-4">
						<div class="h-4 w-4 animate-spin rounded-full border-2 border-brand-500 border-t-transparent"></div>
						<span class="text-sm text-text-muted">Loading bots...</span>
					</div>
				{:else if myBots.length === 0}
					<div class="rounded-lg bg-bg-secondary p-6 text-center">
						<p class="text-sm text-text-muted">You have no bots yet.</p>
						<p class="text-xs text-text-muted">Create one above to get started.</p>
					</div>
				{:else}
					<div class="space-y-3">
						{#each myBots as bot (bot.id)}
							<div class="rounded-lg bg-bg-secondary">
								<!-- Bot Header -->
								<div class="flex items-center justify-between p-4">
									<div class="flex items-center gap-3">
										<div class="flex h-10 w-10 items-center justify-center rounded-full bg-brand-500/20 text-brand-400">
											<svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
												<path d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
											</svg>
										</div>
										<div>
											{#if editingBotId === bot.id}
												<div class="flex items-center gap-2">
													<input
														type="text"
														class="input w-40 text-sm"
														bind:value={editBotName}
														maxlength="32"
													/>
													<input
														type="text"
														class="input w-48 text-sm"
														bind:value={editBotDescription}
														placeholder="Description"
														maxlength="128"
													/>
													<button class="text-xs text-brand-400 hover:text-brand-300" onclick={handleSaveBot} disabled={savingBot}>
														{savingBot ? 'Saving...' : 'Save'}
													</button>
													<button class="text-xs text-text-muted hover:text-text-primary" onclick={cancelEditBot}>
														Cancel
													</button>
												</div>
											{:else}
												<h4 class="text-sm font-semibold text-text-primary">{bot.username}</h4>
												{#if bot.display_name}
													<p class="text-xs text-text-muted">{bot.display_name}</p>
												{/if}
												<p class="text-2xs text-text-muted font-mono">{bot.id}</p>
											{/if}
										</div>
									</div>
									<div class="flex items-center gap-2">
										<button
											class="text-xs text-text-muted hover:text-text-primary"
											onclick={() => toggleBotExpand(bot.id)}
										>
											{expandedBotId === bot.id ? 'Collapse' : 'Expand'}
										</button>
										{#if editingBotId !== bot.id}
											<button
												class="text-xs text-brand-400 hover:text-brand-300"
												onclick={() => startEditBot(bot)}
											>
												Edit
											</button>
										{/if}
										<button
											class="text-xs text-red-400 hover:text-red-300"
											onclick={() => handleDeleteBot(bot.id)}
										>
											Delete
										</button>
									</div>
								</div>

								<!-- Expanded Bot Details -->
								{#if expandedBotId === bot.id}
									<div class="border-t border-bg-modifier p-4">
										<!-- Token Management -->
										<div class="mb-6">
											<h5 class="mb-3 text-xs font-bold uppercase tracking-wide text-text-muted">API Tokens</h5>

											{#if createdTokenRaw}
												<div class="mb-3 rounded bg-green-500/10 p-3">
													<p class="mb-1 text-xs font-semibold text-green-400">Token created! Copy it now -- it will not be shown again.</p>
													<div class="flex items-center gap-2">
														<code class="flex-1 break-all rounded bg-bg-primary px-2 py-1 text-xs text-text-primary">{createdTokenRaw}</code>
														<button
															class="btn-secondary text-xs"
															onclick={() => { copyToClipboard(createdTokenRaw!); }}
														>
															Copy
														</button>
													</div>
												</div>
											{/if}

											<div class="mb-3 flex items-center gap-2">
												<input
													type="text"
													class="input flex-1 text-sm"
													placeholder="Token name (optional)"
													maxlength="64"
													bind:value={newTokenName}
												/>
												<button
													class="btn-primary text-xs"
													onclick={() => handleCreateToken(bot.id)}
													disabled={creatingToken}
												>
													{creatingToken ? 'Generating...' : 'Generate Token'}
												</button>
											</div>

											{#if loadingBotTokens === bot.id}
												<p class="text-xs text-text-muted">Loading tokens...</p>
											{:else if (botTokens[bot.id] ?? []).length === 0}
												<p class="text-xs text-text-muted">No tokens yet. Generate one to authenticate your bot.</p>
											{:else}
												<div class="space-y-2">
													{#each botTokens[bot.id] ?? [] as token (token.id)}
														<div class="flex items-center justify-between rounded bg-bg-primary p-2">
															<div>
																<span class="text-sm text-text-primary">{token.name}</span>
																<span class="ml-2 text-2xs text-text-muted">
																	Created {new Date(token.created_at).toLocaleDateString()}
																	{#if token.last_used_at}
																		&middot; Last used {new Date(token.last_used_at).toLocaleDateString()}
																	{/if}
																</span>
															</div>
															<button
																class="text-xs text-red-400 hover:text-red-300"
																onclick={() => handleDeleteToken(bot.id, token.id)}
															>
																Revoke
															</button>
														</div>
													{/each}
												</div>
											{/if}
										</div>

										<!-- Slash Commands -->
										<div>
											<h5 class="mb-3 text-xs font-bold uppercase tracking-wide text-text-muted">Slash Commands</h5>

											<div class="mb-3 flex items-end gap-2">
												<div class="flex-1">
													<label class="mb-1 block text-2xs text-text-muted">Name</label>
													<input
														type="text"
														class="input w-full text-sm"
														placeholder="command-name"
														maxlength="32"
														bind:value={newCommandName}
													/>
												</div>
												<div class="flex-1">
													<label class="mb-1 block text-2xs text-text-muted">Description</label>
													<input
														type="text"
														class="input w-full text-sm"
														placeholder="What does this command do?"
														maxlength="100"
														bind:value={newCommandDescription}
													/>
												</div>
												<button
													class="btn-primary text-xs"
													onclick={() => handleRegisterCommand(bot.id)}
													disabled={creatingCommand || !newCommandName.trim() || !newCommandDescription.trim()}
												>
													{creatingCommand ? 'Adding...' : 'Add'}
												</button>
											</div>

											{#if loadingBotCommands === bot.id}
												<p class="text-xs text-text-muted">Loading commands...</p>
											{:else if (botCommands[bot.id] ?? []).length === 0}
												<p class="text-xs text-text-muted">No slash commands registered.</p>
											{:else}
												<div class="space-y-2">
													{#each botCommands[bot.id] ?? [] as cmd (cmd.id)}
														<div class="flex items-center justify-between rounded bg-bg-primary p-2">
															<div>
																<span class="text-sm font-medium text-text-primary">/{cmd.name}</span>
																<span class="ml-2 text-xs text-text-muted">{cmd.description}</span>
																{#if cmd.guild_id}
																	<span class="ml-1 rounded bg-bg-modifier px-1 py-0.5 text-2xs text-text-muted">Guild-scoped</span>
																{:else}
																	<span class="ml-1 rounded bg-brand-500/10 px-1 py-0.5 text-2xs text-brand-400">Global</span>
																{/if}
															</div>
															<button
																class="text-xs text-red-400 hover:text-red-300"
																onclick={() => handleDeleteCommand(bot.id, cmd.id)}
															>
																Delete
															</button>
														</div>
													{/each}
												</div>
											{/if}
										</div>
									</div>
								{/if}
							</div>
						{/each}
					</div>
				{/if}

			<!-- ==================== VOICE & VIDEO ==================== -->
			{:else if currentTab === 'voice'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Voice & Video</h1>

				{#if voiceLoading}
					<p class="text-sm text-text-muted">Loading voice preferences...</p>
				{:else if voicePrefs}
					{#if voiceError}
						<div class="mb-4 rounded-lg bg-red-500/10 px-4 py-2 text-sm text-red-400">{voiceError}</div>
					{/if}
					{#if voiceSuccess}
						<div class="mb-4 rounded-lg bg-status-online/10 px-4 py-2 text-sm text-status-online">{voiceSuccess}</div>
					{/if}

					<!-- Audio Input -->
					<div class="mb-6">
						<h2 class="mb-3 text-2xs font-medium uppercase tracking-wide text-text-muted">Audio Input</h2>
						<div class="flex flex-col gap-4 rounded-lg bg-bg-secondary p-4">
							<div class="flex flex-col gap-1">
								<label class="text-sm font-medium text-text-secondary">Input Device</label>
								<select
									class="rounded border border-bg-tertiary bg-bg-primary px-3 py-2 text-sm text-text-primary outline-none focus:border-brand-500"
									bind:value={inputDeviceId}
								>
									<option value="">Default</option>
									{#each availableInputDevices as device}
										<option value={device.deviceId}>{device.label || `Microphone ${device.deviceId.slice(0, 8)}`}</option>
									{/each}
								</select>
							</div>

							<div class="flex flex-col gap-1">
								<label class="flex items-center justify-between text-sm font-medium text-text-secondary">
									Input Volume
									<span class="text-xs text-text-muted">{Math.round(voicePrefs.input_volume * 100)}%</span>
								</label>
								<input
									type="range"
									min="0"
									max="2"
									step="0.05"
									bind:value={voicePrefs.input_volume}
									class="w-full accent-brand-500"
								/>
							</div>

							<label class="flex items-center justify-between">
								<span class="text-sm text-text-primary">Noise Suppression</span>
								<input type="checkbox" bind:checked={voicePrefs.noise_suppression} class="accent-brand-500" />
							</label>

							<label class="flex items-center justify-between">
								<span class="text-sm text-text-primary">Echo Cancellation</span>
								<input type="checkbox" bind:checked={voicePrefs.echo_cancellation} class="accent-brand-500" />
							</label>

							<label class="flex items-center justify-between">
								<span class="text-sm text-text-primary">Auto Gain Control</span>
								<input type="checkbox" bind:checked={voicePrefs.auto_gain_control} class="accent-brand-500" />
							</label>
						</div>
					</div>

					<!-- Audio Output -->
					<div class="mb-6">
						<h2 class="mb-3 text-2xs font-medium uppercase tracking-wide text-text-muted">Audio Output</h2>
						<div class="flex flex-col gap-4 rounded-lg bg-bg-secondary p-4">
							<div class="flex flex-col gap-1">
								<label class="text-sm font-medium text-text-secondary">Output Device</label>
								<select
									class="rounded border border-bg-tertiary bg-bg-primary px-3 py-2 text-sm text-text-primary outline-none focus:border-brand-500"
									bind:value={outputDeviceId}
								>
									<option value="">Default</option>
									{#each availableOutputDevices as device}
										<option value={device.deviceId}>{device.label || `Speaker ${device.deviceId.slice(0, 8)}`}</option>
									{/each}
								</select>
							</div>

							<div class="flex flex-col gap-1">
								<label class="flex items-center justify-between text-sm font-medium text-text-secondary">
									Output Volume
									<span class="text-xs text-text-muted">{Math.round(voicePrefs.output_volume * 100)}%</span>
								</label>
								<input
									type="range"
									min="0"
									max="2"
									step="0.05"
									bind:value={voicePrefs.output_volume}
									class="w-full accent-brand-500"
								/>
							</div>
						</div>
					</div>

					<!-- Voice Activity -->
					<div class="mb-6">
						<h2 class="mb-3 text-2xs font-medium uppercase tracking-wide text-text-muted">Voice Activity</h2>
						<div class="flex flex-col gap-4 rounded-lg bg-bg-secondary p-4">
							<div class="flex flex-col gap-1">
								<label class="text-sm font-medium text-text-secondary">Input Mode</label>
								<select
									class="rounded border border-bg-tertiary bg-bg-primary px-3 py-2 text-sm text-text-primary outline-none focus:border-brand-500"
									bind:value={voicePrefs.input_mode}
								>
									<option value="vad">Voice Activity Detection</option>
									<option value="ptt">Push to Talk</option>
								</select>
							</div>

							{#if voicePrefs.input_mode === 'vad'}
								<div class="flex flex-col gap-1">
									<label class="flex items-center justify-between text-sm font-medium text-text-secondary">
										VAD Sensitivity
										<span class="text-xs text-text-muted">{Math.round(voicePrefs.vad_threshold * 100)}%</span>
									</label>
									<input
										type="range"
										min="0"
										max="1"
										step="0.05"
										bind:value={voicePrefs.vad_threshold}
										class="w-full accent-brand-500"
									/>
								</div>
							{:else}
								<div class="flex flex-col gap-1">
									<label class="text-sm font-medium text-text-secondary">PTT Keybind</label>
									<button
										class="rounded border px-4 py-2 text-sm font-mono {recordingVoicePTTKey ? 'border-brand-500 text-brand-400 animate-pulse' : 'border-bg-tertiary bg-bg-primary text-text-primary'}"
										onclick={() => recordingVoicePTTKey = !recordingVoicePTTKey}
									>
										{recordingVoicePTTKey ? 'Press a key...' : formatVoiceKeyName(voicePrefs.ptt_key)}
									</button>
								</div>
							{/if}
						</div>
					</div>

					<!-- Camera Defaults -->
					<div class="mb-6">
						<h2 class="mb-3 text-2xs font-medium uppercase tracking-wide text-text-muted">Camera Defaults</h2>
						<div class="flex flex-col gap-4 rounded-lg bg-bg-secondary p-4">
							<div class="flex flex-col gap-1">
								<label class="text-sm font-medium text-text-secondary">Resolution</label>
								<select
									class="rounded border border-bg-tertiary bg-bg-primary px-3 py-2 text-sm text-text-primary outline-none focus:border-brand-500"
									bind:value={voicePrefs.camera_resolution}
								>
									<option value="360p">360p (Low bandwidth)</option>
									<option value="720p">720p (HD)</option>
									<option value="1080p">1080p (Full HD)</option>
								</select>
							</div>

							<div class="flex flex-col gap-1">
								<label class="text-sm font-medium text-text-secondary">Frame Rate</label>
								<select
									class="rounded border border-bg-tertiary bg-bg-primary px-3 py-2 text-sm text-text-primary outline-none focus:border-brand-500"
									bind:value={voicePrefs.camera_framerate}
								>
									<option value={15}>15 fps (Low bandwidth)</option>
									<option value={30}>30 fps (Standard)</option>
									<option value={60}>60 fps (Smooth)</option>
								</select>
							</div>
						</div>
					</div>

					<!-- Screen Share Defaults -->
					<div class="mb-6">
						<h2 class="mb-3 text-2xs font-medium uppercase tracking-wide text-text-muted">Screen Share Defaults</h2>
						<div class="flex flex-col gap-4 rounded-lg bg-bg-secondary p-4">
							<div class="flex flex-col gap-1">
								<label class="text-sm font-medium text-text-secondary">Resolution</label>
								<select
									class="rounded border border-bg-tertiary bg-bg-primary px-3 py-2 text-sm text-text-primary outline-none focus:border-brand-500"
									bind:value={voicePrefs.screenshare_resolution}
								>
									<option value="720p">720p (HD)</option>
									<option value="1080p">1080p (Full HD)</option>
									<option value="4k">4K (Ultra HD)</option>
								</select>
							</div>

							<div class="flex flex-col gap-1">
								<label class="text-sm font-medium text-text-secondary">Frame Rate</label>
								<select
									class="rounded border border-bg-tertiary bg-bg-primary px-3 py-2 text-sm text-text-primary outline-none focus:border-brand-500"
									bind:value={voicePrefs.screenshare_framerate}
								>
									<option value={15}>15 fps (Low bandwidth)</option>
									<option value={30}>30 fps (Standard)</option>
									<option value={60}>60 fps (Smooth)</option>
								</select>
							</div>

							<label class="flex items-center justify-between">
								<span class="text-sm text-text-primary">Share System Audio</span>
								<input type="checkbox" bind:checked={voicePrefs.screenshare_audio} class="accent-brand-500" />
							</label>
						</div>
					</div>

					<!-- Save Button -->
					<button
						class="rounded bg-brand-500 px-6 py-2 text-sm font-semibold text-white hover:brightness-110 disabled:cursor-not-allowed disabled:opacity-60"
						onclick={saveVoicePreferences}
						disabled={voiceSaving}
					>
						{voiceSaving ? 'Saving...' : 'Save Changes'}
					</button>
				{:else if voiceError}
					<div class="rounded-lg bg-red-500/10 px-4 py-2 text-sm text-red-400">{voiceError}</div>
				{/if}

			<!-- ==================== DATA & PRIVACY ==================== -->
			{:else if currentTab === 'encryption'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Encryption</h1>

				<!-- E2EE info -->
				<div class="mb-6 rounded-lg bg-bg-secondary p-4">
					<div class="flex items-center gap-3">
						<div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-status-online/15">
							<svg class="h-5 w-5 text-status-online" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
								<path d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
							</svg>
						</div>
						<div>
							<h3 class="text-sm font-semibold text-text-primary">End-to-End Encryption</h3>
							<p class="text-xs text-text-muted">
								Encryption is managed per-channel using passphrases. When you enable encryption on a channel,
								you set a passphrase that is used to derive the encryption key. Share the passphrase with channel
								members out-of-band. The server never sees the passphrase or the encryption key.
							</p>
						</div>
					</div>
				</div>

				<!-- Reset all keys -->
				<div class="mb-6 rounded-lg bg-bg-secondary p-4">
					<h3 class="mb-1 text-sm font-semibold text-text-primary">Clear All Keys</h3>
					<p class="mb-3 text-xs text-text-muted">
						Remove all stored encryption keys from this device. You will need to re-enter
						passphrases for any encrypted channels.
					</p>
					<button
						class="rounded-md bg-red-500 px-4 py-2 text-xs font-medium text-white hover:bg-red-600"
						onclick={() => { e2ee.reset(); addToast('All encryption keys cleared', 'success'); }}
					>
						Clear All Keys
					</button>
				</div>

			{:else if currentTab === 'data'}
				<h1 class="mb-6 text-xl font-bold text-text-primary">Data & Privacy</h1>

				<!-- GDPR Data Export -->
				<div class="mb-6 rounded-lg bg-bg-secondary p-4">
					<h3 class="mb-1 text-sm font-semibold text-text-primary">Export My Data</h3>
					<p class="mb-3 text-xs text-text-muted">
						Download a copy of all your data including your profile, messages, guild memberships,
						bookmarks, reactions, read states, and relationships. This complies with GDPR data
						portability requirements. Limited to one export per 24 hours.
					</p>
					{#if exportDataSuccess}
						<div class="mb-3 rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{exportDataSuccess}</div>
					{/if}
					{#if exportDataError}
						<div class="mb-3 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{exportDataError}</div>
					{/if}
					<button
						class="btn-primary"
						onclick={handleExportData}
						disabled={exportingData}
					>
						{#if exportingData}
							<span class="flex items-center gap-2">
								<span class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
								Exporting...
							</span>
						{:else}
							Export My Data
						{/if}
					</button>
				</div>

				<!-- Account Migration -->
				<div class="mb-6 rounded-lg bg-bg-secondary p-4">
					<h3 class="mb-1 text-sm font-semibold text-text-primary">Account Migration</h3>
					<p class="mb-3 text-xs text-text-muted">
						Export your account profile and settings for migration to another AmityVox instance,
						or import data from another instance. Messages are not included as they belong to
						the originating instance.
					</p>

					{#if exportAccountSuccess}
						<div class="mb-3 rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{exportAccountSuccess}</div>
					{/if}
					{#if exportAccountError}
						<div class="mb-3 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{exportAccountError}</div>
					{/if}
					{#if importAccountSuccess}
						<div class="mb-3 rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{importAccountSuccess}</div>
					{/if}
					{#if importAccountError}
						<div class="mb-3 rounded bg-red-500/10 px-3 py-2 text-sm text-red-400">{importAccountError}</div>
					{/if}

					<div class="flex flex-wrap items-center gap-3">
						<button
							class="btn-primary"
							onclick={handleExportAccount}
							disabled={exportingAccount}
						>
							{#if exportingAccount}
								<span class="flex items-center gap-2">
									<span class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
									Exporting...
								</span>
							{:else}
								Export Account
							{/if}
						</button>

						<div class="flex items-center gap-2">
							<input
								bind:this={accountImportFileInput}
								type="file"
								accept=".json"
								class="text-xs text-text-muted file:mr-2 file:rounded file:border-0 file:bg-bg-modifier file:px-3 file:py-1.5 file:text-xs file:text-text-primary file:cursor-pointer hover:file:bg-bg-modifier/80"
							/>
							<button
								class="btn-secondary text-sm"
								onclick={handleImportAccount}
								disabled={importingAccount}
							>
								{importingAccount ? 'Importing...' : 'Import Account'}
							</button>
						</div>
					</div>
				</div>

				<!-- Information section -->
				<div class="rounded-lg bg-bg-secondary p-4">
					<h3 class="mb-1 text-sm font-semibold text-text-primary">About Your Data</h3>
					<div class="space-y-2 text-xs text-text-muted">
						<p>
							<span class="font-semibold text-text-secondary">Full data export</span> includes
							everything: your profile, messages (up to 10,000 most recent), guild memberships with
							roles, bookmarks, reactions, read states, and friend/block relationships.
						</p>
						<p>
							<span class="font-semibold text-text-secondary">Account migration export</span> includes
							only your profile settings (display name, bio, status, pronouns, accent color) and
							client settings. Messages remain on the originating instance.
						</p>
						<p>
							<span class="font-semibold text-text-secondary">Account import</span> applies the
							profile and settings from an export file to your current account. It will not change
							your username, email, or password.
						</p>
						<p>
							To permanently delete your account and all associated data, use the
							<button class="text-red-400 underline hover:text-red-300" onclick={() => (currentTab = 'account')}>
								My Account
							</button>
							tab.
						</p>
					</div>
				</div>
			{/if}
		</div>
	</div>
</div>

<!-- Image Cropper Modal -->
<Modal open={!!cropperFile} title={cropperTarget === 'avatar' ? 'Crop Avatar' : 'Crop Banner'} onclose={() => (cropperFile = null)}>
	{#if cropperFile}
		<ImageCropper
			file={cropperFile}
			shape={cropperTarget === 'avatar' ? 'circle' : 'rect'}
			outputWidth={cropperTarget === 'avatar' ? 256 : 960}
			outputHeight={cropperTarget === 'avatar' ? 256 : 320}
			oncrop={handleCropComplete}
			oncancel={() => (cropperFile = null)}
		/>
	{/if}
</Modal>
