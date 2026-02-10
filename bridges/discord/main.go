// Package main implements the Discord bridge for AmityVox. It runs as a separate
// Docker container and uses Discord's bot API to relay messages bidirectionally
// between AmityVox channels and Discord channels. See docs/architecture.md
// Section 10.2 for the bridge specification.
//
// The bridge:
//   - Connects to Discord via a bot token and the Discord gateway
//   - Maps AmityVox channels ↔ Discord channels
//   - Relays messages bidirectionally using webhooks for display name/avatar fidelity
//   - Useful for migration: run both simultaneously during transition
//
// This bridge will be fully implemented in v0.2.0.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("AmityVox Discord Bridge")
	fmt.Println("This bridge is not yet implemented.")
	fmt.Println("It will be available in v0.2.0.")
	fmt.Println()
	fmt.Println("Configuration:")
	fmt.Printf("  AMITYVOX_URL:    %s\n", envOr("AMITYVOX_URL", "(not set)"))
	fmt.Printf("  AMITYVOX_TOKEN:  %s\n", envOr("AMITYVOX_TOKEN", "(not set)"))
	fmt.Printf("  DISCORD_TOKEN:   %s\n", envOr("DISCORD_TOKEN", "(not set)"))
	os.Exit(1)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
