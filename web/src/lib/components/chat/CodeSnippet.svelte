<script lang="ts">
	import { untrack } from 'svelte';
	import { api, type CodeSnippet as CodeSnippetData } from '$lib/api/client';
	import { createAsyncOp } from '$lib/utils/asyncOp';

	interface Props {
		channelId: string;
		snippet?: CodeSnippetData;
		onclose?: () => void;
		oncreated?: (snippet: CodeSnippetData) => void;
	}

	let { channelId, snippet, onclose, oncreated }: Props = $props();

	let title = $state('');
	let language = $state('javascript');
	let code = $state('');
	let createOp = $state(createAsyncOp());
	let copied = $state(false);
	let showCreateForm = $state(untrack(() => !snippet));
	let displayCode = $derived(snippet?.code ?? code);
	let displayLanguage = $derived(snippet?.language ?? language);
	let lineNumbers = $derived(displayCode.split('\n').map((_, i) => i + 1));

	const languages = [
		'javascript', 'typescript', 'python', 'go', 'rust', 'java', 'c', 'cpp',
		'csharp', 'ruby', 'php', 'swift', 'kotlin', 'html', 'css', 'sql',
		'bash', 'json', 'yaml', 'toml', 'markdown', 'plaintext'
	];

	function highlightCode(source: string, lang: string): string {
		let escaped = source.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
		escaped = escaped.replace(/(["'`])(?:(?=(\\?))\2.)*?\1/g, '<span class="text-green-400">$&</span>');
		escaped = escaped.replace(/(\/\/.*$|#.*$)/gm, '<span class="text-text-muted italic">$&</span>');
		escaped = escaped.replace(/\b(\d+\.?\d*)\b/g, '<span class="text-orange-400">$1</span>');
		const keywords: Record<string, string[]> = {
			javascript: ['const', 'let', 'var', 'function', 'return', 'if', 'else', 'for', 'while', 'class', 'import', 'export', 'async', 'await'],
			typescript: ['const', 'let', 'var', 'function', 'return', 'if', 'else', 'interface', 'type', 'class', 'import', 'export', 'async', 'await'],
			python: ['def', 'class', 'import', 'from', 'return', 'if', 'elif', 'else', 'for', 'while', 'True', 'False', 'None'],
			go: ['func', 'package', 'import', 'return', 'if', 'else', 'for', 'range', 'type', 'struct', 'interface', 'nil'],
			rust: ['fn', 'let', 'mut', 'const', 'use', 'struct', 'enum', 'impl', 'trait', 'return', 'match'],
		};
		const words = keywords[lang] ?? [];
		if (words.length > 0) {
			escaped = escaped.replace(new RegExp(`\\b(${words.join('|')})\\b`, 'g'), '<span class="text-purple-400 font-medium">$1</span>');
		}
		return escaped;
	}

	async function createSnippet() {
		if (!code.trim()) {
			createOp.error = 'Code content is required';
			return;
		}
		const created = await createOp.run(() => api.createCodeSnippet(channelId, {
			title: title || undefined,
			language,
			code
		}));
		if (created) {
			oncreated?.(created);
			onclose?.();
		}
	}

	async function copyCode() {
		try {
			await navigator.clipboard.writeText(displayCode);
			copied = true;
			setTimeout(() => (copied = false), 2000);
		} catch {
			createOp.error = 'Failed to copy code';
		}
	}
</script>

{#if showCreateForm}
	<div class="overflow-hidden rounded-lg border border-border-primary bg-bg-secondary">
		<div class="flex items-center justify-between border-b border-border-primary bg-bg-tertiary px-3 py-2">
			<span class="text-sm font-medium text-text-primary">New Code Snippet</span>
			{#if onclose}
				<button type="button" class="text-text-muted hover:text-text-primary" onclick={onclose} aria-label="Close code snippet form">x</button>
			{/if}
		</div>
		<div class="space-y-3 p-3">
			{#if createOp.error}
				<div class="rounded border border-red-500/20 bg-red-500/10 p-2 text-sm text-red-400">{createOp.error}</div>
			{/if}
			<div class="flex gap-2">
				<input class="input flex-1" placeholder="Snippet title (optional)" bind:value={title} />
				<select class="input w-40" bind:value={language}>
					{#each languages as lang}
						<option value={lang}>{lang}</option>
					{/each}
				</select>
			</div>
			<textarea class="h-48 w-full resize-none rounded border border-border-primary bg-bg-primary p-3 font-mono text-sm text-text-primary outline-none focus:border-brand-500" placeholder="Paste your code here..." bind:value={code} spellcheck="false"></textarea>
			<div class="flex justify-end gap-2">
				{#if onclose}
					<button type="button" class="btn-secondary text-sm" onclick={onclose}>Cancel</button>
				{/if}
				<button type="button" class="btn-primary text-sm" disabled={createOp.loading || !code.trim()} onclick={createSnippet}>
					{createOp.loading ? 'Sharing...' : 'Share Code'}
				</button>
			</div>
		</div>
	</div>
{:else if snippet}
	<div class="max-w-2xl overflow-hidden rounded-lg border border-border-primary bg-bg-secondary">
		<div class="flex items-center justify-between border-b border-border-primary bg-bg-tertiary px-3 py-2">
			<div class="flex min-w-0 items-center gap-2">
				<span class="rounded bg-brand-500/20 px-1.5 py-0.5 font-mono text-xs text-brand-400">{displayLanguage}</span>
				{#if snippet.title}
					<span class="truncate text-sm font-medium text-text-primary">{snippet.title}</span>
				{/if}
			</div>
			<button type="button" class="rounded p-1 text-text-muted hover:bg-bg-primary hover:text-text-primary" title={copied ? 'Copied!' : 'Copy code'} onclick={copyCode}>
				{copied ? 'Copied' : 'Copy'}
			</button>
		</div>
		<div class="overflow-x-auto">
			<div class="flex font-mono text-sm">
				<div class="shrink-0 select-none border-r border-border-primary bg-bg-tertiary/50 px-2 py-3 text-right text-text-muted">
					{#each lineNumbers as num}
						<div class="leading-5">{num}</div>
					{/each}
				</div>
				<pre class="flex-1 overflow-x-auto px-3 py-3 leading-5 text-text-primary"><code>{@html highlightCode(displayCode, displayLanguage)}</code></pre>
			</div>
		</div>
	</div>
{/if}
