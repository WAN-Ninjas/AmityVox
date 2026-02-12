<script lang="ts">
	interface Props {
		content: string;
	}

	let { content }: Props = $props();

	// Parse markdown into HTML. Handles: bold, italic, strikethrough, inline code,
	// code blocks, spoilers, block quotes, links, and lists.
	const rendered = $derived.by(() => {
		if (!content) return '';
		let text = escapeHtml(content);

		// Code blocks: ```lang\ncode\n```
		text = text.replace(/```(\w*)\n([\s\S]*?)```/g, (_match, lang, code) => {
			const langAttr = lang ? ` data-lang="${lang}"` : '';
			return `<pre class="my-1 overflow-x-auto rounded bg-bg-primary p-3 text-xs"><code${langAttr}>${code.trimEnd()}</code></pre>`;
		});

		// Inline code: `code`
		text = text.replace(/`([^`]+)`/g, '<code class="rounded bg-bg-primary px-1 py-0.5 text-xs font-mono text-brand-300">$1</code>');

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
