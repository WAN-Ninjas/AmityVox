<script lang="ts">
	import katex from 'katex';

	interface Props {
		content: string;
	}

	let { content }: Props = $props();

	/**
	 * Render a LaTeX formula to HTML using KaTeX.
	 * Returns rendered HTML on success, or a fallback error span on failure.
	 */
	function renderMath(formula: string, displayMode: boolean): string {
		try {
			return katex.renderToString(formula, {
				displayMode,
				throwOnError: true,
				trust: false,
				strict: false
			});
		} catch (_e) {
			// On parse error, show the raw formula with error styling.
			const escaped = escapeHtml(formula);
			const delimiter = displayMode ? '$$' : '$';
			return `<span class="text-red-400" title="KaTeX parse error">${delimiter}${escaped}${delimiter}</span>`;
		}
	}

	// Parse markdown into HTML. Handles: math (KaTeX), bold, italic, strikethrough,
	// inline code, code blocks, spoilers, block quotes, links, and lists.
	const rendered = $derived.by(() => {
		if (!content) return '';

		// --- Phase 1: Extract code blocks and math before HTML escaping ---
		// We need to pull out code blocks and math expressions before escaping,
		// because they need access to the raw content (e.g., < in math formulas).

		const placeholders: string[] = [];
		let placeholderIndex = 0;

		function addPlaceholder(html: string): string {
			const key = `\x00PH${placeholderIndex++}\x00`;
			placeholders.push(html);
			return key;
		}

		let text = content;

		// Extract fenced code blocks first (```...```) — they should not be
		// processed for math or other markdown.
		text = text.replace(/```(\w*)\n([\s\S]*?)```/g, (_match, lang, code) => {
			const langAttr = lang ? ` data-lang="${lang}"` : '';
			const escapedCode = escapeHtml(code.trimEnd());
			return addPlaceholder(
				`<pre class="my-1 overflow-x-auto rounded bg-bg-primary p-3 text-xs"><code${langAttr}>${escapedCode}</code></pre>`
			);
		});

		// Extract inline code (`...`) — should not be processed for math.
		text = text.replace(/`([^`]+)`/g, (_match, code) => {
			const escapedCode = escapeHtml(code);
			return addPlaceholder(
				`<code class="rounded bg-bg-primary px-1 py-0.5 text-xs font-mono text-brand-300">${escapedCode}</code>`
			);
		});

		// Extract display math: $$...$$  (may span multiple lines)
		text = text.replace(/\$\$([\s\S]+?)\$\$/g, (_match, formula) => {
			return addPlaceholder(renderMath(formula, true));
		});

		// Extract inline math: $...$  (single line, non-greedy)
		// Negative lookbehind/lookahead to avoid matching $$, and to avoid
		// matching $ inside words like "price is $5" (require non-digit after opening $).
		text = text.replace(/(?<!\$)\$(?!\$)([^\n$]+?)\$(?!\$)/g, (_match, formula) => {
			return addPlaceholder(renderMath(formula, false));
		});

		// --- Phase 2: Escape HTML in the remaining text ---
		text = escapeHtml(text);

		// --- Phase 3: Standard markdown transformations ---

		// Spoiler: ||text||
		text = text.replace(/\|\|([^|]+)\|\|/g, '<span class="cursor-pointer rounded bg-bg-modifier px-1 text-transparent hover:text-text-secondary hover:bg-transparent" role="button" tabindex="0">$1</span>');

		// Bold + italic: ***text*** or ___text___
		text = text.replace(/\*\*\*(.+?)\*\*\*/g, '<strong><em>$1</em></strong>');
		text = text.replace(/___(.+?)___/g, '<strong><em>$1</em></strong>');

		// Bold: **text** or __text__
		text = text.replace(/\*\*(.+?)\*\*/g, '<strong class="font-semibold text-text-primary">$1</strong>');
		text = text.replace(/__(.+?)__/g, '<strong class="font-semibold text-text-primary">$1</strong>');

		// Italic: *text* or _text_
		text = text.replace(/(?<!\w)\*([^*]+)\*(?!\w)/g, '<em>$1</em>');
		text = text.replace(/(?<!\w)_([^_]+)_(?!\w)/g, '<em>$1</em>');

		// Strikethrough: ~~text~~
		text = text.replace(/~~(.+?)~~/g, '<del class="text-text-muted">$1</del>');

		// Block quotes: > text (at start of line)
		text = text.replace(/^&gt; (.+)$/gm, '<div class="border-l-3 border-text-muted pl-3 text-text-muted">$1</div>');

		// Links: [text](url) or auto-link URLs
		text = text.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener" class="text-text-link hover:underline">$1</a>');
		text = text.replace(/(^|[^"=])(https?:\/\/[^\s<]+)/g, '$1<a href="$2" target="_blank" rel="noopener" class="text-text-link hover:underline">$2</a>');

		// Unordered lists: - item or * item (at start of line)
		text = text.replace(/^(?:- |\* )(.+)$/gm, '<li class="ml-4 list-disc text-text-secondary">$1</li>');

		// Ordered lists: 1. item (at start of line)
		text = text.replace(/^\d+\. (.+)$/gm, '<li class="ml-4 list-decimal text-text-secondary">$1</li>');

		// --- Phase 4: Restore placeholders ---
		text = text.replace(/\x00PH(\d+)\x00/g, (_match, idx) => {
			return placeholders[parseInt(idx, 10)] ?? '';
		});

		return text;
	});

	function escapeHtml(str: string): string {
		return str
			.replace(/&/g, '&amp;')
			.replace(/</g, '&lt;')
			.replace(/>/g, '&gt;');
	}
</script>

{@html rendered}
