import { describe, it, expect } from 'vitest';

/**
 * Test the markdown parsing logic from MarkdownRenderer.svelte.
 * Since Svelte 5 components can't be rendered in happy-dom (SSR mode),
 * we replicate the pure transformation pipeline here as a standalone function.
 * This matches the exact logic in MarkdownRenderer.svelte's $derived.by().
 */
function escapeHtml(str: string): string {
	return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

function renderMarkdown(content: string): string {
	if (!content) return '';
	let text = escapeHtml(content);

	// Code blocks
	text = text.replace(/```(\w*)\n([\s\S]*?)```/g, (_match, lang, code) => {
		const langAttr = lang ? ` data-lang="${lang}"` : '';
		return `<pre class="my-1 overflow-x-auto rounded bg-bg-primary p-3 text-xs"><code${langAttr}>${code.trimEnd()}</code></pre>`;
	});

	// Inline code
	text = text.replace(
		/`([^`]+)`/g,
		'<code class="rounded bg-bg-primary px-1 py-0.5 text-xs font-mono text-brand-300">$1</code>'
	);

	// Spoiler
	text = text.replace(
		/\|\|([^|]+)\|\|/g,
		'<span class="cursor-pointer rounded bg-bg-modifier px-1 text-transparent hover:text-text-secondary hover:bg-transparent" role="button" tabindex="0">$1</span>'
	);

	// Bold + italic
	text = text.replace(/\*\*\*(.+?)\*\*\*/g, '<strong><em>$1</em></strong>');
	text = text.replace(/___(.+?)___/g, '<strong><em>$1</em></strong>');

	// Bold
	text = text.replace(
		/\*\*(.+?)\*\*/g,
		'<strong class="font-semibold text-text-primary">$1</strong>'
	);
	text = text.replace(
		/__(.+?)__/g,
		'<strong class="font-semibold text-text-primary">$1</strong>'
	);

	// Italic
	text = text.replace(/(?<!\w)\*([^*]+)\*(?!\w)/g, '<em>$1</em>');
	text = text.replace(/(?<!\w)_([^_]+)_(?!\w)/g, '<em>$1</em>');

	// Strikethrough
	text = text.replace(/~~(.+?)~~/g, '<del class="text-text-muted">$1</del>');

	// Block quotes
	text = text.replace(
		/^&gt; (.+)$/gm,
		'<div class="border-l-3 border-text-muted pl-3 text-text-muted">$1</div>'
	);

	// Links
	text = text.replace(
		/\[([^\]]+)\]\(([^)]+)\)/g,
		'<a href="$2" target="_blank" rel="noopener" class="text-text-link hover:underline">$1</a>'
	);
	text = text.replace(
		/(^|[^"=])(https?:\/\/[^\s<]+)/g,
		'$1<a href="$2" target="_blank" rel="noopener" class="text-text-link hover:underline">$2</a>'
	);

	// Unordered lists
	text = text.replace(
		/^(?:- |\* )(.+)$/gm,
		'<li class="ml-4 list-disc text-text-secondary">$1</li>'
	);

	// Ordered lists
	text = text.replace(
		/^\d+\. (.+)$/gm,
		'<li class="ml-4 list-decimal text-text-secondary">$1</li>'
	);

	return text;
}

describe('MarkdownRenderer', () => {
	// --- empty / falsy input ---

	it('returns empty string for empty content', () => {
		expect(renderMarkdown('')).toBe('');
	});

	// --- HTML escaping ---

	it('escapes HTML entities to prevent injection', () => {
		const html = renderMarkdown('<script>alert("xss")</script>');
		expect(html).not.toContain('<script>');
		expect(html).toContain('&lt;script&gt;');
	});

	it('escapes ampersands', () => {
		const html = renderMarkdown('A & B');
		expect(html).toContain('&amp;');
	});

	it('escapes angle brackets in plain text', () => {
		const html = renderMarkdown('1 < 2 and 3 > 2');
		expect(html).toContain('&lt;');
		expect(html).toContain('&gt;');
	});

	// --- Bold ---

	it('renders **bold** as <strong>', () => {
		const html = renderMarkdown('**bold text**');
		expect(html).toContain('<strong');
		expect(html).toContain('bold text');
		expect(html).toContain('</strong>');
	});

	it('renders __bold__ as <strong>', () => {
		const html = renderMarkdown('__bold text__');
		expect(html).toContain('<strong');
		expect(html).toContain('bold text');
		expect(html).toContain('</strong>');
	});

	// --- Italic ---

	it('renders *italic* as <em>', () => {
		const html = renderMarkdown('*italic text*');
		expect(html).toContain('<em>');
		expect(html).toContain('italic text');
		expect(html).toContain('</em>');
	});

	it('renders _italic_ as <em>', () => {
		const html = renderMarkdown('_italic text_');
		expect(html).toContain('<em>');
		expect(html).toContain('italic text');
		expect(html).toContain('</em>');
	});

	// --- Bold + Italic ---

	it('renders ***bold italic*** as <strong><em>', () => {
		const html = renderMarkdown('***bold italic***');
		expect(html).toContain('<strong><em>');
		expect(html).toContain('bold italic');
		expect(html).toContain('</em></strong>');
	});

	// --- Strikethrough ---

	it('renders ~~text~~ as <del>', () => {
		const html = renderMarkdown('~~strikethrough~~');
		expect(html).toContain('<del');
		expect(html).toContain('strikethrough');
		expect(html).toContain('</del>');
	});

	// --- Inline code ---

	it('renders `code` as <code>', () => {
		const html = renderMarkdown('use `console.log` here');
		expect(html).toContain('<code');
		expect(html).toContain('console.log');
		expect(html).toContain('</code>');
	});

	it('wraps inline code content in code tag', () => {
		const html = renderMarkdown('`some code`');
		expect(html).toContain('<code');
		expect(html).toContain('some code');
		expect(html).toContain('</code>');
	});

	// --- Code blocks ---

	it('renders fenced code blocks as <pre><code>', () => {
		const content = '```js\nconst x = 1;\n```';
		const html = renderMarkdown(content);
		expect(html).toContain('<pre');
		expect(html).toContain('<code');
		expect(html).toContain('const x = 1;');
		expect(html).toContain('data-lang="js"');
	});

	it('renders code blocks without a language specifier', () => {
		const content = '```\nhello world\n```';
		const html = renderMarkdown(content);
		expect(html).toContain('<pre');
		expect(html).toContain('<code');
		expect(html).toContain('hello world');
		expect(html).not.toContain('data-lang');
	});

	it('preserves multiline content in code blocks', () => {
		const content = '```\nline1\nline2\nline3\n```';
		const html = renderMarkdown(content);
		expect(html).toContain('line1');
		expect(html).toContain('line2');
		expect(html).toContain('line3');
	});

	// --- Spoiler ---

	it('renders ||spoiler|| as a hidden span', () => {
		const html = renderMarkdown('this is ||secret|| info');
		expect(html).toContain('<span');
		expect(html).toContain('text-transparent');
		expect(html).toContain('secret');
	});

	it('spoiler span has correct interaction attributes', () => {
		const html = renderMarkdown('||hidden||');
		expect(html).toContain('role="button"');
		expect(html).toContain('tabindex="0"');
		expect(html).toContain('cursor-pointer');
	});

	// --- Block quotes ---

	it('renders > text as a bordered block quote div', () => {
		const html = renderMarkdown('> this is a quote');
		expect(html).toContain('<div');
		expect(html).toContain('border-l-3');
		expect(html).toContain('this is a quote');
	});

	it('handles multiple block quote lines independently', () => {
		const html = renderMarkdown('> line one\n> line two');
		const matches = html.match(/<div[^>]*border-l-3/g);
		expect(matches).toHaveLength(2);
	});

	// --- Links ---

	it('renders [text](url) as an anchor tag', () => {
		const html = renderMarkdown('[click here](https://example.com)');
		expect(html).toContain('<a');
		expect(html).toContain('href="https://example.com"');
		expect(html).toContain('click here');
		expect(html).toContain('target="_blank"');
		expect(html).toContain('rel="noopener"');
	});

	it('auto-links plain URLs', () => {
		const html = renderMarkdown('visit https://example.com today');
		expect(html).toContain('<a');
		expect(html).toContain('href="https://example.com"');
	});

	it('auto-links http URLs', () => {
		const html = renderMarkdown('visit http://example.com today');
		expect(html).toContain('<a');
		expect(html).toContain('href="http://example.com"');
	});

	it('does not double-wrap links from [text](url) syntax', () => {
		const html = renderMarkdown('[test](https://example.com)');
		const anchorCount = (html.match(/<a /g) || []).length;
		expect(anchorCount).toBe(1);
	});

	// --- Unordered lists ---

	it('renders - item as <li> with list-disc', () => {
		const html = renderMarkdown('- first item\n- second item');
		expect(html).toContain('<li');
		expect(html).toContain('list-disc');
		expect(html).toContain('first item');
		expect(html).toContain('second item');
	});

	it('renders multiple - items as list', () => {
		const html = renderMarkdown('- alpha\n- beta\n- gamma');
		const matches = html.match(/<li/g);
		expect(matches).toHaveLength(3);
		expect(html).toContain('alpha');
		expect(html).toContain('beta');
		expect(html).toContain('gamma');
	});

	// --- Ordered lists ---

	it('renders 1. item as <li> with list-decimal', () => {
		const html = renderMarkdown('1. first\n2. second\n3. third');
		expect(html).toContain('<li');
		expect(html).toContain('list-decimal');
		expect(html).toContain('first');
		expect(html).toContain('second');
		expect(html).toContain('third');
	});

	// --- Combined formatting ---

	it('handles bold inside a list item', () => {
		const html = renderMarkdown('- **important** item');
		expect(html).toContain('<li');
		expect(html).toContain('<strong');
		expect(html).toContain('important');
	});

	it('handles inline code inside bold text', () => {
		const html = renderMarkdown('use `code` in **bold context**');
		expect(html).toContain('<code');
		expect(html).toContain('<strong');
	});

	it('handles multiple formatting types in a single message', () => {
		const content = '**bold** and *italic* and ~~struck~~ and `code`';
		const html = renderMarkdown(content);
		expect(html).toContain('<strong');
		expect(html).toContain('<em>');
		expect(html).toContain('<del');
		expect(html).toContain('<code');
	});

	// --- Edge cases ---

	it('does not crash on content with only whitespace', () => {
		const html = renderMarkdown('   ');
		expect(html).toBeDefined();
	});

	it('preserves plain text without any markdown', () => {
		const html = renderMarkdown('just some plain text');
		expect(html).toContain('just some plain text');
	});

	it('handles escaped HTML inside code blocks', () => {
		const content = '```\n<div>test</div>\n```';
		const html = renderMarkdown(content);
		expect(html).toContain('&lt;div&gt;');
		expect(html).not.toContain('<div>test</div>');
	});
});
