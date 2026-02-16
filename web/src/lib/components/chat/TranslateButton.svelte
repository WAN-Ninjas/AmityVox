<script lang="ts">
	import { api } from '$lib/api/client';
	import { addToast } from '$lib/stores/toast';

	interface Props {
		channelId: string;
		messageIds: string[];
	}

	let { channelId, messageIds }: Props = $props();

	let loading = $state(false);
	let translations = $state<Array<{ translated_text: string; source_lang: string; target_lang: string }>>([]);
	let showTranslation = $state(false);
	let showLangPicker = $state(false);

	const commonLanguages = [
		{ code: 'en', label: 'English' },
		{ code: 'es', label: 'Spanish' },
		{ code: 'fr', label: 'French' },
		{ code: 'de', label: 'German' },
		{ code: 'it', label: 'Italian' },
		{ code: 'pt', label: 'Portuguese' },
		{ code: 'ru', label: 'Russian' },
		{ code: 'ja', label: 'Japanese' },
		{ code: 'ko', label: 'Korean' },
		{ code: 'zh', label: 'Chinese' },
		{ code: 'ar', label: 'Arabic' },
		{ code: 'hi', label: 'Hindi' },
		{ code: 'nl', label: 'Dutch' },
		{ code: 'pl', label: 'Polish' },
		{ code: 'tr', label: 'Turkish' },
		{ code: 'uk', label: 'Ukrainian' }
	];

	// Load user's preferred language from localStorage.
	function getStoredLang(): string {
		try {
			return localStorage.getItem('av-translate-lang') ?? 'en';
		} catch {
			return 'en';
		}
	}
	let preferredLang = $state(getStoredLang());

	async function translate(lang?: string, force = false) {
		if (messageIds.length === 0) return;
		const target = lang ?? preferredLang;
		loading = true;
		showLangPicker = false;
		try {
			const results = await Promise.all(
				messageIds.map((id) => api.translateMessage(channelId, id, target, force))
			);
			translations = results;
			showTranslation = results.length > 0;

			// Save preferred language.
			localStorage.setItem('av-translate-lang', target);
			preferredLang = target;
		} catch (err: any) {
			const msg = err?.message ?? 'Translation failed';
			addToast(msg, 'error');
		} finally {
			loading = false;
		}
	}

	function hideTranslation() {
		showTranslation = false;
	}

	function toggleLangPicker(e: MouseEvent) {
		e.stopPropagation();
		showLangPicker = !showLangPicker;
	}
</script>

<div class="relative inline-flex items-center">
	{#if !showTranslation}
		<button
			class="flex items-center gap-1 rounded px-1.5 py-0.5 text-2xs text-text-muted hover:bg-bg-modifier hover:text-text-secondary transition-colors"
			onclick={() => translate()}
			disabled={loading}
			title="Translate {messageIds.length > 1 ? `${messageIds.length} messages` : 'message'}"
		>
			{#if loading}
				<div class="h-3 w-3 animate-spin rounded-full border border-text-muted border-t-transparent"></div>
			{:else}
				<svg class="h-3 w-3" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path d="M3 5h12M9 3v2m1.048 9.5A18.022 18.022 0 016.412 9m6.088 9h7M11 21l5-10 5 10M12.751 5C11.783 10.77 8.07 15.61 3 18.129" />
				</svg>
			{/if}
			<span>Translate{messageIds.length > 1 ? ` (${messageIds.length})` : ''}</span>
		</button>
		<button
			class="rounded p-0.5 text-text-muted hover:bg-bg-modifier hover:text-text-secondary transition-colors"
			onclick={toggleLangPicker}
			title="Choose language"
		>
			<svg class="h-2.5 w-2.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path d="M19 9l-7 7-7-7" />
			</svg>
		</button>
	{/if}

	<!-- Language picker dropdown -->
	{#if showLangPicker}
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="absolute left-0 top-full z-20 mt-1 max-h-48 w-40 overflow-y-auto rounded bg-bg-floating shadow-lg border border-bg-modifier"
			onclick={(e) => e.stopPropagation()}
		>
			{#each commonLanguages as lang (lang.code)}
				<button
					class="flex w-full items-center gap-2 px-3 py-1.5 text-xs text-text-secondary hover:bg-bg-modifier transition-colors
						{lang.code === preferredLang ? 'bg-brand-500/10 text-brand-400' : ''}"
					onclick={() => translate(lang.code)}
				>
					<span class="w-5 text-text-muted">{lang.code}</span>
					<span>{lang.label}</span>
				</button>
			{/each}
		</div>
	{/if}
</div>

<!-- Translation results shown below -->
{#if showTranslation && translations.length > 0}
	<div class="mt-1 rounded border-l-2 border-blue-500/50 bg-blue-500/5 px-2.5 py-1.5">
		<div class="mb-0.5 flex items-center gap-2">
			<span class="text-2xs text-blue-400">
				{translations[0].source_lang ?? 'auto'} &rarr; {translations[0].target_lang ?? preferredLang}
				{#if translations.length > 1}
					&middot; {translations.length} messages
				{/if}
			</span>
			<button
				class="text-2xs text-text-muted hover:text-text-secondary"
				onclick={hideTranslation}
			>
				Hide
			</button>
			<button
				class="text-2xs text-text-muted hover:text-text-secondary"
				onclick={() => translate(undefined, true)}
				disabled={loading}
			>
				{loading ? 'Retrying...' : 'Retry'}
			</button>
			<button
				class="text-2xs text-text-muted hover:text-text-secondary"
				onclick={toggleLangPicker}
			>
				Change language
			</button>
		</div>
		{#each translations as t, i}
			{#if i > 0}
				<div class="my-1 border-t border-blue-500/20"></div>
			{/if}
			<p class="text-sm text-text-secondary leading-relaxed break-words">{t.translated_text}</p>
		{/each}
	</div>
{/if}
