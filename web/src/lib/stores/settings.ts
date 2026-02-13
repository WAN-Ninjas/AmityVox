// Settings store -- manages user preferences including custom themes and DND scheduling.
// Persists to localStorage and syncs to user_settings API when available.

import { writable, derived, get } from 'svelte/store';
import { api } from '$lib/api/client';

// --- Custom Theme Types ---

export interface CustomThemeColors {
	'brand-500': string;
	'brand-600': string;
	'brand-700': string;
	'bg-primary': string;
	'bg-secondary': string;
	'bg-tertiary': string;
	'bg-modifier': string;
	'bg-floating': string;
	'text-primary': string;
	'text-secondary': string;
	'text-muted': string;
	'text-link': string;
	'status-online': string;
	'status-idle': string;
	'status-dnd': string;
	'status-offline': string;
	'border-primary': string;
	'scrollbar-thumb': string;
}

export interface CustomTheme {
	name: string;
	colors: CustomThemeColors;
	createdAt: string;
}

export interface ThemeExport {
	name: string;
	version: 1;
	colors: CustomThemeColors;
}

// --- DND Schedule Types ---

export interface DndSchedule {
	enabled: boolean;
	startHour: number; // 0-23
	startMinute: number; // 0-59
	endHour: number; // 0-23
	endMinute: number; // 0-59
}

// --- Default values ---

const DEFAULT_DND_SCHEDULE: DndSchedule = {
	enabled: false,
	startHour: 23,
	startMinute: 0,
	endHour: 7,
	endMinute: 0
};

export const DEFAULT_THEME_COLORS: CustomThemeColors = {
	'brand-500': '#3f51b5',
	'brand-600': '#3949ab',
	'brand-700': '#303f9f',
	'bg-primary': '#1e1f22',
	'bg-secondary': '#2b2d31',
	'bg-tertiary': '#313338',
	'bg-modifier': '#383a40',
	'bg-floating': '#111214',
	'text-primary': '#f2f3f5',
	'text-secondary': '#b5bac1',
	'text-muted': '#949ba4',
	'text-link': '#00a8fc',
	'status-online': '#23a55a',
	'status-idle': '#f0b232',
	'status-dnd': '#f23f43',
	'status-offline': '#80848e',
	'border-primary': '#383a40',
	'scrollbar-thumb': '#1e1f22'
};

// Color definitions for the editor UI labels.
export const THEME_COLOR_LABELS: Record<keyof CustomThemeColors, string> = {
	'brand-500': 'Brand Primary',
	'brand-600': 'Brand Hover',
	'brand-700': 'Brand Active',
	'bg-primary': 'Background Primary',
	'bg-secondary': 'Background Secondary',
	'bg-tertiary': 'Background Tertiary',
	'bg-modifier': 'Background Modifier',
	'bg-floating': 'Background Floating',
	'text-primary': 'Text Primary',
	'text-secondary': 'Text Secondary',
	'text-muted': 'Text Muted',
	'text-link': 'Text Link',
	'status-online': 'Online',
	'status-idle': 'Idle',
	'status-dnd': 'Do Not Disturb',
	'status-offline': 'Offline',
	'border-primary': 'Border Primary',
	'scrollbar-thumb': 'Scrollbar Thumb'
};

// Color groupings for the editor UI.
export const THEME_COLOR_GROUPS = [
	{
		label: 'Brand',
		keys: ['brand-500', 'brand-600', 'brand-700'] as (keyof CustomThemeColors)[]
	},
	{
		label: 'Backgrounds',
		keys: ['bg-primary', 'bg-secondary', 'bg-tertiary', 'bg-modifier', 'bg-floating'] as (keyof CustomThemeColors)[]
	},
	{
		label: 'Text',
		keys: ['text-primary', 'text-secondary', 'text-muted', 'text-link'] as (keyof CustomThemeColors)[]
	},
	{
		label: 'Status',
		keys: ['status-online', 'status-idle', 'status-dnd', 'status-offline'] as (keyof CustomThemeColors)[]
	},
	{
		label: 'UI Elements',
		keys: ['border-primary', 'scrollbar-thumb'] as (keyof CustomThemeColors)[]
	}
];

// --- Notification Sound Defaults ---

const DEFAULT_SOUND_PRESET = 'default';
const DEFAULT_SOUND_VOLUME = 80;

// --- Custom CSS ---

const CUSTOM_CSS_MAX_LENGTH = 10000;
const CUSTOM_CSS_STORAGE_KEY = 'amityvox_custom_css';

// Sanitize custom CSS to prevent XSS attacks.
// Strips characters and patterns that have no valid use in CSS but could
// enable HTML injection or dangerous URL scheme execution.
export function sanitizeCustomCss(css: string): string {
	let sanitized = css.length > CUSTOM_CSS_MAX_LENGTH
		? css.slice(0, CUSTOM_CSS_MAX_LENGTH)
		: css;

	// '<' has no valid use in CSS — strip all occurrences to prevent
	// HTML injection (e.g. <script>, </style>) including nested bypasses.
	sanitized = sanitized.replace(/</g, '');
	// Dangerous URL schemes.
	sanitized = sanitized.replace(/javascript\s*:/gi, '');
	sanitized = sanitized.replace(/vbscript\s*:/gi, '');
	sanitized = sanitized.replace(/data\s*:/gi, '');
	// CSS expression evaluation (IE).
	sanitized = sanitized.replace(/expression\s*\(/gi, '');
	// External resource loading.
	sanitized = sanitized.replace(/@import/gi, '');
	// Mozilla XBL binding.
	sanitized = sanitized.replace(/-moz-binding/gi, '');

	return sanitized;
}

// Apply custom CSS by injecting/updating a <style> element in document.head.
function applyCustomCssToDocument(css: string) {
	if (typeof document === 'undefined') return;
	const id = 'amityvox-custom-css';
	let styleEl = document.getElementById(id) as HTMLStyleElement | null;
	if (!css) {
		// Remove the style element if CSS is empty.
		if (styleEl) {
			styleEl.remove();
		}
		return;
	}
	if (!styleEl) {
		styleEl = document.createElement('style');
		styleEl.id = id;
		document.head.appendChild(styleEl);
	}
	styleEl.textContent = sanitizeCustomCss(css);
}

// --- Stores ---

export const customThemes = writable<CustomTheme[]>([]);
export const activeCustomThemeName = writable<string | null>(null);
export const dndSchedule = writable<DndSchedule>(DEFAULT_DND_SCHEDULE);
export const dndManualOverride = writable<boolean>(false);
export const notificationSoundsEnabled = writable<boolean>(true);
export const notificationSoundPreset = writable<string>(DEFAULT_SOUND_PRESET);
export const notificationVolume = writable<number>(DEFAULT_SOUND_VOLUME);
export const customCss = writable<string>('');

// Whether DND is currently active (either by schedule or manual toggle).
export const isDndActive = derived(
	[dndSchedule, dndManualOverride],
	([$schedule, $manual]) => {
		if ($manual) return true;
		if (!$schedule.enabled) return false;
		return isWithinDndWindow($schedule);
	}
);

// --- DND Time Logic ---

function isWithinDndWindow(schedule: DndSchedule): boolean {
	const now = new Date();
	const currentMinutes = now.getHours() * 60 + now.getMinutes();
	const startMinutes = schedule.startHour * 60 + schedule.startMinute;
	const endMinutes = schedule.endHour * 60 + schedule.endMinute;

	if (startMinutes <= endMinutes) {
		// Same day window (e.g., 9:00 to 17:00)
		return currentMinutes >= startMinutes && currentMinutes < endMinutes;
	} else {
		// Overnight window (e.g., 23:00 to 7:00)
		return currentMinutes >= startMinutes || currentMinutes < endMinutes;
	}
}

// --- Custom Theme CSS Application ---

export function applyCustomThemeColors(colors: CustomThemeColors) {
	const root = document.documentElement;
	for (const [key, value] of Object.entries(colors)) {
		root.style.setProperty(`--${key}`, value);
	}
}

export function clearCustomThemeColors() {
	const root = document.documentElement;
	for (const key of Object.keys(DEFAULT_THEME_COLORS)) {
		root.style.removeProperty(`--${key}`);
	}
}

// Get the current computed CSS variable values from the active built-in theme.
export function getCurrentThemeColors(): CustomThemeColors {
	const root = document.documentElement;
	const style = getComputedStyle(root);
	const colors: Partial<CustomThemeColors> = {};
	for (const key of Object.keys(DEFAULT_THEME_COLORS) as (keyof CustomThemeColors)[]) {
		const val = style.getPropertyValue(`--${key}`).trim();
		colors[key] = val || DEFAULT_THEME_COLORS[key];
	}
	return colors as CustomThemeColors;
}

// --- Persistence ---

export function loadSettings() {
	// Load custom themes, filling in any missing color keys with defaults.
	try {
		const stored = localStorage.getItem('av-custom-themes');
		if (stored) {
			const parsed = JSON.parse(stored);
			if (Array.isArray(parsed)) {
				const migrated = parsed.map((t: CustomTheme) => ({
					...t,
					colors: { ...DEFAULT_THEME_COLORS, ...t.colors }
				}));
				customThemes.set(migrated);
			}
		}
	} catch {
		// Ignore malformed JSON.
	}

	// Load active custom theme name
	const activeTheme = localStorage.getItem('av-active-custom-theme');
	activeCustomThemeName.set(activeTheme);

	// Load DND schedule
	try {
		const stored = localStorage.getItem('av-dnd-schedule');
		if (stored) {
			const parsed = JSON.parse(stored);
			dndSchedule.set({ ...DEFAULT_DND_SCHEDULE, ...parsed });
		}
	} catch {
		// Ignore malformed JSON.
	}

	// Load manual DND override
	dndManualOverride.set(localStorage.getItem('av-dnd-manual') === 'true');

	// Load notification sound preferences
	const storedSoundsEnabled = localStorage.getItem('av-sounds-enabled');
	if (storedSoundsEnabled !== null) {
		notificationSoundsEnabled.set(storedSoundsEnabled !== 'false');
	}
	const storedPreset = localStorage.getItem('av-sound-preset');
	if (storedPreset) {
		notificationSoundPreset.set(storedPreset);
	}
	const storedVolume = localStorage.getItem('av-sound-volume');
	if (storedVolume) {
		const vol = parseInt(storedVolume, 10);
		if (!isNaN(vol) && vol >= 0 && vol <= 100) {
			notificationVolume.set(vol);
		}
	}

	// Load custom CSS
	try {
		const storedCss = localStorage.getItem(CUSTOM_CSS_STORAGE_KEY);
		if (storedCss) {
			customCss.set(storedCss);
			applyCustomCssToDocument(storedCss);
		}
	} catch {
		// Ignore read errors.
	}

	// Apply active custom theme if one is selected
	if (activeTheme) {
		const themes = get(customThemes);
		const theme = themes.find((t) => t.name === activeTheme);
		if (theme) {
			applyCustomThemeColors(theme.colors);
		}
	}
}

export function saveCustomThemes() {
	const themes = get(customThemes);
	localStorage.setItem('av-custom-themes', JSON.stringify(themes));
}

export function saveNotificationSoundsEnabled() {
	const enabled = get(notificationSoundsEnabled);
	localStorage.setItem('av-sounds-enabled', String(enabled));
}

export function saveNotificationSoundPreset() {
	const preset = get(notificationSoundPreset);
	localStorage.setItem('av-sound-preset', preset);
}

export function saveNotificationVolume() {
	const vol = get(notificationVolume);
	localStorage.setItem('av-sound-volume', String(vol));
}

export function saveCustomCss(css: string) {
	const sanitized = sanitizeCustomCss(css);
	customCss.set(sanitized);
	localStorage.setItem(CUSTOM_CSS_STORAGE_KEY, sanitized);
	applyCustomCssToDocument(sanitized);
}

export function clearCustomCss() {
	customCss.set('');
	localStorage.removeItem(CUSTOM_CSS_STORAGE_KEY);
	applyCustomCssToDocument('');
}

export function saveDndSchedule() {
	const schedule = get(dndSchedule);
	localStorage.setItem('av-dnd-schedule', JSON.stringify(schedule));
}

export function saveDndManualOverride() {
	const manual = get(dndManualOverride);
	localStorage.setItem('av-dnd-manual', String(manual));
}

// --- Custom Theme CRUD ---

export function addCustomTheme(theme: CustomTheme) {
	customThemes.update((themes) => {
		// Replace if same name exists.
		const filtered = themes.filter((t) => t.name !== theme.name);
		return [...filtered, theme];
	});
	saveCustomThemes();
}

export function deleteCustomTheme(name: string) {
	customThemes.update((themes) => themes.filter((t) => t.name !== name));
	if (get(activeCustomThemeName) === name) {
		activeCustomThemeName.set(null);
		clearCustomThemeColors();
		localStorage.removeItem('av-active-custom-theme');
	}
	saveCustomThemes();
}

export function activateCustomTheme(name: string) {
	const themes = get(customThemes);
	const theme = themes.find((t) => t.name === name);
	if (!theme) return;

	activeCustomThemeName.set(name);
	localStorage.setItem('av-active-custom-theme', name);
	applyCustomThemeColors(theme.colors);
}

export function deactivateCustomTheme() {
	activeCustomThemeName.set(null);
	localStorage.removeItem('av-active-custom-theme');
	clearCustomThemeColors();
}

// --- Theme Import/Export ---

export function exportTheme(theme: CustomTheme): string {
	const exported: ThemeExport = {
		name: theme.name,
		version: 1,
		colors: { ...theme.colors }
	};
	return JSON.stringify(exported, null, 2);
}

export function importTheme(jsonStr: string): CustomTheme {
	const parsed = JSON.parse(jsonStr);

	if (!parsed.name || typeof parsed.name !== 'string') {
		throw new Error('Theme must have a name.');
	}
	if (!parsed.colors || typeof parsed.colors !== 'object') {
		throw new Error('Theme must have a colors object.');
	}

	// Validate present color keys have valid hex format.
	// Missing keys will be filled from defaults for backward compatibility.
	for (const key of Object.keys(parsed.colors)) {
		if (typeof parsed.colors[key] === 'string' && !/^#[0-9a-fA-F]{6}$/.test(parsed.colors[key])) {
			throw new Error(`Invalid hex color for ${key}: ${parsed.colors[key]}`);
		}
	}

	// Merge with defaults so older theme exports (missing new keys) still work.
	const colors: CustomThemeColors = { ...DEFAULT_THEME_COLORS };
	for (const key of Object.keys(DEFAULT_THEME_COLORS) as (keyof CustomThemeColors)[]) {
		if (typeof parsed.colors[key] === 'string' && /^#[0-9a-fA-F]{6}$/.test(parsed.colors[key])) {
			colors[key] = parsed.colors[key];
		}
	}

	return {
		name: parsed.name,
		colors,
		createdAt: new Date().toISOString()
	};
}

// --- Sync to API ---

export async function syncSettingsToApi() {
	try {
		const schedule = get(dndSchedule);
		const themes = get(customThemes);
		const activeTheme = get(activeCustomThemeName);
		const soundPreset = get(notificationSoundPreset);
		const soundVolume = get(notificationVolume);
		const css = get(customCss);

		await api.updateUserSettings({
			dnd_schedule: schedule,
			custom_themes: themes,
			active_custom_theme: activeTheme,
			notification_sound_preset: soundPreset,
			notification_volume: soundVolume,
			custom_css: css || undefined
		} as any);
	} catch {
		// Silently fail -- localStorage is the primary store.
	}
}

export async function loadSettingsFromApi() {
	try {
		const settings = await api.getUserSettings();

		if (settings.dnd_schedule) {
			const schedule = settings.dnd_schedule as unknown as DndSchedule;
			dndSchedule.set({ ...DEFAULT_DND_SCHEDULE, ...schedule });
			saveDndSchedule();
		}

		if (settings.custom_themes) {
			const themes = settings.custom_themes as unknown as CustomTheme[];
			if (Array.isArray(themes)) {
				customThemes.set(themes);
				saveCustomThemes();
			}
		}

		if (settings.active_custom_theme) {
			const name = settings.active_custom_theme as unknown as string;
			activeCustomThemeName.set(name);
			localStorage.setItem('av-active-custom-theme', name);
		}

		if (settings.notification_sounds !== undefined) {
			notificationSoundsEnabled.set(settings.notification_sounds);
			saveNotificationSoundsEnabled();
		}

		if (settings.notification_sound_preset) {
			notificationSoundPreset.set(settings.notification_sound_preset);
			saveNotificationSoundPreset();
		}

		if (settings.notification_volume !== undefined && settings.notification_volume !== null) {
			notificationVolume.set(settings.notification_volume);
			saveNotificationVolume();
		}

		if (settings.custom_css !== undefined && settings.custom_css !== null) {
			const css = settings.custom_css as string;
			saveCustomCss(css);
		}
	} catch {
		// Use localStorage values if API fails.
	}
}

// --- DND Check Timer ---

let dndCheckInterval: ReturnType<typeof setInterval> | null = null;

// Start periodic DND checks to update the derived store when crossing time boundaries.
export function startDndChecker() {
	stopDndChecker();
	// Re-evaluate every 30 seconds by triggering a store update.
	dndCheckInterval = setInterval(() => {
		dndSchedule.update((s) => ({ ...s }));
	}, 30000);
}

export function stopDndChecker() {
	if (dndCheckInterval) {
		clearInterval(dndCheckInterval);
		dndCheckInterval = null;
	}
}
