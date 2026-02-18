// Package mentions extracts user, role, and @here mentions from message content.
// Mention syntax: <@ULID> for users, <@&ULID> for roles, @here for channel-wide pings.
// Mentions inside code blocks (``` ```) and inline code (` `) are ignored.
package mentions

import (
	"regexp"
	"strings"
)

// ParseResult holds the extracted mentions from a message.
type ParseResult struct {
	UserIDs     []string
	RoleIDs     []string
	MentionHere bool
}

var (
	// ULID: 26 uppercase alphanumeric characters (Crockford base32).
	userMentionRe = regexp.MustCompile(`<@([0-9A-Z]{26})>`)
	roleMentionRe = regexp.MustCompile(`<@&([0-9A-Z]{26})>`)
	// Code block and inline code patterns for stripping.
	codeBlockRe = regexp.MustCompile("(?s)```.*?```")
	inlineCodeRe = regexp.MustCompile("`[^`]+`")
)

// Parse extracts mentions from message content, ignoring mentions inside code blocks
// and inline code spans. Results are deduplicated.
func Parse(content string) ParseResult {
	var result ParseResult

	// Strip code blocks and inline code so mentions inside them are ignored.
	stripped := codeBlockRe.ReplaceAllString(content, "")
	stripped = inlineCodeRe.ReplaceAllString(stripped, "")

	// Extract user mentions.
	seen := map[string]bool{}
	for _, match := range userMentionRe.FindAllStringSubmatch(stripped, -1) {
		id := match[1]
		if !seen[id] {
			seen[id] = true
			result.UserIDs = append(result.UserIDs, id)
		}
	}

	// Extract role mentions.
	seenRoles := map[string]bool{}
	for _, match := range roleMentionRe.FindAllStringSubmatch(stripped, -1) {
		id := match[1]
		if !seenRoles[id] {
			seenRoles[id] = true
			result.RoleIDs = append(result.RoleIDs, id)
		}
	}

	// Detect @here (case-sensitive, must be standalone word boundary).
	if strings.Contains(stripped, "@here") {
		result.MentionHere = true
	}

	return result
}
