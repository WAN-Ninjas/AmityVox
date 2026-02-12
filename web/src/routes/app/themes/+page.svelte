<script lang="ts">
	import { api } from '$lib/api/client';
	import { onMount } from 'svelte';
	import { addToast } from '$lib/stores/toast';

	interface SharedTheme {
		id: string;
		user_id: string;
		author_name: string;
		name: string;
		description: string;
		variables: Record<string, string>;
		custom_css: string;
		preview_colors: string[];
		share_code: string;
		downloads: number;
		like_count: number;
		liked: boolean;
		created_at: string;
	}

	let themes = $state<SharedTheme[]>([]);
	let loading = $state(true);
	let error = $state('');
	let sort = $state<'newest' | 'downloads' | 'likes'>('newest');
	let search = $state('');
	let searchTimeout: ReturnType<typeof setTimeout>;

	// Share theme modal
	let showShareModal = $state(false);
	let shareName = $state('');
	let shareDescription = $state('');
	let sharing = $state(false);

	function authHeaders(): Record<string, string> {
		const token = api.getToken();
		const headers: Record<string, string> = { 'Content-Type': 'application/json' };
		if (token) headers['Authorization'] = `Bearer ${token}`;
		return headers;
	}

	async function loadThemes() {
		loading = true;
		error = '';
		try {
			const params = new URLSearchParams({ sort, limit: '60' });
			if (search.trim()) params.set('q', search.trim());
			const res = await fetch(`/api/v1/themes?${params}`, {
				headers: authHeaders()
			});
			if (!res.ok) {
				const body = await res.json();
				throw new Error(body?.error?.message || 'Failed to load themes');
			}
			const data = await res.json();
			themes = data.data ?? [];
		} catch (err: any) {
			error = err.message || 'Failed to load themes';
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		loadThemes();
	});

	$effect(() => {
		sort;
		loadThemes();
	});

	function onSearchInput() {
		clearTimeout(searchTimeout);
		searchTimeout = setTimeout(() => loadThemes(), 300);
	}

	async function toggleLike(theme: SharedTheme) {
		try {
			const method = theme.liked ? 'DELETE' : 'PUT';
			const res = await fetch(`/api/v1/themes/${theme.id}/like`, {
				method,
				headers: authHeaders()
			});
			if (!res.ok && res.status !== 204) {
				throw new Error('Failed to update like');
			}
			if (theme.liked) {
				themes = themes.map(t =>
					t.id === theme.id ? { ...t, liked: false, like_count: t.like_count - 1 } : t
				);
			} else {
				themes = themes.map(t =>
					t.id === theme.id ? { ...t, liked: true, like_count: t.like_count + 1 } : t
				);
			}
		} catch (err: any) {
			addToast('Failed to update like', 'error');
		}
	}

	function applyTheme(theme: SharedTheme) {
		if (theme.variables && typeof theme.variables === 'object') {
			const root = document.documentElement;
			for (const [key, value] of Object.entries(theme.variables)) {
				root.style.setProperty(`--${key}`, value);
			}
			// Store theme in localStorage for persistence.
			localStorage.setItem('amityvox_custom_theme', JSON.stringify(theme.variables));
			localStorage.setItem('amityvox_custom_theme_name', theme.name);
			document.documentElement.setAttribute('data-theme', 'custom');
		}
		if (theme.custom_css) {
			let styleEl = document.getElementById('amityvox-custom-css');
			if (!styleEl) {
				styleEl = document.createElement('style');
				styleEl.id = 'amityvox-custom-css';
				document.head.appendChild(styleEl);
			}
			styleEl.textContent = theme.custom_css;
			localStorage.setItem('amityvox_custom_css', theme.custom_css);
		}
		addToast(`Theme "${theme.name}" applied`, 'success');
	}

	function copyShareLink(shareCode: string) {
		const url = `${window.location.origin}/app/themes?code=${shareCode}`;
		navigator.clipboard.writeText(url).then(
			() => addToast('Share link copied to clipboard', 'success'),
			() => addToast('Failed to copy link', 'error')
		);
	}

	async function shareCurrentTheme() {
		if (!shareName.trim()) return;
		sharing = true;
		try {
			// Collect current CSS variables from root.
			const computed = getComputedStyle(document.documentElement);
			const varNames = [
				'brand-500', 'bg-primary', 'bg-secondary', 'bg-tertiary',
				'bg-modifier', 'bg-floating', 'text-primary', 'text-secondary',
				'text-muted', 'text-link'
			];
			const variables: Record<string, string> = {};
			const previewColors: string[] = [];
			for (const name of varNames) {
				const val = computed.getPropertyValue(`--${name}`).trim();
				if (val) {
					variables[name] = val;
					if (previewColors.length < 6) previewColors.push(val);
				}
			}

			const res = await fetch('/api/v1/themes', {
				method: 'POST',
				headers: authHeaders(),
				body: JSON.stringify({
					name: shareName.trim(),
					description: shareDescription.trim(),
					variables,
					preview_colors: previewColors,
					custom_css: localStorage.getItem('amityvox_custom_css') || ''
				})
			});
			if (!res.ok) {
				const body = await res.json();
				throw new Error(body?.error?.message || 'Failed to share theme');
			}

			addToast('Theme shared to gallery', 'success');
			showShareModal = false;
			shareName = '';
			shareDescription = '';
			loadThemes();
		} catch (err: any) {
			addToast(err.message || 'Failed to share theme', 'error');
		} finally {
			sharing = false;
		}
	}

	async function deleteTheme(themeId: string) {
		if (!confirm('Are you sure you want to delete this theme?')) return;
		try {
			const res = await fetch(`/api/v1/themes/${themeId}`, {
				method: 'DELETE',
				headers: authHeaders()
			});
			if (!res.ok && res.status !== 204) {
				const body = await res.json();
				throw new Error(body?.error?.message || 'Failed to delete theme');
			}
			themes = themes.filter(t => t.id !== themeId);
			addToast('Theme deleted', 'info');
		} catch (err: any) {
			addToast(err.message || 'Failed to delete theme', 'error');
		}
	}

	function formatDate(iso: string): string {
		return new Date(iso).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
	}

	function getPreviewColors(theme: SharedTheme): string[] {
		if (Array.isArray(theme.preview_colors) && theme.preview_colors.length > 0) {
			return theme.preview_colors.slice(0, 6);
		}
		// Fallback: extract from variables.
		if (theme.variables && typeof theme.variables === 'object') {
			const keys = ['bg-primary', 'bg-secondary', 'bg-tertiary', 'brand-500', 'text-primary', 'text-muted'];
			return keys.map(k => theme.variables[k]).filter(Boolean).slice(0, 6);
		}
		return [];
	}
</script>

<svelte:head>
	<title>Theme Gallery — AmityVox</title>
</svelte:head>

<div class="flex h-full flex-col">
	<!-- Header -->
	<div class="flex h-12 items-center justify-between border-b border-bg-modifier px-4">
		<div class="flex items-center gap-2">
			<svg class="h-5 w-5 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M7 21a4 4 0 01-4-4V5a2 2 0 012-2h4a2 2 0 012 2v12a4 4 0 01-4 4zm0 0h12a2 2 0 002-2v-4a2 2 0 00-2-2h-2.343M11 7.343l1.657-1.657a2 2 0 012.828 0l2.829 2.829a2 2 0 010 2.828l-8.486 8.485M7 17h.01" />
			</svg>
			<h1 class="text-base font-semibold text-text-primary">Theme Gallery</h1>
		</div>
		<button
			class="btn-primary text-sm"
			onclick={() => (showShareModal = true)}
		>
			Share Your Theme
		</button>
	</div>

	<!-- Toolbar: Search + Sort -->
	<div class="flex items-center gap-3 border-b border-bg-modifier px-4 py-2">
		<div class="relative flex-1">
			<svg class="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-text-muted" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
			</svg>
			<input
				type="text"
				class="input w-full pl-8"
				placeholder="Search themes..."
				bind:value={search}
				oninput={onSearchInput}
			/>
		</div>
		<div class="flex items-center gap-1 rounded-lg bg-bg-secondary p-0.5">
			<button
				class="rounded-md px-3 py-1 text-xs font-medium transition-colors {sort === 'newest' ? 'bg-bg-modifier text-text-primary' : 'text-text-muted hover:text-text-secondary'}"
				onclick={() => (sort = 'newest')}
			>
				Newest
			</button>
			<button
				class="rounded-md px-3 py-1 text-xs font-medium transition-colors {sort === 'downloads' ? 'bg-bg-modifier text-text-primary' : 'text-text-muted hover:text-text-secondary'}"
				onclick={() => (sort = 'downloads')}
			>
				Popular
			</button>
			<button
				class="rounded-md px-3 py-1 text-xs font-medium transition-colors {sort === 'likes' ? 'bg-bg-modifier text-text-primary' : 'text-text-muted hover:text-text-secondary'}"
				onclick={() => (sort = 'likes')}
			>
				Most Liked
			</button>
		</div>
	</div>

	<!-- Theme grid -->
	<div class="flex-1 overflow-y-auto p-6">
		{#if loading}
			<div class="flex items-center justify-center py-20">
				<p class="text-sm text-text-muted">Loading themes...</p>
			</div>
		{:else if error}
			<div class="flex items-center justify-center py-20">
				<p class="text-sm text-red-400">{error}</p>
			</div>
		{:else if themes.length === 0}
			<div class="flex flex-col items-center justify-center py-20 text-center">
				<svg class="mb-4 h-16 w-16 text-text-muted opacity-50" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
					<path d="M7 21a4 4 0 01-4-4V5a2 2 0 012-2h4a2 2 0 012 2v12a4 4 0 01-4 4zm0 0h12a2 2 0 002-2v-4a2 2 0 00-2-2h-2.343M11 7.343l1.657-1.657a2 2 0 012.828 0l2.829 2.829a2 2 0 010 2.828l-8.486 8.485M7 17h.01" />
				</svg>
				<h2 class="mb-2 text-lg font-semibold text-text-primary">No themes yet</h2>
				<p class="text-sm text-text-muted">Be the first to share a theme with the community.</p>
			</div>
		{:else}
			<div class="mx-auto grid max-w-5xl grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
				{#each themes as theme (theme.id)}
					{@const colors = getPreviewColors(theme)}
					<div class="group rounded-lg border border-bg-modifier bg-bg-secondary p-4 transition-colors hover:border-brand-500/30">
						<!-- Color preview dots -->
						{#if colors.length > 0}
							<div class="mb-3 flex items-center gap-1.5">
								{#each colors as color}
									<div
										class="h-6 w-6 rounded-full border border-bg-modifier"
										style="background-color: {color};"
										title={color}
									></div>
								{/each}
							</div>
						{:else}
							<div class="mb-3 flex h-6 items-center">
								<span class="text-xs text-text-muted">No preview</span>
							</div>
						{/if}

						<!-- Theme name + author -->
						<h3 class="truncate text-sm font-semibold text-text-primary">{theme.name}</h3>
						<p class="mt-0.5 truncate text-xs text-text-muted">
							by {theme.author_name}
						</p>

						<!-- Description -->
						{#if theme.description}
							<p class="mt-2 line-clamp-2 text-xs text-text-secondary">{theme.description}</p>
						{/if}

						<!-- Stats -->
						<div class="mt-3 flex items-center gap-3 text-xs text-text-muted">
							<span class="flex items-center gap-1" title="{theme.downloads} downloads">
								<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
									<path d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
								</svg>
								{theme.downloads}
							</span>
							<button
								class="flex items-center gap-1 transition-colors {theme.liked ? 'text-red-400' : 'hover:text-red-400'}"
								onclick={() => toggleLike(theme)}
								title="{theme.like_count} likes"
							>
								<svg class="h-3.5 w-3.5" fill={theme.liked ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
									<path d="M4.318 6.318a4.5 4.5 0 000 6.364L12 20.364l7.682-7.682a4.5 4.5 0 00-6.364-6.364L12 7.636l-1.318-1.318a4.5 4.5 0 00-6.364 0z" />
								</svg>
								{theme.like_count}
							</button>
							<span class="text-2xs">{formatDate(theme.created_at)}</span>
						</div>

						<!-- Actions -->
						<div class="mt-3 flex items-center gap-2">
							<button
								class="btn-primary flex-1 py-1.5 text-xs"
								onclick={() => applyTheme(theme)}
							>
								Apply
							</button>
							<button
								class="rounded border border-bg-modifier px-2 py-1.5 text-xs text-text-muted transition-colors hover:bg-bg-modifier hover:text-text-primary"
								onclick={() => copyShareLink(theme.share_code)}
								title="Copy share link"
							>
								<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
									<path d="M8.684 13.342C8.886 12.938 9 12.482 9 12c0-.482-.114-.938-.316-1.342m0 2.684a3 3 0 110-2.684m0 2.684l6.632 3.316m-6.632-6l6.632-3.316m0 0a3 3 0 105.367-2.684 3 3 0 00-5.367 2.684zm0 9.316a3 3 0 105.368 2.684 3 3 0 00-5.368-2.684z" />
								</svg>
							</button>
							<button
								class="rounded border border-bg-modifier px-2 py-1.5 text-xs text-red-400 opacity-0 transition-all hover:bg-red-500/10 group-hover:opacity-100"
								onclick={() => deleteTheme(theme.id)}
								title="Delete theme"
							>
								<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
									<path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
								</svg>
							</button>
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</div>
</div>

<!-- Share Theme Modal -->
{#if showShareModal}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
		onclick={() => (showShareModal = false)}
	>
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="w-full max-w-md rounded-lg bg-bg-floating p-6 shadow-xl"
			onclick={(e) => e.stopPropagation()}
		>
			<h2 class="mb-4 text-lg font-semibold text-text-primary">Share Your Theme</h2>
			<p class="mb-4 text-sm text-text-muted">
				Share your current theme with the community. Your active CSS variables and custom CSS will be captured.
			</p>

			<div class="mb-4">
				<label for="themeName" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
					Theme Name
				</label>
				<input
					id="themeName"
					type="text"
					class="input w-full"
					bind:value={shareName}
					placeholder="My Awesome Theme"
					maxlength="64"
				/>
			</div>

			<div class="mb-4">
				<label for="themeDesc" class="mb-1 block text-xs font-bold uppercase tracking-wide text-text-muted">
					Description (optional)
				</label>
				<textarea
					id="themeDesc"
					class="input w-full resize-none"
					rows="3"
					bind:value={shareDescription}
					placeholder="A dark theme with purple accents..."
					maxlength="500"
				></textarea>
			</div>

			<div class="flex justify-end gap-2">
				<button class="btn-secondary" onclick={() => (showShareModal = false)}>Cancel</button>
				<button
					class="btn-primary"
					onclick={shareCurrentTheme}
					disabled={sharing || !shareName.trim()}
				>
					{sharing ? 'Sharing...' : 'Share Theme'}
				</button>
			</div>
		</div>
	</div>
{/if}
