package mentions

import (
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantUsers   []string
		wantRoles   []string
		wantHere    bool
	}{
		{
			name:      "no mentions",
			content:   "hello world",
			wantUsers: nil,
			wantRoles: nil,
			wantHere:  false,
		},
		{
			name:      "single user mention",
			content:   "hey <@01ARZ3NDEKTSV4RRFFQ69G5FAV>!",
			wantUsers: []string{"01ARZ3NDEKTSV4RRFFQ69G5FAV"},
			wantRoles: nil,
			wantHere:  false,
		},
		{
			name:      "multiple user mentions",
			content:   "<@01ARZ3NDEKTSV4RRFFQ69G5FAV> and <@01ARZ3NDEKTSV4RRFFQ69G5FAW>",
			wantUsers: []string{"01ARZ3NDEKTSV4RRFFQ69G5FAV", "01ARZ3NDEKTSV4RRFFQ69G5FAW"},
			wantRoles: nil,
			wantHere:  false,
		},
		{
			name:      "duplicate user mentions deduplicated",
			content:   "<@01ARZ3NDEKTSV4RRFFQ69G5FAV> said <@01ARZ3NDEKTSV4RRFFQ69G5FAV>",
			wantUsers: []string{"01ARZ3NDEKTSV4RRFFQ69G5FAV"},
			wantRoles: nil,
			wantHere:  false,
		},
		{
			name:      "single role mention",
			content:   "hey <@&01ARZ3NDEKTSV4RRFFQ69G5FAV>",
			wantUsers: nil,
			wantRoles: []string{"01ARZ3NDEKTSV4RRFFQ69G5FAV"},
			wantHere:  false,
		},
		{
			name:      "duplicate role mentions deduplicated",
			content:   "<@&01ARZ3NDEKTSV4RRFFQ69G5FAV> <@&01ARZ3NDEKTSV4RRFFQ69G5FAV>",
			wantRoles: []string{"01ARZ3NDEKTSV4RRFFQ69G5FAV"},
			wantHere:  false,
		},
		{
			name:      "@here detected",
			content:   "attention @here please read",
			wantUsers: nil,
			wantRoles: nil,
			wantHere:  true,
		},
		{
			name:      "mixed mentions",
			content:   "<@01ARZ3NDEKTSV4RRFFQ69G5FAV> <@&01ARZ3NDEKTSV4RRFFQ69G5FAW> @here",
			wantUsers: []string{"01ARZ3NDEKTSV4RRFFQ69G5FAV"},
			wantRoles: []string{"01ARZ3NDEKTSV4RRFFQ69G5FAW"},
			wantHere:  true,
		},
		{
			name:      "user mention inside code block ignored",
			content:   "```\n<@01ARZ3NDEKTSV4RRFFQ69G5FAV>\n```",
			wantUsers: nil,
			wantRoles: nil,
			wantHere:  false,
		},
		{
			name:      "user mention inside inline code ignored",
			content:   "use `<@01ARZ3NDEKTSV4RRFFQ69G5FAV>` syntax",
			wantUsers: nil,
			wantRoles: nil,
			wantHere:  false,
		},
		{
			name:      "@here inside code block ignored",
			content:   "```\n@here\n```",
			wantUsers: nil,
			wantRoles: nil,
			wantHere:  false,
		},
		{
			name:      "@here inside inline code ignored",
			content:   "type `@here` to ping",
			wantUsers: nil,
			wantRoles: nil,
			wantHere:  false,
		},
		{
			name:      "mention outside code block still detected",
			content:   "```\ncode\n``` <@01ARZ3NDEKTSV4RRFFQ69G5FAV>",
			wantUsers: []string{"01ARZ3NDEKTSV4RRFFQ69G5FAV"},
			wantRoles: nil,
			wantHere:  false,
		},
		{
			name:      "role mention inside inline code ignored",
			content:   "`<@&01ARZ3NDEKTSV4RRFFQ69G5FAV>`",
			wantRoles: nil,
			wantHere:  false,
		},
		{
			name:      "invalid ULID length ignored",
			content:   "<@SHORT> <@&SHORT>",
			wantUsers: nil,
			wantRoles: nil,
			wantHere:  false,
		},
		{
			name:      "lowercase ulid ignored",
			content:   "<@01arz3ndektsv4rrffq69g5fav>",
			wantUsers: nil,
			wantRoles: nil,
			wantHere:  false,
		},
		{
			name:      "@here inside email not detected",
			content:   "contact user@here.com for help",
			wantUsers: nil,
			wantRoles: nil,
			wantHere:  false,
		},
		{
			name:      "@here with punctuation detected",
			content:   "hey @here, read this!",
			wantUsers: nil,
			wantRoles: nil,
			wantHere:  true,
		},
		{
			name:      "empty content",
			content:   "",
			wantUsers: nil,
			wantRoles: nil,
			wantHere:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.content)

			if !sliceEqual(got.UserIDs, tt.wantUsers) {
				t.Errorf("UserIDs = %v, want %v", got.UserIDs, tt.wantUsers)
			}
			if !sliceEqual(got.RoleIDs, tt.wantRoles) {
				t.Errorf("RoleIDs = %v, want %v", got.RoleIDs, tt.wantRoles)
			}
			if got.MentionHere != tt.wantHere {
				t.Errorf("MentionHere = %v, want %v", got.MentionHere, tt.wantHere)
			}
		})
	}
}

func sliceEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
