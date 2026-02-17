/**
 * Checks if a string contains only emoji characters (and optional whitespace),
 * with at most `maxEmoji` emoji. Used to render emoji-only messages larger.
 */
export function isEmojiOnly(text: string, maxEmoji: number = 10): boolean {
	if (!text || !text.trim()) return false;

	// Match emoji sequences: emoji characters, variation selectors, ZWJ, skin tones,
	// regional indicators (flags), and combining marks.
	const emojiRegex =
		/(?:\p{Regional_Indicator}{2}|\p{Emoji_Presentation}\p{Emoji_Modifier}?(?:\u{FE0F}?\u{20E3})?(?:\u{200D}\p{Emoji_Presentation}\p{Emoji_Modifier}?)*|\p{Emoji}\u{FE0F}\p{Emoji_Modifier}?(?:\u{200D}\p{Emoji_Presentation}\p{Emoji_Modifier}?)*)/gu;

	// Remove all emoji sequences from the string
	const withoutEmoji = text.replace(emojiRegex, '');

	// If anything remains besides whitespace, it's not emoji-only
	if (withoutEmoji.trim().length > 0) return false;

	// Count emoji sequences
	const matches = text.match(emojiRegex);
	if (!matches || matches.length === 0) return false;

	return matches.length <= maxEmoji;
}
