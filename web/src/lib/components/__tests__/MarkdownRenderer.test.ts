import { describe, it, expect } from 'vitest';

/**
 * Test the markdown parsing logic from MarkdownRenderer.svelte.
 * Since Svelte 5 components can't be rendered in happy-dom (SSR mode),
 * we replicate the pure transformation pipeline here as a standalone function.
 * This matches the exact logic in MarkdownRenderer.svelte's $derived.by().
 */

// Mock katex for unit tests — we don't need the full rendering engine,
// just verify that the right formulas are detected and passed through.
const mockKatex = {
	renderToString(formula: string, opts: { displayMode: boolean; throwOnError: boolean }) {
		// Simulate KaTeX output: wrap in a span with class "katex" or "katex-display".
		if (formula === 'INVALID') throw new Error('KaTeX parse error');
		const cls = opts.displayMode ? 'katex-display' : 'katex';
		return `<span class="${cls}">${formula}</span>`;
	}
};

function escapeHtml(str: string): string {
	return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

function renderMath(formula: string, displayMode: boolean): string {
	try {
		return mockKatex.renderToString(formula, {
			displayMode,
			throwOnError: true
		});
	} catch (_e) {
		const escaped = escapeHtml(formula);
		const delimiter = displayMode ? '$$' : '$';
		return `<span class="text-red-400" title="KaTeX parse error">${delimiter}${escaped}${delimiter}</span>`;
	}
}

interface MemberLike {
	user_id: string;
	nickname?: string | null;
	user?: { username?: string; display_name?: string | null } | null;
}

interface RoleLike {
	id: string;
	name: string;
	color?: string | null;
}

function renderMarkdown(
	content: string,
	members?: Map<string, MemberLike>,
	roles?: Map<string, RoleLike>
): string {
	if (!content) return '';

	const placeholders: string[] = [];
	let placeholderIndex = 0;

	function addPlaceholder(html: string): string {
		const key = `\x00PH${placeholderIndex++}\x00`;
		placeholders.push(html);
		return key;
	}

	let text = content;

	// Code blocks
	text = text.replace(/```(\w*)\n([\s\S]*?)```/g, (_match, lang, code) => {
		const langAttr = lang ? ` data-lang="${lang}"` : '';
		const escapedCode = escapeHtml(code.trimEnd());
		return addPlaceholder(
			`<pre class="my-1 overflow-x-auto rounded bg-bg-primary p-3 text-xs"><code${langAttr}>${escapedCode}</code></pre>`
		);
	});

	// Inline code
	text = text.replace(/`([^`]+)`/g, (_match, code) => {
		const escapedCode = escapeHtml(code);
		return addPlaceholder(
			`<code class="rounded bg-bg-primary px-1 py-0.5 text-xs font-mono text-brand-300">${escapedCode}</code>`
		);
	});

	// Display math: $$...$$
	text = text.replace(/\$\$([\s\S]+?)\$\$/g, (_match, formula) => {
		return addPlaceholder(renderMath(formula, true));
	});

	// Inline math: $...$
	text = text.replace(/(?<!\$)\$(?!\$)([^\n$]+?)\$(?!\$)/g, (_match, formula) => {
		return addPlaceholder(renderMath(formula, false));
	});

	// Mentions: user <@ULID>, role <@&ULID>, @here
	text = text.replace(/<@([0-9A-Z]{26})>/g, (_match, userId) => {
		const member = members?.get(userId);
		const name = member?.nickname ?? member?.user?.display_name ?? member?.user?.username ?? userId.slice(0, 8);
		return addPlaceholder(
			`<span class="inline-block rounded bg-brand-500/20 px-1 py-0.5 text-xs font-medium text-brand-300 cursor-pointer hover:bg-brand-500/30">@${escapeHtml(name)}</span>`
		);
	});

	text = text.replace(/<@&([0-9A-Z]{26})>/g, (_match, roleId) => {
		const role = roles?.get(roleId);
		const name = role?.name ?? 'Unknown Role';
		const color = role?.color ?? '#99aab5';
		return addPlaceholder(
			`<span class="inline-block rounded px-1 py-0.5 text-xs font-medium cursor-pointer" style="background-color: ${escapeHtml(color)}20; color: ${escapeHtml(color)}">@${escapeHtml(name)}</span>`
		);
	});

	text = text.replace(/@here/g, () => {
		return addPlaceholder(
			`<span class="inline-block rounded bg-yellow-500/20 px-1 py-0.5 text-xs font-medium text-yellow-300 cursor-pointer hover:bg-yellow-500/30">@here</span>`
		);
	});

	// Escape HTML
	text = escapeHtml(text);

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

	// Restore placeholders
	text = text.replace(/\x00PH(\d+)\x00/g, (_match, idx) => {
		return placeholders[parseInt(idx, 10)] ?? '';
	});

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

	// --- KaTeX / Math Rendering ---

	describe('inline math ($...$)', () => {
		it('renders inline math with single dollar signs', () => {
			const html = renderMarkdown('The formula $E=mc^2$ is famous');
			expect(html).toContain('class="katex"');
			expect(html).toContain('E=mc^2');
		});

		it('renders multiple inline math expressions', () => {
			const html = renderMarkdown('$a^2$ plus $b^2$ equals $c^2$');
			const matches = html.match(/class="katex"/g);
			expect(matches).toHaveLength(3);
		});

		it('does not match double dollar signs as inline math', () => {
			const html = renderMarkdown('$$x^2$$');
			// Should be display math, not inline
			expect(html).toContain('class="katex-display"');
			expect(html).not.toContain('class="katex"');
		});

		it('does not process math inside inline code', () => {
			const html = renderMarkdown('use `$variable$` in code');
			expect(html).toContain('<code');
			expect(html).toContain('$variable$');
			expect(html).not.toContain('class="katex"');
		});

		it('does not process math inside code blocks', () => {
			const html = renderMarkdown('```\n$x^2$\n```');
			expect(html).toContain('<pre');
			expect(html).toContain('$x^2$');
			expect(html).not.toContain('class="katex"');
		});
	});

	describe('display math ($$...$$)', () => {
		it('renders display math with double dollar signs', () => {
			const html = renderMarkdown('$$\\int_0^1 x^2 dx$$');
			expect(html).toContain('class="katex-display"');
			expect(html).toContain('\\int_0^1 x^2 dx');
		});

		it('renders multiline display math', () => {
			const html = renderMarkdown('$$\nx^2 + y^2 = z^2\n$$');
			expect(html).toContain('class="katex-display"');
			expect(html).toContain('x^2 + y^2 = z^2');
		});

		it('renders display math alongside text', () => {
			const html = renderMarkdown('Consider the equation:\n$$a^2 + b^2 = c^2$$\nwhich is well known.');
			expect(html).toContain('class="katex-display"');
			expect(html).toContain('Consider the equation:');
			expect(html).toContain('which is well known.');
		});
	});

	describe('math error handling', () => {
		it('shows raw formula with error styling on parse failure', () => {
			const html = renderMarkdown('$INVALID$');
			expect(html).toContain('text-red-400');
			expect(html).toContain('KaTeX parse error');
			expect(html).toContain('$INVALID$');
		});

		it('shows raw display formula with error styling on parse failure', () => {
			const html = renderMarkdown('$$INVALID$$');
			expect(html).toContain('text-red-400');
			expect(html).toContain('KaTeX parse error');
			expect(html).toContain('$$INVALID$$');
		});
	});

	describe('math with other markdown', () => {
		it('renders math alongside bold text', () => {
			const html = renderMarkdown('**Theorem:** $a^2 + b^2 = c^2$');
			expect(html).toContain('<strong');
			expect(html).toContain('Theorem:');
			expect(html).toContain('class="katex"');
		});

		it('renders math alongside italic text', () => {
			const html = renderMarkdown('*Note:* $x = 5$');
			expect(html).toContain('<em>');
			expect(html).toContain('class="katex"');
		});

		it('does not apply markdown formatting inside math', () => {
			// The underscores in x_1 should NOT become italic
			const html = renderMarkdown('$x_1 + x_2$');
			expect(html).toContain('class="katex"');
			expect(html).toContain('x_1 + x_2');
			// Should not contain <em> generated from the underscores
			expect(html).not.toContain('<em>1 + x</em>');
		});
	});

	// --- Mention Rendering ---

	describe('mention rendering', () => {
		const testMembers = new Map<string, MemberLike>([
			['01ARZ3NDEKTSV4RRFFQ69G5FAV', {
				user_id: '01ARZ3NDEKTSV4RRFFQ69G5FAV',
				nickname: 'CoolNick',
				user: { username: 'testuser', display_name: 'Test User' }
			}],
			['01ARZ3NDEKTSV4RRFFQ69G5FAW', {
				user_id: '01ARZ3NDEKTSV4RRFFQ69G5FAW',
				nickname: null,
				user: { username: 'another', display_name: null }
			}]
		]);

		const testRoles = new Map<string, RoleLike>([
			['01ARZ3NDEKTSV4RRFFQ69G5FAX', { id: '01ARZ3NDEKTSV4RRFFQ69G5FAX', name: 'Moderator', color: '#e74c3c' }],
			['01ARZ3NDEKTSV4RRFFQ69G5FAY', { id: '01ARZ3NDEKTSV4RRFFQ69G5FAY', name: 'Admin', color: null }]
		]);

		it('renders user mention as styled pill with nickname', () => {
			const html = renderMarkdown('hello <@01ARZ3NDEKTSV4RRFFQ69G5FAV>!', testMembers, testRoles);
			expect(html).toContain('@CoolNick');
			expect(html).toContain('bg-brand-500/20');
			expect(html).toContain('text-brand-300');
		});

		it('renders user mention with username fallback when no nickname', () => {
			const html = renderMarkdown('<@01ARZ3NDEKTSV4RRFFQ69G5FAW>', testMembers, testRoles);
			expect(html).toContain('@another');
		});

		it('renders unknown user mention with truncated ID', () => {
			const html = renderMarkdown('<@01ARZ3NDEKTSV4RRFFQ69G5FBB>', testMembers, testRoles);
			expect(html).toContain('@01ARZ3ND');
		});

		it('renders role mention with role name and color', () => {
			const html = renderMarkdown('<@&01ARZ3NDEKTSV4RRFFQ69G5FAX>', testMembers, testRoles);
			expect(html).toContain('@Moderator');
			expect(html).toContain('#e74c3c');
		});

		it('renders role mention with default color when role has null color', () => {
			const html = renderMarkdown('<@&01ARZ3NDEKTSV4RRFFQ69G5FAY>', testMembers, testRoles);
			expect(html).toContain('@Admin');
			expect(html).toContain('#99aab5');
		});

		it('renders unknown role mention', () => {
			const html = renderMarkdown('<@&01ARZ3NDEKTSV4RRFFQ69G5FBC>', testMembers, testRoles);
			expect(html).toContain('@Unknown Role');
		});

		it('renders @here as styled yellow pill', () => {
			const html = renderMarkdown('attention @here');
			expect(html).toContain('@here');
			expect(html).toContain('bg-yellow-500/20');
			expect(html).toContain('text-yellow-300');
		});

		it('does not render user mention inside code block', () => {
			const html = renderMarkdown('```\n<@01ARZ3NDEKTSV4RRFFQ69G5FAV>\n```', testMembers, testRoles);
			expect(html).not.toContain('@CoolNick');
			expect(html).toContain('&lt;@01ARZ3NDEKTSV4RRFFQ69G5FAV&gt;');
		});

		it('does not render user mention inside inline code', () => {
			const html = renderMarkdown('use `<@01ARZ3NDEKTSV4RRFFQ69G5FAV>` syntax', testMembers, testRoles);
			expect(html).not.toContain('@CoolNick');
		});

		it('does not render @here inside code block', () => {
			const html = renderMarkdown('```\n@here\n```');
			expect(html).not.toContain('bg-yellow-500/20');
		});

		it('renders mention alongside other markdown', () => {
			const html = renderMarkdown('**hey** <@01ARZ3NDEKTSV4RRFFQ69G5FAV>!', testMembers, testRoles);
			expect(html).toContain('<strong');
			expect(html).toContain('@CoolNick');
		});
	});
});
