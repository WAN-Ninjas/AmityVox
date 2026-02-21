package search

import (
	"encoding/json"
	"testing"
)

func TestIndexConstants(t *testing.T) {
	indexes := map[string]string{
		"messages": IndexMessages,
		"users":    IndexUsers,
		"guilds":   IndexGuilds,
		"channels": IndexChannels,
	}

	for expected, actual := range indexes {
		if actual != expected {
			t.Errorf("index constant = %q, want %q", actual, expected)
		}
	}
}

func TestMessageDoc_JSON(t *testing.T) {
	doc := MessageDoc{
		ID:        "msg_001",
		ChannelID: "ch_001",
		GuildID:   "guild_001",
		AuthorID:  "user_001",
		Content:   "hello world",
		CreatedAt: 1707566400,
	}

	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded MessageDoc
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.ID != doc.ID {
		t.Errorf("id = %q, want %q", decoded.ID, doc.ID)
	}
	if decoded.Content != doc.Content {
		t.Errorf("content = %q, want %q", decoded.Content, doc.Content)
	}
	if decoded.CreatedAt != doc.CreatedAt {
		t.Errorf("created_at = %d, want %d", decoded.CreatedAt, doc.CreatedAt)
	}
}

func TestMessageDoc_OmitEmptyGuildID(t *testing.T) {
	doc := MessageDoc{
		ID:        "msg_dm",
		ChannelID: "ch_dm",
		AuthorID:  "user_001",
		Content:   "dm message",
	}

	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	// guild_id should be omitted when empty.
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	if _, exists := raw["guild_id"]; exists {
		t.Error("guild_id should be omitted when empty")
	}
}

func TestUserDoc_JSON(t *testing.T) {
	displayName := "Alice"
	doc := UserDoc{
		ID:          "user_001",
		InstanceID:  "inst_001",
		Username:    "alice",
		DisplayName: &displayName,
	}

	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded UserDoc
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Username != "alice" {
		t.Errorf("username = %q, want %q", decoded.Username, "alice")
	}
	if decoded.DisplayName == nil || *decoded.DisplayName != "Alice" {
		t.Errorf("display_name = %v, want %q", decoded.DisplayName, "Alice")
	}
}

func TestUserDoc_OmitEmptyDisplayName(t *testing.T) {
	doc := UserDoc{
		ID:       "user_002",
		Username: "bob",
	}

	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	if _, exists := raw["display_name"]; exists {
		t.Error("display_name should be omitted when nil")
	}
}

func TestGuildDoc_JSON(t *testing.T) {
	doc := GuildDoc{
		ID:          "guild_001",
		Name:        "Test Guild",
		Description: "A test guild",
		MemberCount: 42,
	}

	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded GuildDoc
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Name != "Test Guild" {
		t.Errorf("name = %q, want %q", decoded.Name, "Test Guild")
	}
	if decoded.MemberCount != 42 {
		t.Errorf("member_count = %d, want 42", decoded.MemberCount)
	}
}

func TestGuildDoc_OmitEmptyDescription(t *testing.T) {
	doc := GuildDoc{
		ID:   "guild_002",
		Name: "No Desc Guild",
	}

	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	if _, exists := raw["description"]; exists {
		t.Error("description should be omitted when empty")
	}
}

func TestSearchRequest_Defaults(t *testing.T) {
	req := SearchRequest{
		Query: "hello",
		Index: IndexMessages,
	}

	if req.Limit != 0 {
		t.Errorf("default limit = %d, want 0", req.Limit)
	}
	if req.Offset != 0 {
		t.Errorf("default offset = %d, want 0", req.Offset)
	}

	// The Search() method defaults Limit to 20 when <= 0.
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 20
	}
	if req.Limit != 20 {
		t.Errorf("normalized limit = %d, want 20", req.Limit)
	}
}

func TestSearchResult_JSON(t *testing.T) {
	result := SearchResult{
		IDs:              []string{"msg_001"},
		EstimatedTotal:   100,
		ProcessingTimeMs: 5,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded SearchResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.EstimatedTotal != 100 {
		t.Errorf("estimated_total = %d, want 100", decoded.EstimatedTotal)
	}
	if decoded.ProcessingTimeMs != 5 {
		t.Errorf("processing_time_ms = %d, want 5", decoded.ProcessingTimeMs)
	}
	if len(decoded.IDs) != 1 {
		t.Errorf("IDs length = %d, want 1", len(decoded.IDs))
	}
	if len(decoded.IDs) > 0 && decoded.IDs[0] != "msg_001" {
		t.Errorf("IDs[0] = %q, want %q", decoded.IDs[0], "msg_001")
	}
}

func TestSearchResult_EmptyIDs(t *testing.T) {
	result := SearchResult{
		IDs:            []string{},
		EstimatedTotal: 0,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded SearchResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if len(decoded.IDs) != 0 {
		t.Errorf("IDs length = %d, want 0", len(decoded.IDs))
	}
}

func TestDocOpts(t *testing.T) {
	opts := docOpts()
	if opts == nil {
		t.Fatal("docOpts returned nil")
	}
	if opts.PrimaryKey == nil {
		t.Fatal("PrimaryKey is nil")
	}
	if *opts.PrimaryKey != "id" {
		t.Errorf("PrimaryKey = %q, want %q", *opts.PrimaryKey, "id")
	}
}
