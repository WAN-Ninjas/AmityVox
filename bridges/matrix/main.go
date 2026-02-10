// Package main implements the Matrix bridge for AmityVox. It runs as a separate
// Docker container and implements the Matrix Application Service API to bridge
// messages, presence, and typing indicators between AmityVox channels and Matrix
// rooms. See docs/architecture.md Section 10.1 for the bridge specification.
//
// The bridge:
//   - Registers as an appservice on a Matrix homeserver (Conduit, Dendrite, or Synapse)
//   - Maps AmityVox channels ↔ Matrix rooms bidirectionally
//   - Translates message formats (markdown ↔ Matrix event format)
//   - Bridges user presence and typing indicators
//   - Uses masquerade on the AmityVox side (bridged Matrix users show their Matrix name/avatar)
//   - Uses virtual Matrix users on the Matrix side
//
// This bridge will be fully implemented in v0.2.0.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("AmityVox Matrix Bridge")
	fmt.Println("This bridge is not yet implemented.")
	fmt.Println("It will be available in v0.2.0.")
	fmt.Println()
	fmt.Println("Configuration:")
	fmt.Printf("  AMITYVOX_URL:     %s\n", envOr("AMITYVOX_URL", "(not set)"))
	fmt.Printf("  AMITYVOX_TOKEN:   %s\n", envOr("AMITYVOX_TOKEN", "(not set)"))
	fmt.Printf("  MATRIX_HOMESERVER: %s\n", envOr("MATRIX_HOMESERVER", "(not set)"))
	fmt.Printf("  MATRIX_AS_TOKEN:  %s\n", envOr("MATRIX_AS_TOKEN", "(not set)"))
	os.Exit(1)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
