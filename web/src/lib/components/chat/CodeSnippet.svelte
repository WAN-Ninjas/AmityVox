<!-- CodeSnippet.svelte — Code sharing with syntax highlighting and a Run button. -->
<script lang="ts">
	import { api } from '$lib/api/client';
	import { createAsyncOp } from '$lib/utils/asyncOp';

	interface CodeSnippetData {
		id: string;
		channel_id: string;
		author_id: string;
		title?: string;
		language: string;
		code: string;
		stdin?: string;
		output?: string;
		output_error?: string;
		exit_code?: number;
		runtime_ms?: number;
		runnable: boolean;
	}

	interface Props {
		channelId: string;
		snippet?: CodeSnippetData;
		onclose?: () => void;
	}

	let { channelId, snippet, onclose }: Props = $props();

	// Create mode state.
	let title = $state('');
	let language = $state('javascript');
	let code = $state('');
	let stdin = $state('');
	let runnable = $state(false);
	let createOp = $state(createAsyncOp());
	let runOp = $state(createAsyncOp());
	let output = $state<string | null>(snippet?.output ?? null);
	let outputError = $state<string | null>(snippet?.output_error ?? null);
	let exitCode = $state<number | null>(snippet?.exit_code ?? null);
	let runtimeMs = $state<number | null>(snippet?.runtime_ms ?? null);
	let copied = $state(false);
	let showCreateForm = $state(!snippet);
	let lineNumbers = $derived(
		(snippet?.code ?? code).split('\n').map((_, i) => i + 1)
	);

	const languages = [
		{ value: 'javascript', label: 'JavaScript' },
		{ value: 'typescript', label: 'TypeScript' },
		{ value: 'python', label: 'Python' },
		{ value: 'go', label: 'Go' },
		{ value: 'rust', label: 'Rust' },
		{ value: 'java', label: 'Java' },
		{ value: 'c', label: 'C' },
		{ value: 'cpp', label: 'C++' },
		{ value: 'csharp', label: 'C#' },
		{ value: 'ruby', label: 'Ruby' },
		{ value: 'php', label: 'PHP' },
		{ value: 'swift', label: 'Swift' },
		{ value: 'kotlin', label: 'Kotlin' },
		{ value: 'html', label: 'HTML' },
		{ value: 'css', label: 'CSS' },
		{ value: 'sql', label: 'SQL' },
		{ value: 'bash', label: 'Bash' },
		{ value: 'json', label: 'JSON' },
		{ value: 'yaml', label: 'YAML' },
		{ value: 'toml', label: 'TOML' },
		{ value: 'markdown', label: 'Markdown' },
		{ value: 'plaintext', label: 'Plain Text' }
	];

	// Simple keyword-based syntax highlighting.
	function highlightCode(source: string, lang: string): string {
		let escaped = source
			.replace(/&/g, '&amp;')
			.replace(/</g, '&lt;')
			.replace(/>/g, '&gt;');

		// String literals (single and double quotes).
		escaped = escaped.replace(
			/(["'`])(?:(?=(\\?))\2.)*?\1/g,
			'<span class="text-green-400">$&</span>'
		);

		// Comments (// and #).
		escaped = escaped.replace(
			/(\/\/.*$|#.*$)/gm,
			'<span class="text-text-muted italic">$&</span>'
		);

		// Numbers.
		escaped = escaped.replace(
			/\b(\d+\.?\d*)\b/g,
			'<span class="text-orange-400">$1</span>'
		);

		// Language keywords.
		const keywords: Record<string, string[]> = {
			javascript: ['const', 'let', 'var', 'function', 'return', 'if', 'else', 'for', 'while', 'class', 'import', 'export', 'from', 'async', 'await', 'new', 'this', 'true', 'false', 'null', 'undefined', 'try', 'catch', 'throw'],
			typescript: ['const', 'let', 'var', 'function', 'return', 'if', 'else', 'for', 'while', 'class', 'import', 'export', 'from', 'async', 'await', 'new', 'this', 'true', 'false', 'null', 'undefined', 'interface', 'type', 'enum'],
			python: ['def', 'class', 'import', 'from', 'return', 'if', 'elif', 'else', 'for', 'while', 'in', 'not', 'and', 'or', 'True', 'False', 'None', 'with', 'as', 'try', 'except', 'raise', 'pass', 'lambda', 'yield'],
			go: ['func', 'package', 'import', 'return', 'if', 'else', 'for', 'range', 'switch', 'case', 'default', 'var', 'const', 'type', 'struct', 'interface', 'map', 'chan', 'go', 'defer', 'select', 'true', 'false', 'nil'],
			rust: ['fn', 'let', 'mut', 'const', 'use', 'mod', 'pub', 'struct', 'enum', 'impl', 'trait', 'return', 'if', 'else', 'for', 'while', 'match', 'self', 'true', 'false', 'Some', 'None', 'Ok', 'Err', 'async', 'await'],
		};

		const langKw = keywords[lang] ?? keywords['javascript'] ?? [];
		if (langKw.length > 0) {
			const kwRegex = new RegExp(`\\b(${langKw.join('|')})\\b`, 'g');
			escaped = escaped.replace(kwRegex, '<span class="text-purple-400 font-medium">$1</span>');
		}

		return escaped;
	}

	async function createSnippet() {
		if (!code.trim()) {
			createOp.error = 'Code content is required';
			return;
		}
		await createOp.run(() => api.request('POST', `/channels/${channelId}/experimental/code-snippets`, {
			title: title || undefined,
			language,
			code,
			stdin: stdin || undefined,
			runnable
		}));
		if (!createOp.error) {
			if (onclose) onclose();
		}
	}

	async function runSnippet() {
		if (!snippet) return;
		const result = await runOp.run(() => api.request<{
			output: string;
			output_error?: string;
			exit_code: number;
			runtime_ms: number;
		}>('POST', `/channels/${channelId}/experimental/code-snippets/${snippet.id}/run`));
		if (!runOp.error && result) {
			output = result.output;
			outputError = result.output_error ?? null;
			exitCode = result.exit_code;
			runtimeMs = result.runtime_ms;
		}
	}

	async function copyCode() {
		try {
			await navigator.clipboard.writeText(snippet?.code ?? code);
			copied = true;
			setTimeout(() => (copied = false), 2000);
		} catch {
			// Fallback: select the code text.
		}
	}
</script>

{#if showCreateForm}
	<!-- Create code snippet form -->
	<div class="bg-bg-secondary border border-border-primary rounded-lg overflow-hidden">
		<div class="flex items-center justify-between px-3 py-2 bg-bg-tertiary border-b border-border-primary">
			<span class="text-text-primary text-sm font-medium">New Code Snippet</span>
			{#if onclose}
				<button type="button" class="text-text-muted hover:text-text-primary" onclick={onclose}>
					<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			{/if}
		</div>

		<div class="p-3 space-y-3">
			{#if createOp.error}
				<div class="p-2 bg-red-500/10 border border-red-500/20 rounded text-red-400 text-sm">{createOp.error}</div>
			{/if}

			<div class="flex gap-2">
				<input
					type="text"
					class="flex-1 bg-bg-primary border border-border-primary rounded px-2 py-1.5 text-sm text-text-primary placeholder:text-text-muted focus:border-brand-500 focus:outline-none"
					placeholder="Snippet title (optional)"
					bind:value={title}
				/>
				<select
					class="bg-bg-primary border border-border-primary rounded px-2 py-1.5 text-sm text-text-primary focus:border-brand-500 focus:outline-none"
					bind:value={language}
				>
					{#each languages as lang}
						<option value={lang.value}>{lang.label}</option>
					{/each}
				</select>
			</div>

			<div class="relative">
				<textarea
					class="w-full h-48 bg-bg-primary border border-border-primary rounded p-3 text-sm font-mono text-text-primary placeholder:text-text-muted focus:border-brand-500 focus:outline-none resize-none"
					placeholder="Paste your code here..."
					bind:value={code}
					spellcheck="false"
				></textarea>
			</div>

			<details class="text-sm">
				<summary class="text-text-secondary cursor-pointer hover:text-text-primary">Advanced options</summary>
				<div class="mt-2 space-y-2">
					<textarea
						class="w-full h-16 bg-bg-primary border border-border-primary rounded p-2 text-sm font-mono text-text-primary placeholder:text-text-muted focus:border-brand-500 focus:outline-none resize-none"
						placeholder="Standard input (stdin)"
						bind:value={stdin}
					></textarea>
					<label class="flex items-center gap-2 text-text-secondary">
						<input type="checkbox" bind:checked={runnable} class="rounded" />
						Allow server-side execution (sandbox)
					</label>
				</div>
			</details>

			<div class="flex justify-end gap-2">
				{#if onclose}
					<button type="button" class="btn-secondary text-sm px-3 py-1.5 rounded" onclick={onclose}>Cancel</button>
				{/if}
				<button
					type="button"
					class="btn-primary text-sm px-3 py-1.5 rounded"
					disabled={createOp.loading || !code.trim()}
					onclick={createSnippet}
				>
					{createOp.loading ? 'Sharing...' : 'Share Code'}
				</button>
			</div>
		</div>
	</div>
{:else if snippet}
	<!-- Display code snippet -->
	<div class="bg-bg-secondary border border-border-primary rounded-lg overflow-hidden max-w-2xl">
		<!-- Header -->
		<div class="flex items-center justify-between px-3 py-2 bg-bg-tertiary border-b border-border-primary">
			<div class="flex items-center gap-2">
				<span class="px-1.5 py-0.5 bg-brand-500/20 text-brand-400 text-xs rounded font-mono">
					{snippet.language}
				</span>
				{#if snippet.title}
					<span class="text-text-primary text-sm font-medium">{snippet.title}</span>
				{/if}
			</div>
			<div class="flex items-center gap-1">
				<button
					type="button"
					class="text-text-muted hover:text-text-primary p-1 rounded hover:bg-bg-primary transition-colors"
					title={copied ? 'Copied!' : 'Copy code'}
					onclick={copyCode}
				>
					{#if copied}
						<svg class="w-4 h-4 text-green-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
							<path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
						</svg>
					{:else}
						<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
							<path stroke-linecap="round" stroke-linejoin="round" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
						</svg>
					{/if}
				</button>
				{#if snippet.runnable}
					<button
						type="button"
						class="flex items-center gap-1 text-sm px-2 py-0.5 rounded bg-green-500/20 text-green-400 hover:bg-green-500/30 transition-colors"
						disabled={runOp.loading}
						onclick={runSnippet}
					>
						{#if runOp.loading}
							<svg class="w-3 h-3 animate-spin" fill="none" viewBox="0 0 24 24">
								<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
								<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
							</svg>
							Running...
						{:else}
							<svg class="w-3 h-3" fill="currentColor" viewBox="0 0 24 24">
								<path d="M8 5v14l11-7z" />
							</svg>
							Run
						{/if}
					</button>
				{/if}
			</div>
		</div>

		<!-- Code content -->
		<div class="overflow-x-auto">
			<div class="flex text-sm font-mono">
				<!-- Line numbers -->
				<div class="shrink-0 py-3 px-2 text-right text-text-muted select-none border-r border-border-primary bg-bg-tertiary/50">
					{#each lineNumbers as num}
						<div class="leading-5">{num}</div>
					{/each}
				</div>
				<!-- Code -->
				<pre class="flex-1 py-3 px-3 text-text-primary leading-5 overflow-x-auto"><code>{@html highlightCode(snippet.code, snippet.language)}</code></pre>
			</div>
		</div>

		<!-- Output (if run) -->
		{#if output !== null || outputError}
			<div class="border-t border-border-primary">
				<div class="flex items-center gap-2 px-3 py-1.5 bg-bg-tertiary">
					<span class="text-text-muted text-xs font-medium">Output</span>
					{#if exitCode !== null}
						<span class="text-xs {exitCode === 0 ? 'text-green-400' : 'text-red-400'}">
							Exit: {exitCode}
						</span>
					{/if}
					{#if runtimeMs !== null}
						<span class="text-text-muted text-xs">{runtimeMs}ms</span>
					{/if}
				</div>
				<pre class="p-3 text-sm font-mono text-text-secondary overflow-x-auto max-h-48">{output || ''}{#if outputError}<span class="text-red-400">{outputError}</span>{/if}</pre>
			</div>
		{/if}

		{#if runOp.error}
			<div class="p-2 m-2 bg-red-500/10 border border-red-500/20 rounded text-red-400 text-sm">{runOp.error}</div>
		{/if}
	</div>
{/if}
