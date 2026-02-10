package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewULID(t *testing.T) {
	id := NewULID()
	if id.IsZero() {
		t.Fatal("NewULID returned zero ULID")
	}
	if id.String() == "" {
		t.Fatal("NewULID.String() returned empty string")
	}
	if len(id.String()) != 26 {
		t.Fatalf("ULID string length = %d, want 26", len(id.String()))
	}
}

func TestNewULID_Unique(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		id := NewULID()
		s := id.String()
		if seen[s] {
			t.Fatalf("duplicate ULID generated: %s", s)
		}
		seen[s] = true
	}
}

func TestNewULID_Monotonic(t *testing.T) {
	// ULIDs generated in sequence should have non-decreasing timestamps.
	ids := make([]ULID, 100)
	for i := range ids {
		ids[i] = NewULID()
	}
	for i := 1; i < len(ids); i++ {
		if ids[i].Time().Before(ids[i-1].Time()) {
			t.Fatalf("ULID timestamps not monotonic: %s (%v) before %s (%v)",
				ids[i], ids[i].Time(), ids[i-1], ids[i-1].Time())
		}
	}
}

func TestNewULIDWithTime(t *testing.T) {
	ts := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	id := NewULIDWithTime(ts)
	if id.IsZero() {
		t.Fatal("NewULIDWithTime returned zero ULID")
	}
	// The ULID timestamp should be within 1ms of the input time.
	got := id.Time()
	diff := got.Sub(ts)
	if diff < 0 {
		diff = -diff
	}
	if diff > time.Millisecond {
		t.Fatalf("ULID time = %v, want within 1ms of %v (diff = %v)", got, ts, diff)
	}
}

func TestParseULID(t *testing.T) {
	original := NewULID()
	parsed, err := ParseULID(original.String())
	if err != nil {
		t.Fatalf("ParseULID(%q) error: %v", original.String(), err)
	}
	if parsed.String() != original.String() {
		t.Fatalf("ParseULID roundtrip: got %s, want %s", parsed, original)
	}
}

func TestParseULID_Invalid(t *testing.T) {
	cases := []string{
		"",
		"not-a-ulid",
		"12345",
		"ZZZZZZZZZZZZZZZZZZZZZZZZZZ", // invalid base32
	}
	for _, tc := range cases {
		_, err := ParseULID(tc)
		if err == nil {
			t.Errorf("ParseULID(%q) expected error, got nil", tc)
		}
	}
}

func TestMustParseULID_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("MustParseULID with invalid input did not panic")
		}
	}()
	MustParseULID("invalid")
}

func TestULID_IsZero(t *testing.T) {
	var zero ULID
	if !zero.IsZero() {
		t.Fatal("zero-value ULID.IsZero() = false, want true")
	}
	nonZero := NewULID()
	if nonZero.IsZero() {
		t.Fatal("NewULID().IsZero() = true, want false")
	}
}

func TestULID_JSON(t *testing.T) {
	original := NewULID()

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}

	var parsed ULID
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}

	if parsed.String() != original.String() {
		t.Fatalf("JSON roundtrip: got %s, want %s", parsed, original)
	}
}

func TestULID_JSON_Empty(t *testing.T) {
	var u ULID
	if err := json.Unmarshal([]byte(`""`), &u); err != nil {
		t.Fatalf("unmarshal empty string error: %v", err)
	}
	if !u.IsZero() {
		t.Fatal("unmarshaled empty string should be zero ULID")
	}
}

func TestULID_Scan(t *testing.T) {
	original := NewULID()
	var scanned ULID

	// Scan from string.
	if err := scanned.Scan(original.String()); err != nil {
		t.Fatalf("Scan(string) error: %v", err)
	}
	if scanned.String() != original.String() {
		t.Fatalf("Scan(string) = %s, want %s", scanned, original)
	}

	// Scan from []byte.
	var scanned2 ULID
	if err := scanned2.Scan([]byte(original.String())); err != nil {
		t.Fatalf("Scan([]byte) error: %v", err)
	}
	if scanned2.String() != original.String() {
		t.Fatalf("Scan([]byte) = %s, want %s", scanned2, original)
	}

	// Scan from nil.
	var scanned3 ULID
	if err := scanned3.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) error: %v", err)
	}
	if !scanned3.IsZero() {
		t.Fatal("Scan(nil) should produce zero ULID")
	}
}

func TestULID_Value(t *testing.T) {
	id := NewULID()
	val, err := id.Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}
	if val != id.String() {
		t.Fatalf("Value() = %v, want %s", val, id)
	}

	var zero ULID
	val, err = zero.Value()
	if err != nil {
		t.Fatalf("zero Value() error: %v", err)
	}
	if val != nil {
		t.Fatalf("zero Value() = %v, want nil", val)
	}
}
