package permissions

import (
	"testing"
	"time"
)

func TestPermissionConstants_NoDuplicates(t *testing.T) {
	// Every permission should have a unique bit value.
	seen := make(map[uint64]string)
	for bit, name := range permissionNames {
		if existing, ok := seen[bit]; ok {
			t.Errorf("duplicate bit 0x%X: %s and %s", bit, existing, name)
		}
		seen[bit] = name
	}
}

func TestPermissionConstants_ArePowersOfTwo(t *testing.T) {
	for bit, name := range permissionNames {
		if bit == 0 || (bit&(bit-1)) != 0 {
			t.Errorf("permission %s (0x%X) is not a power of two", name, bit)
		}
	}
}

func TestHasPermission(t *testing.T) {
	tests := []struct {
		name   string
		perms  uint64
		perm   uint64
		expect bool
	}{
		{"has single", SendMessages, SendMessages, true},
		{"missing", SendMessages, ManageGuild, false},
		{"has among many", SendMessages | ViewChannel | ReadHistory, ViewChannel, true},
		{"zero perms", 0, SendMessages, false},
		{"administrator", Administrator, Administrator, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasPermission(tc.perms, tc.perm); got != tc.expect {
				t.Errorf("HasPermission(0x%X, 0x%X) = %v, want %v", tc.perms, tc.perm, got, tc.expect)
			}
		})
	}
}

func TestHasAnyPermission(t *testing.T) {
	perms := SendMessages | ViewChannel
	if !HasAnyPermission(perms, ManageGuild, SendMessages) {
		t.Error("HasAnyPermission should return true when one matches")
	}
	if HasAnyPermission(perms, ManageGuild, BanMembers) {
		t.Error("HasAnyPermission should return false when none match")
	}
}

func TestHasAllPermissions(t *testing.T) {
	perms := SendMessages | ViewChannel | ReadHistory
	if !HasAllPermissions(perms, SendMessages, ViewChannel) {
		t.Error("HasAllPermissions should return true when all present")
	}
	if HasAllPermissions(perms, SendMessages, ManageGuild) {
		t.Error("HasAllPermissions should return false when one missing")
	}
}

func TestCalculatePermissions_OwnerGetsAll(t *testing.T) {
	member := MemberInfo{UserID: "owner123"}
	guild := GuildInfo{OwnerID: "owner123", DefaultPermissions: ViewChannel}

	got := CalculatePermissions(member, guild, nil, nil)
	if got != AllPermissions {
		t.Errorf("owner should get AllPermissions, got 0x%X", got)
	}
}

func TestCalculatePermissions_DefaultPerms(t *testing.T) {
	member := MemberInfo{UserID: "user1"}
	guild := GuildInfo{OwnerID: "other", DefaultPermissions: ViewChannel | SendMessages}

	got := CalculatePermissions(member, guild, nil, nil)
	if got != ViewChannel|SendMessages {
		t.Errorf("got 0x%X, want 0x%X", got, ViewChannel|SendMessages)
	}
}

func TestCalculatePermissions_RoleAllowDeny(t *testing.T) {
	member := MemberInfo{UserID: "user1"}
	guild := GuildInfo{OwnerID: "other", DefaultPermissions: ViewChannel | SendMessages | ReadHistory}
	roles := []RoleInfo{
		{ID: "role1", Position: 1, PermissionsAllow: ManageGuild, PermissionsDeny: SendMessages},
	}

	got := CalculatePermissions(member, guild, roles, nil)
	if !HasPermission(got, ManageGuild) {
		t.Error("role allow should grant ManageGuild")
	}
	if HasPermission(got, SendMessages) {
		t.Error("role deny should remove SendMessages")
	}
	if !HasPermission(got, ViewChannel) {
		t.Error("ViewChannel from defaults should remain")
	}
}

func TestCalculatePermissions_AdministratorBypass(t *testing.T) {
	member := MemberInfo{UserID: "user1"}
	guild := GuildInfo{OwnerID: "other", DefaultPermissions: ViewChannel}
	roles := []RoleInfo{
		{ID: "admin", Position: 1, PermissionsAllow: Administrator},
	}

	got := CalculatePermissions(member, guild, roles, nil)
	if got != AllPermissions {
		t.Errorf("administrator should get AllPermissions, got 0x%X", got)
	}
}

func TestCalculatePermissions_ChannelOverrides(t *testing.T) {
	member := MemberInfo{UserID: "user1"}
	guild := GuildInfo{OwnerID: "other", DefaultPermissions: ViewChannel | SendMessages}
	roles := []RoleInfo{
		{ID: "role1", Position: 1, PermissionsAllow: ReadHistory},
	}

	denyPerms := SendMessages
	channel := &ChannelInfo{
		Overrides: []ChannelOverride{
			{TargetType: "role", TargetID: "role1", PermissionsAllow: ManageMessages},
			{TargetType: "user", TargetID: "user1", PermissionsDeny: uint64(denyPerms)},
		},
	}

	got := CalculatePermissions(member, guild, roles, channel)
	if !HasPermission(got, ManageMessages) {
		t.Error("channel role override should grant ManageMessages")
	}
	if HasPermission(got, SendMessages) {
		t.Error("channel user override should deny SendMessages")
	}
}

func TestCalculatePermissions_ChannelDefaultOverrides(t *testing.T) {
	member := MemberInfo{UserID: "user1"}
	guild := GuildInfo{OwnerID: "other", DefaultPermissions: ViewChannel | SendMessages}

	allow := EmbedLinks
	deny := SendMessages
	channel := &ChannelInfo{
		DefaultPermissionsAllow: &allow,
		DefaultPermissionsDeny:  &deny,
	}

	got := CalculatePermissions(member, guild, nil, channel)
	if !HasPermission(got, EmbedLinks) {
		t.Error("channel default allow should grant EmbedLinks")
	}
	if HasPermission(got, SendMessages) {
		t.Error("channel default deny should remove SendMessages")
	}
}

func TestCalculatePermissions_Timeout(t *testing.T) {
	future := time.Now().Add(1 * time.Hour)
	member := MemberInfo{UserID: "user1", TimeoutUntil: &future}
	guild := GuildInfo{OwnerID: "other", DefaultPermissions: ViewChannel | SendMessages | AddReactions | Connect}

	// Timeout is applied at step 8, which requires a channel context.
	channel := &ChannelInfo{}
	got := CalculatePermissions(member, guild, nil, channel)
	if HasPermission(got, SendMessages) {
		t.Error("timed-out member should not have SendMessages")
	}
	if HasPermission(got, AddReactions) {
		t.Error("timed-out member should not have AddReactions")
	}
	if !HasPermission(got, ViewChannel) {
		t.Error("timed-out member should still have ViewChannel")
	}
}

func TestCalculatePermissions_NoViewNoPerms(t *testing.T) {
	member := MemberInfo{UserID: "user1"}
	guild := GuildInfo{OwnerID: "other", DefaultPermissions: SendMessages | ReadHistory}

	deny := ViewChannel
	channel := &ChannelInfo{
		DefaultPermissionsDeny: &deny,
	}

	got := CalculatePermissions(member, guild, nil, channel)
	if got != 0 {
		t.Errorf("no ViewChannel should result in 0 perms, got 0x%X", got)
	}
}

func TestNames(t *testing.T) {
	perms := SendMessages | ViewChannel
	names := Names(perms)
	if len(names) != 2 {
		t.Fatalf("Names returned %d names, want 2", len(names))
	}

	nameMap := make(map[string]bool)
	for _, n := range names {
		nameMap[n] = true
	}
	if !nameMap["SendMessages"] || !nameMap["ViewChannel"] {
		t.Errorf("Names(%d) = %v, want SendMessages and ViewChannel", perms, names)
	}
}

func TestString(t *testing.T) {
	if s := String(0); s != "none" {
		t.Errorf("String(0) = %q, want %q", s, "none")
	}
	s := String(SendMessages)
	if s != "SendMessages" {
		t.Errorf("String(SendMessages) = %q, want %q", s, "SendMessages")
	}
}

func TestDebug(t *testing.T) {
	d := Debug(SendMessages)
	if d == "" {
		t.Fatal("Debug returned empty string")
	}
	if len(d) < 10 {
		t.Errorf("Debug output too short: %q", d)
	}
}

func TestAllPermissions_IncludesAdministrator(t *testing.T) {
	if AllPermissions&Administrator == 0 {
		t.Error("AllPermissions should include Administrator")
	}
}

func TestTimeoutActionMask_DoesNotIncludeView(t *testing.T) {
	if TimeoutActionMask&ViewChannel != 0 {
		t.Error("TimeoutActionMask should not include ViewChannel")
	}
	if TimeoutActionMask&SendMessages == 0 {
		t.Error("TimeoutActionMask should include SendMessages")
	}
}
