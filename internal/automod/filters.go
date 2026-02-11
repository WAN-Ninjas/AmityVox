package automod

import (
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
)

// --- Word Filter ---

// checkWordFilter checks if the message contains any blocked words.
func checkWordFilter(content string, cfg RuleConfig) (bool, string) {
	if len(cfg.Words) == 0 {
		return false, ""
	}

	lower := strings.ToLower(content)
	for _, word := range cfg.Words {
		word = strings.ToLower(word)
		if cfg.MatchWholeWord {
			if containsWholeWord(lower, word) {
				return true, "blocked word: " + word
			}
		} else {
			if strings.Contains(lower, word) {
				return true, "blocked word: " + word
			}
		}
	}
	return false, ""
}

// containsWholeWord checks for a word bounded by non-alphanumeric characters.
func containsWholeWord(text, word string) bool {
	idx := 0
	for {
		pos := strings.Index(text[idx:], word)
		if pos == -1 {
			return false
		}
		absPos := idx + pos

		// Check left boundary.
		leftOK := absPos == 0 || !isAlphaNum(rune(text[absPos-1]))
		// Check right boundary.
		endPos := absPos + len(word)
		rightOK := endPos >= len(text) || !isAlphaNum(rune(text[endPos]))

		if leftOK && rightOK {
			return true
		}
		idx = absPos + len(word)
		if idx >= len(text) {
			return false
		}
	}
}

func isAlphaNum(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// --- Regex Filter ---

// regexCache caches compiled regexps to avoid recompilation per message.
var (
	regexCache   = map[string]*regexp.Regexp{}
	regexCacheMu sync.RWMutex
)

func getRegexp(pattern string) (*regexp.Regexp, error) {
	regexCacheMu.RLock()
	re, ok := regexCache[pattern]
	regexCacheMu.RUnlock()
	if ok {
		return re, nil
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	regexCacheMu.Lock()
	// Prevent unbounded cache growth.
	if len(regexCache) < 1000 {
		regexCache[pattern] = re
	}
	regexCacheMu.Unlock()

	return re, nil
}

// checkRegexFilter tests the message against configured regex patterns.
func checkRegexFilter(content string, cfg RuleConfig) (bool, string) {
	for _, pattern := range cfg.Patterns {
		re, err := getRegexp(pattern)
		if err != nil {
			continue // Skip invalid patterns.
		}
		if re.MatchString(content) {
			return true, "matched pattern: " + pattern
		}
	}
	return false, ""
}

// --- Invite Filter ---

// Common invite patterns: discord.gg/xxx, invite.gg/xxx, amityvox.example/invite/xxx
var inviteRegex = regexp.MustCompile(`(?i)(?:discord\.gg|discordapp\.com/invite|invite\.gg|amityvox\.[a-z]+/invite)/([A-Za-z0-9_-]+)`)

// checkInviteFilter detects invite links in the message.
func checkInviteFilter(content string, cfg RuleConfig, _ string) (bool, string) {
	matches := inviteRegex.FindAllString(content, -1)
	if len(matches) > 0 {
		return true, "invite link detected"
	}
	return false, ""
}

// --- Mention Spam ---

var mentionRegex = regexp.MustCompile(`<@!?[0-9A-Za-z]+>`)

// checkMentionSpam counts user mentions in the message.
func checkMentionSpam(content string, cfg RuleConfig) (bool, string) {
	maxMentions := cfg.MaxMentions
	if maxMentions <= 0 {
		maxMentions = 5
	}

	mentions := mentionRegex.FindAllString(content, -1)
	if len(mentions) >= maxMentions {
		return true, "too many mentions"
	}
	return false, ""
}

// --- Caps Filter ---

// checkCapsFilter triggers if too many characters are uppercase.
func checkCapsFilter(content string, cfg RuleConfig) (bool, string) {
	minLength := cfg.MinLength
	if minLength <= 0 {
		minLength = 10
	}
	maxPercent := cfg.MaxCapsPercent
	if maxPercent <= 0 {
		maxPercent = 70
	}

	// Only check messages long enough.
	letters := 0
	upper := 0
	for _, r := range content {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsUpper(r) {
				upper++
			}
		}
	}

	if letters < minLength {
		return false, ""
	}

	percent := (upper * 100) / letters
	if percent >= maxPercent {
		return true, "excessive caps"
	}
	return false, ""
}

// --- Spam Filter ---

// SpamTracker tracks recent messages per user for spam detection.
type SpamTracker struct {
	mu      sync.Mutex
	history map[string][]spamEntry // key: "userID:channelID"
}

type spamEntry struct {
	content string
	ts      time.Time
}

// NewSpamTracker creates a new spam tracker.
func NewSpamTracker() *SpamTracker {
	return &SpamTracker{
		history: make(map[string][]spamEntry),
	}
}

// Check evaluates whether a message triggers the spam filter.
func (st *SpamTracker) Check(userID, channelID, content string, cfg RuleConfig) (bool, string) {
	maxMessages := cfg.MaxMessages
	if maxMessages <= 0 {
		maxMessages = 5
	}
	windowSec := cfg.WindowSeconds
	if windowSec <= 0 {
		windowSec = 5
	}
	maxDuplicates := cfg.MaxDuplicates
	if maxDuplicates <= 0 {
		maxDuplicates = 3
	}

	key := userID + ":" + channelID
	now := time.Now()
	window := time.Duration(windowSec) * time.Second

	st.mu.Lock()
	defer st.mu.Unlock()

	// Prune old entries.
	entries := st.history[key]
	cutoff := now.Add(-window)
	pruned := entries[:0]
	for _, e := range entries {
		if e.ts.After(cutoff) {
			pruned = append(pruned, e)
		}
	}

	// Add current message.
	pruned = append(pruned, spamEntry{content: content, ts: now})
	st.history[key] = pruned

	// Check message rate.
	if len(pruned) > maxMessages {
		return true, "message rate exceeded"
	}

	// Check duplicate content.
	duplicates := 0
	lower := strings.ToLower(content)
	for _, e := range pruned {
		if strings.ToLower(e.content) == lower {
			duplicates++
		}
	}
	if duplicates > maxDuplicates {
		return true, "duplicate message spam"
	}

	return false, ""
}

// Cleanup removes stale entries older than the given duration.
func (st *SpamTracker) Cleanup(maxAge time.Duration) {
	st.mu.Lock()
	defer st.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for key, entries := range st.history {
		pruned := entries[:0]
		for _, e := range entries {
			if e.ts.After(cutoff) {
				pruned = append(pruned, e)
			}
		}
		if len(pruned) == 0 {
			delete(st.history, key)
		} else {
			st.history[key] = pruned
		}
	}
}

// --- Link Filter ---

// checkLinkFilter checks URLs in the message against allowed/blocked domain lists.
func checkLinkFilter(content string, cfg RuleConfig) (bool, string) {
	// Simple URL extraction: find http:// or https:// links.
	urls := extractURLs(content)
	if len(urls) == 0 {
		return false, ""
	}

	for _, rawURL := range urls {
		parsed, err := url.Parse(rawURL)
		if err != nil {
			continue
		}
		host := strings.ToLower(parsed.Hostname())
		if host == "" {
			continue
		}

		// If allowed_domains is set, only those domains are allowed.
		if len(cfg.AllowedDomains) > 0 {
			if !domainMatch(host, cfg.AllowedDomains) {
				return true, "link from non-allowed domain: " + host
			}
			continue
		}

		// If blocked_domains is set, those domains are blocked.
		if len(cfg.BlockedDomains) > 0 {
			if domainMatch(host, cfg.BlockedDomains) {
				return true, "link from blocked domain: " + host
			}
		}
	}

	return false, ""
}

// extractURLs finds http/https URLs in text.
func extractURLs(text string) []string {
	var urls []string
	for _, word := range strings.Fields(text) {
		if strings.HasPrefix(word, "http://") || strings.HasPrefix(word, "https://") {
			// Trim trailing punctuation.
			word = strings.TrimRight(word, ".,;:!?)]}>\"'")
			urls = append(urls, word)
		}
	}
	return urls
}

// domainMatch checks if a host matches any domain in the list (supports subdomains).
func domainMatch(host string, domains []string) bool {
	for _, d := range domains {
		d = strings.ToLower(d)
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}
