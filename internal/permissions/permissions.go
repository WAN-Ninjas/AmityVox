// Package permissions implements the AmityVox bitfield permission system. It defines
// all permission constants, the CalculatePermissions algorithm from the architecture
// specification (Section 5.4), and helper functions for checking, combining, and
// displaying permission sets.
package permissions

import (
	"fmt"
	"strings"
	"time"
)

// Server-scoped permissions (bits 0-19).
const (
	ManageChannels    uint64 = 1 << 0
	ManageGuild       uint64 = 1 << 1
	ManagePermissions uint64 = 1 << 2
	ManageRoles       uint64 = 1 << 3
	ManageEmoji       uint64 = 1 << 4
	ManageWebhooks    uint64 = 1 << 5
	KickMembers       uint64 = 1 << 6
	BanMembers        uint64 = 1 << 7
	TimeoutMembers    uint64 = 1 << 8
	AssignRoles       uint64 = 1 << 9
	ChangeNickname    uint64 = 1 << 10
	ManageNicknames   uint64 = 1 << 11
	ChangeAvatar      uint64 = 1 << 12
	RemoveAvatars     uint64 = 1 << 13
	ViewAuditLog      uint64 = 1 << 14
	ViewGuildInsights uint64 = 1 << 15
	MentionEveryone   uint64 = 1 << 16
)

// Channel-scoped permissions (bits 20-39).
const (
	ViewChannel      uint64 = 1 << 20
	ReadHistory      uint64 = 1 << 21
	SendMessages     uint64 = 1 << 22
	ManageMessages   uint64 = 1 << 23
	EmbedLinks       uint64 = 1 << 24
	UploadFiles      uint64 = 1 << 25
	AddReactions     uint64 = 1 << 26
	UseExternalEmoji uint64 = 1 << 27
	Connect          uint64 = 1 << 28
	Speak            uint64 = 1 << 29
	MuteMembers      uint64 = 1 << 30
	DeafenMembers    uint64 = 1 << 31
	MoveMembers      uint64 = 1 << 32
	UseVAD           uint64 = 1 << 33
	PrioritySpeaker  uint64 = 1 << 34
	Stream           uint64 = 1 << 35
	Masquerade       uint64 = 1 << 36
	CreateInvites    uint64 = 1 << 37
	ManageThreads    uint64 = 1 << 38
	CreateThreads    uint64 = 1 << 39
)

// Administrator (bit 63) bypasses all permission checks.
const Administrator uint64 = 1 << 63

// AllPermissions is the bitmask with every defined permission bit set.
const AllPermissions uint64 = ManageChannels | ManageGuild | ManagePermissions |
	ManageRoles | ManageEmoji | ManageWebhooks | KickMembers | BanMembers |
	TimeoutMembers | AssignRoles | ChangeNickname | ManageNicknames |
	ChangeAvatar | RemoveAvatars | ViewAuditLog | ViewGuildInsights |
	MentionEveryone | ViewChannel | ReadHistory | SendMessages |
	ManageMessages | EmbedLinks | UploadFiles | AddReactions |
	UseExternalEmoji | Connect | Speak | MuteMembers | DeafenMembers |
	MoveMembers | UseVAD | PrioritySpeaker | Stream | Masquerade |
	CreateInvites | ManageThreads | CreateThreads | Administrator

// TimeoutActionMask contains the permissions stripped from timed-out members.
const TimeoutActionMask uint64 = SendMessages | AddReactions | Connect |
	Speak | Stream | CreateThreads | CreateInvites

// permissionNames maps each permission bit to a human-readable name.
var permissionNames = map[uint64]string{
	ManageChannels:    "ManageChannels",
	ManageGuild:       "ManageGuild",
	ManagePermissions: "ManagePermissions",
	ManageRoles:       "ManageRoles",
	ManageEmoji:       "ManageEmoji",
	ManageWebhooks:    "ManageWebhooks",
	KickMembers:       "KickMembers",
	BanMembers:        "BanMembers",
	TimeoutMembers:    "TimeoutMembers",
	AssignRoles:       "AssignRoles",
	ChangeNickname:    "ChangeNickname",
	ManageNicknames:   "ManageNicknames",
	ChangeAvatar:      "ChangeAvatar",
	RemoveAvatars:     "RemoveAvatars",
	ViewAuditLog:      "ViewAuditLog",
	ViewGuildInsights: "ViewGuildInsights",
	MentionEveryone:   "MentionEveryone",
	ViewChannel:       "ViewChannel",
	ReadHistory:       "ReadHistory",
	SendMessages:      "SendMessages",
	ManageMessages:    "ManageMessages",
	EmbedLinks:        "EmbedLinks",
	UploadFiles:       "UploadFiles",
	AddReactions:      "AddReactions",
	UseExternalEmoji:  "UseExternalEmoji",
	Connect:           "Connect",
	Speak:             "Speak",
	MuteMembers:       "MuteMembers",
	DeafenMembers:     "DeafenMembers",
	MoveMembers:       "MoveMembers",
	UseVAD:            "UseVAD",
	PrioritySpeaker:   "PrioritySpeaker",
	Stream:            "Stream",
	Masquerade:        "Masquerade",
	CreateInvites:     "CreateInvites",
	ManageThreads:     "ManageThreads",
	CreateThreads:     "CreateThreads",
	Administrator:     "Administrator",
}

// MemberInfo holds the fields needed to calculate permissions for a guild member.
type MemberInfo struct {
	UserID       string
	TimeoutUntil *time.Time
}

// GuildInfo holds the guild-level fields needed for permission calculation.
type GuildInfo struct {
	OwnerID            string
	DefaultPermissions uint64
}

// RoleInfo holds role allow/deny bitfields for permission calculation.
type RoleInfo struct {
	ID               string
	Position         int
	PermissionsAllow uint64
	PermissionsDeny  uint64
}

// ChannelOverride holds a channel-level permission override for a role or user.
type ChannelOverride struct {
	TargetType       string // "role" or "user"
	TargetID         string
	PermissionsAllow uint64
	PermissionsDeny  uint64
}

// ChannelInfo holds the channel-level fields needed for permission calculation.
type ChannelInfo struct {
	DefaultPermissionsAllow *uint64
	DefaultPermissionsDeny  *uint64
	Overrides               []ChannelOverride
}

// CalculatePermissions computes the effective permission set for a member in a
// specific channel, following the algorithm specified in docs/architecture.md
// Section 5.4.
//
// The resolution order is:
//  1. Guild owner gets all permissions
//  2. Start with @everyone base permissions
//  3. Apply role allow/deny (sorted by position descending — lowest position = highest priority applied last)
//  4. Administrator bypass
//  5. Channel-level @everyone overrides
//  6. Channel-level role overrides
//  7. Channel-level user overrides
//  8. Timeout strips action permissions
//  9. No view = no permissions
func CalculatePermissions(member MemberInfo, guild GuildInfo, roles []RoleInfo, channel *ChannelInfo) uint64 {
	// 1. Guild owner always has everything.
	if member.UserID == guild.OwnerID {
		return AllPermissions
	}

	// 2. Start with @everyone base permissions.
	perms := guild.DefaultPermissions

	// 3. Apply role permissions (sorted by position DESC — lowest position = highest priority last).
	// The caller should provide roles sorted by position descending.
	for _, role := range roles {
		perms |= role.PermissionsAllow
		perms &^= role.PermissionsDeny
	}

	// 4. Administrator bypasses everything.
	if perms&Administrator != 0 {
		return AllPermissions
	}

	// If no channel context, return guild-level permissions.
	if channel == nil {
		return perms
	}

	// 5. Apply channel-level @everyone overrides.
	if channel.DefaultPermissionsAllow != nil {
		perms |= *channel.DefaultPermissionsAllow
	}
	if channel.DefaultPermissionsDeny != nil {
		perms &^= *channel.DefaultPermissionsDeny
	}

	// 6. Apply channel-level role overrides.
	roleIDs := make(map[string]bool, len(roles))
	for _, r := range roles {
		roleIDs[r.ID] = true
	}
	for _, override := range channel.Overrides {
		if override.TargetType == "role" && roleIDs[override.TargetID] {
			perms |= override.PermissionsAllow
			perms &^= override.PermissionsDeny
		}
	}

	// 7. Apply channel-level user overrides.
	for _, override := range channel.Overrides {
		if override.TargetType == "user" && override.TargetID == member.UserID {
			perms |= override.PermissionsAllow
			perms &^= override.PermissionsDeny
		}
	}

	// 8. Timeout strips action permissions.
	if member.TimeoutUntil != nil && member.TimeoutUntil.After(time.Now()) {
		perms &^= TimeoutActionMask
	}

	// 9. Can't do anything in a channel you can't see.
	if perms&ViewChannel == 0 {
		return 0
	}

	return perms
}

// HasPermission reports whether the given permission set includes the specified permission.
func HasPermission(perms, perm uint64) bool {
	return perms&perm == perm
}

// HasAnyPermission reports whether the given permission set includes any of the
// specified permissions.
func HasAnyPermission(perms uint64, checkPerms ...uint64) bool {
	for _, p := range checkPerms {
		if perms&p == p {
			return true
		}
	}
	return false
}

// HasAllPermissions reports whether the given permission set includes all of the
// specified permissions.
func HasAllPermissions(perms uint64, checkPerms ...uint64) bool {
	for _, p := range checkPerms {
		if perms&p != p {
			return false
		}
	}
	return true
}

// Names returns a slice of human-readable names for all set permission bits.
func Names(perms uint64) []string {
	var names []string
	for bit, name := range permissionNames {
		if perms&bit == bit {
			names = append(names, name)
		}
	}
	return names
}

// String returns a human-readable comma-separated list of set permission names.
func String(perms uint64) string {
	names := Names(perms)
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

// Debug returns a detailed debug string showing the permission bitfield value
// and all set permission names.
func Debug(perms uint64) string {
	return fmt.Sprintf("0x%016X [%s]", perms, String(perms))
}
