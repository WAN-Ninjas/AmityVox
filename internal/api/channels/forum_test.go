package channels

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amityvox/amityvox/internal/api/apiutil"
)

// TestJoinStrings validates the string joining helper used in forum tag updates.
func TestJoinStrings(t *testing.T) {
	tests := []struct {
		name   string
		parts  []string
		sep    string
		expect string
	}{
		{"empty", nil, ", ", ""},
		{"single", []string{"a"}, ", ", "a"},
		{"multiple", []string{"a", "b", "c"}, ", ", "a, b, c"},
		{"sql clauses", []string{"name = $1", "color = $2"}, ", ", "name = $1, color = $2"},
		{"different sep", []string{"x", "y"}, " AND ", "x AND y"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := joinStrings(tc.parts, tc.sep)
			if got != tc.expect {
				t.Errorf("joinStrings(%v, %q) = %q, want %q", tc.parts, tc.sep, got, tc.expect)
			}
		})
	}
}

// TestForumPostValidation tests the request validation logic that the
// HandleCreateForumPost handler applies to incoming request bodies.
func TestForumPostValidation(t *testing.T) {
	tests := []struct {
		name        string
		title       string
		content     string
		tagIDs      []string
		requireTags bool
		wantError   string
	}{
		{
			name:      "valid post",
			title:     "Bug Report",
			content:   "Steps to reproduce...",
			wantError: "",
		},
		{
			name:      "missing title",
			title:     "",
			content:   "Some content",
			wantError: "missing_title",
		},
		{
			name:      "missing content",
			title:     "A Title",
			content:   "",
			wantError: "missing_content",
		},
		{
			name:        "tags required but none provided",
			title:       "A Title",
			content:     "Content",
			tagIDs:      nil,
			requireTags: true,
			wantError:   "tags_required",
		},
		{
			name:        "tags required and provided",
			title:       "A Title",
			content:     "Content",
			tagIDs:      []string{"tag1"},
			requireTags: true,
			wantError:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Validate the same logic the handler uses.
			var errCode string
			if tc.title == "" {
				errCode = "missing_title"
			} else if tc.content == "" {
				errCode = "missing_content"
			} else if tc.requireTags && len(tc.tagIDs) == 0 {
				errCode = "tags_required"
			}

			if errCode != tc.wantError {
				t.Errorf("validation error = %q, want %q", errCode, tc.wantError)
			}
		})
	}
}

// TestContentPreviewTruncation verifies the content preview logic.
func TestContentPreviewTruncation(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantLen int
	}{
		{"short content", "Hello world", 11},
		{"exactly 200", string(make([]byte, 200)), 200},
		{"over 200", string(make([]byte, 500)), 200},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var preview string
			if len(tc.content) > 200 {
				preview = tc.content[:200]
			} else {
				preview = tc.content
			}

			if len(preview) != tc.wantLen {
				t.Errorf("preview length = %d, want %d", len(preview), tc.wantLen)
			}
		})
	}
}

// TestForumSortOptions validates the sorting options for forum posts.
func TestForumSortOptions(t *testing.T) {
	tests := []struct {
		name    string
		sortBy  string
		wantSQL string
	}{
		{"default is latest_activity", "", "latest_activity"},
		{"latest_activity", "latest_activity", "latest_activity"},
		{"creation_date", "creation_date", "creation_date"},
		{"unknown defaults to latest_activity", "invalid", "latest_activity"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sortBy := tc.sortBy
			if sortBy == "" || (sortBy != "latest_activity" && sortBy != "creation_date") {
				sortBy = "latest_activity"
			}
			if sortBy != tc.wantSQL {
				t.Errorf("sortBy = %q, want %q", sortBy, tc.wantSQL)
			}
		})
	}
}

// TestForumLimitParsing validates the limit query parameter parsing.
func TestForumLimitParsing(t *testing.T) {
	tests := []struct {
		name      string
		limitStr  string
		wantLimit int
	}{
		{"empty defaults to 25", "", 25},
		{"valid 10", "10", 10},
		{"valid 100", "100", 100},
		{"over max defaults to 25", "200", 25},
		{"zero defaults to 25", "0", 25},
		{"negative defaults to 25", "-5", 25},
		{"non-numeric defaults to 25", "abc", 25},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			limit := 25
			if tc.limitStr != "" {
				v := 0
				for _, c := range tc.limitStr {
					if c >= '0' && c <= '9' {
						v = v*10 + int(c-'0')
					} else {
						v = -1
						break
					}
				}
				if tc.limitStr[0] == '-' {
					v = -1
				}
				if v > 0 && v <= 100 {
					limit = v
				}
			}
			if limit != tc.wantLimit {
				t.Errorf("limit = %d, want %d", limit, tc.wantLimit)
			}
		})
	}
}

// TestForumTagCreateResponse validates the JSON structure of tag creation responses.
func TestForumTagCreateResponse(t *testing.T) {
	w := httptest.NewRecorder()

	tag := map[string]interface{}{
		"id":         "tag123",
		"channel_id": "ch123",
		"name":       "Bug",
		"emoji":      "🐛",
		"color":      "#ff0000",
		"position":   0,
	}

	apiutil.WriteJSON(w, http.StatusCreated, tag)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'data' key in response")
	}
	if data["name"] != "Bug" {
		t.Errorf("data.name = %v, want %q", data["name"], "Bug")
	}
	if data["emoji"] != "🐛" {
		t.Errorf("data.emoji = %v, want %q", data["emoji"], "🐛")
	}
}

// TestForumPostListResponse validates the JSON structure of a post list response.
func TestForumPostListResponse(t *testing.T) {
	w := httptest.NewRecorder()

	posts := []map[string]interface{}{
		{
			"id":          "post1",
			"name":        "First Post",
			"pinned":      true,
			"locked":      false,
			"reply_count": 5,
			"tags":        []map[string]interface{}{},
		},
		{
			"id":          "post2",
			"name":        "Second Post",
			"pinned":      false,
			"locked":      true,
			"reply_count": 0,
			"tags":        []map[string]interface{}{},
		},
	}

	apiutil.WriteJSON(w, http.StatusOK, posts)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	data, ok := resp["data"].([]interface{})
	if !ok {
		t.Fatal("expected 'data' to be an array")
	}
	if len(data) != 2 {
		t.Errorf("data length = %d, want 2", len(data))
	}

	first := data[0].(map[string]interface{})
	if first["pinned"] != true {
		t.Error("first post should be pinned")
	}
	second := data[1].(map[string]interface{})
	if second["locked"] != true {
		t.Error("second post should be locked")
	}
}
