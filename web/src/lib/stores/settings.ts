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
	'text-link': '#00a8fc'
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
	'text-link': 'Text Link'
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
	}
];

// --- Stores ---

export const customThemes = writable<CustomTheme[]>([]);
export const activeCustomThemeName = writable<string | null>(null);
export const dndSchedule = writable<DndSchedule>(DEFAULT_DND_SCHEDULE);
export const dndManualOverride = writable<boolean>(false);

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
	// Load custom themes
	try {
		const stored = localStorage.getItem('av-custom-themes');
		if (stored) {
			const parsed = JSON.parse(stored);
			if (Array.isArray(parsed)) {
				customThemes.set(parsed);
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

	// Validate all required color keys are present.
	for (const key of Object.keys(DEFAULT_THEME_COLORS)) {
		if (typeof parsed.colors[key] !== 'string') {
			throw new Error(`Missing or invalid color: ${key}`);
		}
		// Validate it looks like a hex color.
		if (!/^#[0-9a-fA-F]{6}$/.test(parsed.colors[key])) {
			throw new Error(`Invalid hex color for ${key}: ${parsed.colors[key]}`);
		}
	}

	return {
		name: parsed.name,
		colors: parsed.colors,
		createdAt: new Date().toISOString()
	};
}

// --- Sync to API ---

export async function syncSettingsToApi() {
	try {
		const schedule = get(dndSchedule);
		const themes = get(customThemes);
		const activeTheme = get(activeCustomThemeName);

		await api.updateUserSettings({
			dnd_schedule: schedule,
			custom_themes: themes,
			active_custom_theme: activeTheme
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
