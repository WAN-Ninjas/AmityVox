package automod

import (
	"testing"
	"time"
)

func TestCheckWordFilter_Substring(t *testing.T) {
	cfg := RuleConfig{Words: []string{"badword", "evil"}}

	if ok, _ := checkWordFilter("this is a badword test", cfg); !ok {
		t.Error("expected word filter to trigger on substring match")
	}
	if ok, _ := checkWordFilter("this is clean", cfg); ok {
		t.Error("expected word filter to not trigger on clean content")
	}
	if ok, _ := checkWordFilter("BADWORD uppercase", cfg); !ok {
		t.Error("expected case-insensitive match")
	}
}

func TestCheckWordFilter_WholeWord(t *testing.T) {
	cfg := RuleConfig{Words: []string{"bad"}, MatchWholeWord: true}

	if ok, _ := checkWordFilter("this is bad", cfg); !ok {
		t.Error("expected whole word match at end")
	}
	if ok, _ := checkWordFilter("bad start", cfg); !ok {
		t.Error("expected whole word match at start")
	}
	if ok, _ := checkWordFilter("this is badword", cfg); ok {
		t.Error("expected whole word to NOT match substring")
	}
	if ok, _ := checkWordFilter("not a problem", cfg); ok {
		t.Error("expected no match on clean content")
	}
}

func TestCheckRegexFilter(t *testing.T) {
	cfg := RuleConfig{Patterns: []string{`\b\d{16}\b`}} // credit card pattern

	if ok, _ := checkRegexFilter("my card is 1234567890123456 here", cfg); !ok {
		t.Error("expected regex to match 16-digit number")
	}
	if ok, _ := checkRegexFilter("just a normal message", cfg); ok {
		t.Error("expected no match on clean content")
	}
}

func TestCheckRegexFilter_InvalidPattern(t *testing.T) {
	cfg := RuleConfig{Patterns: []string{`[invalid`}}

	if ok, _ := checkRegexFilter("test", cfg); ok {
		t.Error("expected invalid regex to be skipped, not trigger")
	}
}

func TestCheckInviteFilter(t *testing.T) {
	cfg := RuleConfig{}

	if ok, _ := checkInviteFilter("join discord.gg/abc123", cfg, "guild1"); !ok {
		t.Error("expected invite link to trigger")
	}
	if ok, _ := checkInviteFilter("just chatting", cfg, "guild1"); ok {
		t.Error("expected no trigger on clean content")
	}
	if ok, _ := checkInviteFilter("check discordapp.com/invite/xyz", cfg, "guild1"); !ok {
		t.Error("expected discordapp invite to trigger")
	}
}

func TestCheckMentionSpam(t *testing.T) {
	cfg := RuleConfig{MaxMentions: 3}

	msg := "<@user1> <@user2> <@user3>"
	if ok, _ := checkMentionSpam(msg, cfg); !ok {
		t.Error("expected mention spam to trigger with 3 mentions and max 3")
	}

	msg = "<@user1> hello"
	if ok, _ := checkMentionSpam(msg, cfg); ok {
		t.Error("expected no trigger with 1 mention")
	}
}

func TestCheckCapsFilter(t *testing.T) {
	cfg := RuleConfig{MaxCapsPercent: 70, MinLength: 5}

	if ok, _ := checkCapsFilter("THIS IS ALL CAPS MESSAGE", cfg); !ok {
		t.Error("expected caps filter to trigger")
	}
	if ok, _ := checkCapsFilter("this is normal", cfg); ok {
		t.Error("expected no trigger on normal case")
	}
	if ok, _ := checkCapsFilter("HI", cfg); ok {
		t.Error("expected no trigger on short message")
	}
}

func TestCheckLinkFilter_Blocked(t *testing.T) {
	cfg := RuleConfig{BlockedDomains: []string{"malware.com", "spam.org"}}

	if ok, _ := checkLinkFilter("check https://malware.com/payload", cfg); !ok {
		t.Error("expected blocked domain to trigger")
	}
	if ok, _ := checkLinkFilter("check https://safe.com/page", cfg); ok {
		t.Error("expected non-blocked domain to pass")
	}
	if ok, _ := checkLinkFilter("no links here", cfg); ok {
		t.Error("expected no trigger without links")
	}
}

func TestCheckLinkFilter_Allowed(t *testing.T) {
	cfg := RuleConfig{AllowedDomains: []string{"safe.com", "example.org"}}

	if ok, _ := checkLinkFilter("check https://safe.com/page", cfg); ok {
		t.Error("expected allowed domain to pass")
	}
	if ok, _ := checkLinkFilter("check https://evil.com/page", cfg); !ok {
		t.Error("expected non-allowed domain to trigger")
	}
}

func TestCheckLinkFilter_Subdomain(t *testing.T) {
	cfg := RuleConfig{BlockedDomains: []string{"spam.org"}}

	if ok, _ := checkLinkFilter("check https://sub.spam.org/page", cfg); !ok {
		t.Error("expected subdomain of blocked domain to trigger")
	}
}

func TestSpamTracker_RateLimit(t *testing.T) {
	st := NewSpamTracker()
	cfg := RuleConfig{MaxMessages: 3, WindowSeconds: 5, MaxDuplicates: 10}

	for i := 0; i < 3; i++ {
		ok, _ := st.Check("user1", "chan1", "msg", cfg)
		if ok {
			t.Fatalf("triggered on message %d, expected no trigger", i)
		}
	}

	// Fourth message should trigger.
	if ok, reason := st.Check("user1", "chan1", "msg", cfg); !ok {
		t.Error("expected rate limit to trigger on 4th message")
	} else if reason != "message rate exceeded" && reason != "duplicate message spam" {
		t.Errorf("unexpected reason: %s", reason)
	}
}

func TestSpamTracker_Duplicates(t *testing.T) {
	st := NewSpamTracker()
	cfg := RuleConfig{MaxMessages: 100, WindowSeconds: 5, MaxDuplicates: 2}

	st.Check("user1", "chan1", "hello", cfg)
	st.Check("user1", "chan1", "hello", cfg)

	// Third duplicate should trigger.
	ok, reason := st.Check("user1", "chan1", "hello", cfg)
	if !ok {
		t.Error("expected duplicate detection to trigger")
	}
	if reason != "duplicate message spam" {
		t.Errorf("unexpected reason: %s", reason)
	}
}

func TestSpamTracker_Cleanup(t *testing.T) {
	st := NewSpamTracker()
	st.history["user1:chan1"] = []spamEntry{
		{content: "old", ts: time.Now().Add(-10 * time.Minute)},
		{content: "recent", ts: time.Now()},
	}

	st.Cleanup(5 * time.Minute)

	st.mu.Lock()
	entries := st.history["user1:chan1"]
	st.mu.Unlock()

	if len(entries) != 1 {
		t.Errorf("expected 1 entry after cleanup, got %d", len(entries))
	}
	if entries[0].content != "recent" {
		t.Error("expected recent entry to survive cleanup")
	}
}

func TestExtractURLs(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"no links here", 0},
		{"check https://example.com", 1},
		{"two https://a.com and http://b.com links", 2},
		{"trailing https://example.com.", 1},
		{"https://example.com/path?q=1", 1},
	}

	for _, tt := range tests {
		urls := extractURLs(tt.input)
		if len(urls) != tt.expected {
			t.Errorf("extractURLs(%q) = %d urls, want %d", tt.input, len(urls), tt.expected)
		}
	}
}

func TestDomainMatch(t *testing.T) {
	domains := []string{"example.com", "evil.org"}

	if !domainMatch("example.com", domains) {
		t.Error("exact match should work")
	}
	if !domainMatch("sub.example.com", domains) {
		t.Error("subdomain match should work")
	}
	if domainMatch("notexample.com", domains) {
		t.Error("partial domain should not match")
	}
	if !domainMatch("evil.org", domains) {
		t.Error("exact match should work for evil.org")
	}
}

func TestIsExempt(t *testing.T) {
	if !isExempt("chan1", []string{"chan1", "chan2"}) {
		t.Error("expected channel to be exempt")
	}
	if isExempt("chan3", []string{"chan1", "chan2"}) {
		t.Error("expected channel to not be exempt")
	}
	if isExempt("chan1", nil) {
		t.Error("expected nil list to not exempt")
	}
}

func TestHasExemptRole(t *testing.T) {
	if !hasExemptRole([]string{"role1", "role2"}, []string{"role2", "role3"}) {
		t.Error("expected role2 to match")
	}
	if hasExemptRole([]string{"role1"}, []string{"role2", "role3"}) {
		t.Error("expected no match")
	}
}

func TestContainsWholeWord(t *testing.T) {
	tests := []struct {
		text, word string
		expected   bool
	}{
		{"hello world", "hello", true},
		{"say hello!", "hello", true},
		{"helloworld", "hello", false},
		{"prefix-hello-suffix", "hello", true},
		{"", "word", false},
		{"word", "word", true},
	}

	for _, tt := range tests {
		got := containsWholeWord(tt.text, tt.word)
		if got != tt.expected {
			t.Errorf("containsWholeWord(%q, %q) = %v, want %v", tt.text, tt.word, got, tt.expected)
		}
	}
}
