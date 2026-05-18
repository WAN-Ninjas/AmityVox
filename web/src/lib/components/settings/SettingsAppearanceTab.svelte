<script lang="ts">
	import { onMount } from 'svelte';
	import {
		customThemes,
		activeCustomThemeName,
		customCss,
		addCustomTheme,
		deleteCustomTheme,
		activateCustomTheme,
		deactivateCustomTheme,
		exportTheme,
		importTheme,
		getCurrentThemeColors,
		applyCustomThemeColors,
		clearCustomThemeColors,
		saveCustomCss,
		clearCustomCss,
		syncSettingsToApi,
		DEFAULT_THEME_COLORS,
		THEME_COLOR_LABELS,
		THEME_COLOR_GROUPS,
		type CustomThemeColors,
		type CustomTheme
	} from '$lib/stores/settings';
	import { getErrorMessage } from '$lib/utils/apiError';

	type ThemeName = 'dark' | 'light' | 'amoled' | 'nord' | 'dracula' | 'catppuccin' | 'solarized' | 'high-contrast';
	type ConnectedAccounts = {
		github: string;
		twitter: string;
		website: string;
	};

	let theme = $state<ThemeName>('dark');
	let fontSize = $state(16);
	let compactMode = $state(false);
	let reducedMotion = $state(false);
	let dyslexicFont = $state(false);
	let appearanceSuccess = $state('');

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
	let importFileInput = $state<HTMLInputElement>();

	let customCssText = $state('');
	let customCssSuccess = $state('');
	const customCssMaxLength = 10000;

	let connectedAccounts = $state<ConnectedAccounts>({ github: '', twitter: '', website: '' });
	let connectedAccountsSuccess = $state('');

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

	onMount(() => {
		theme = (localStorage.getItem('av-theme') as ThemeName) ?? 'dark';
		fontSize = parseInt(localStorage.getItem('av-font-size') ?? '16', 10);
		compactMode = localStorage.getItem('av-compact') === 'true';
		reducedMotion = localStorage.getItem('av-reduced-motion') === 'true';
		dyslexicFont = localStorage.getItem('av-dyslexic-font') === 'true';
		customCssText = $customCss;

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

	function themeButtonClass(value: ThemeName): string {
		const base = 'rounded-lg border-2 px-4 py-3 text-sm transition-colors flex items-center gap-3';
		if (theme === value) return `${base} border-brand-500 bg-brand-500/10 text-text-primary`;
		return `${base} border-bg-modifier text-text-muted hover:border-bg-tertiary`;
	}

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
		appearanceSuccess = 'Appearance settings saved!';
		setTimeout(() => (appearanceSuccess = ''), 3000);
	}

	function openThemeEditor() {
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
		const root = document.documentElement;
		const currentThemeAttr = root.getAttribute('data-theme') || 'dark';
		clearCustomThemeColors();
		root.setAttribute('data-theme', preset);
		editorColors = getCurrentThemeColors();
		root.setAttribute('data-theme', currentThemeAttr);
		applyCustomThemeColors(editorColors);
	}

	function handleHexInput(key: keyof CustomThemeColors, value: string) {
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
		if ($activeCustomThemeName) {
			const active = $customThemes.find((value) => value.name === $activeCustomThemeName);
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
		} catch (err: unknown) {
			importError = getErrorMessage(err, 'Failed to import theme.');
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
			} catch (err: unknown) {
				importError = getErrorMessage(err, 'Failed to import theme from file.');
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
		applyCustomThemeColors(themeObj.colors);
	}

	function saveConnectedAccounts() {
		localStorage.setItem('av-connected-accounts', JSON.stringify(connectedAccounts));
		connectedAccountsSuccess = 'Connected accounts saved!';
		setTimeout(() => (connectedAccountsSuccess = ''), 3000);
	}

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
</script>

<h1 class="mb-6 text-xl font-bold text-text-primary">Appearance</h1>

{#if appearanceSuccess}
	<div class="mb-4 rounded bg-green-500/10 px-3 py-2 text-sm text-green-400">{appearanceSuccess}</div>
{/if}

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
