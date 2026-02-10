// Package encryption implements MLS (RFC 9420) key management for optional
// end-to-end encrypted channels and DMs. The server acts as an MLS Delivery
// Service, routing encrypted messages and storing key packages without ever
// seeing plaintext or private keys.
//
// This package will be fully implemented in v0.2.0. Currently it defines the
// data types and API contracts.
package encryption

import "time"

// KeyPackage represents an MLS key package uploaded by a client for use in
// establishing encrypted group sessions. The server stores these without
// accessing private key material.
type KeyPackage struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	DeviceID  string    `json:"device_id"`
	Data      []byte    `json:"data"`       // Opaque MLS KeyPackage bytes.
	ExpiresAt time.Time `json:"expires_at"` // KeyPackages expire and must be refreshed.
	CreatedAt time.Time `json:"created_at"`
}

// WelcomeMessage is sent to a new member when they join an encrypted channel.
// It contains the MLS Welcome message that allows the new member to derive the
// group's encryption keys.
type WelcomeMessage struct {
	ID        string    `json:"id"`
	ChannelID string    `json:"channel_id"`
	ReceiverID string   `json:"receiver_id"`
	Data      []byte    `json:"data"` // Opaque MLS Welcome bytes.
	CreatedAt time.Time `json:"created_at"`
}

// GroupState tracks the current MLS group epoch for an encrypted channel.
// The server stores this to facilitate member additions and removals.
type GroupState struct {
	ChannelID string    `json:"channel_id"`
	Epoch     uint64    `json:"epoch"`
	TreeHash  []byte    `json:"tree_hash"` // Hash of the ratchet tree for consistency checks.
	UpdatedAt time.Time `json:"updated_at"`
}

// Commit represents an MLS Commit message that advances the group state.
// Commits are published when members join, leave, or update their keys.
type Commit struct {
	ID        string    `json:"id"`
	ChannelID string    `json:"channel_id"`
	SenderID  string    `json:"sender_id"`
	Epoch     uint64    `json:"epoch"`
	Data      []byte    `json:"data"` // Opaque MLS Commit bytes.
	CreatedAt time.Time `json:"created_at"`
}
