// Package models defines shared data types for all AmityVox entities including
// User, Guild, Channel, Message, Member, Role, and others. Types include JSON
// tags for API serialization and match the PostgreSQL schema exactly.
package models

import (
	"crypto/rand"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// ulidEntropy is a thread-safe entropy source for ULID generation.
// It uses a mutex-protected monotonic reader backed by crypto/rand.
var ulidEntropy = &lockedMonotonicReader{
	r: ulid.Monotonic(rand.Reader, 0),
}

type lockedMonotonicReader struct {
	mu sync.Mutex
	r  io.Reader
}

func (lr *lockedMonotonicReader) Read(p []byte) (int, error) {
	lr.mu.Lock()
	defer lr.mu.Unlock()
	return lr.r.Read(p)
}

// ULID is a wrapper around oklog/ulid.ULID that provides JSON marshaling,
// SQL scanning, and string conversion for use throughout AmityVox.
type ULID struct {
	ulid.ULID
}

// NewULID generates a new ULID using the current time and thread-safe
// monotonic entropy. It is safe for concurrent use from multiple goroutines.
func NewULID() ULID {
	return ULID{ulid.MustNew(ulid.Timestamp(time.Now()), ulidEntropy)}
}

// NewULIDWithTime generates a new ULID using the specified time and thread-safe
// monotonic entropy. Useful for testing or importing historical data.
func NewULIDWithTime(t time.Time) ULID {
	return ULID{ulid.MustNew(ulid.Timestamp(t), ulidEntropy)}
}

// ParseULID parses a ULID from its string representation.
func ParseULID(s string) (ULID, error) {
	id, err := ulid.Parse(s)
	if err != nil {
		return ULID{}, fmt.Errorf("parsing ULID %q: %w", s, err)
	}
	return ULID{id}, nil
}

// MustParseULID parses a ULID from its string representation and panics on error.
// Use only in tests or initialization code.
func MustParseULID(s string) ULID {
	id, err := ParseULID(s)
	if err != nil {
		panic(err)
	}
	return id
}

// IsZero reports whether the ULID is the zero value.
func (u ULID) IsZero() bool {
	return u.ULID.Compare(ulid.ULID{}) == 0
}

// Time returns the time encoded in the ULID's timestamp component.
func (u ULID) Time() time.Time {
	return ulid.Time(u.ULID.Time())
}

// String returns the canonical string representation of the ULID.
func (u ULID) String() string {
	return u.ULID.String()
}

// MarshalJSON implements json.Marshaler, encoding the ULID as a JSON string.
func (u ULID) MarshalJSON() ([]byte, error) {
	return json.Marshal(u.String())
}

// UnmarshalJSON implements json.Unmarshaler, decoding a JSON string to a ULID.
func (u *ULID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("unmarshaling ULID JSON: %w", err)
	}
	if s == "" {
		*u = ULID{}
		return nil
	}
	parsed, err := ParseULID(s)
	if err != nil {
		return err
	}
	*u = parsed
	return nil
}

// Scan implements database/sql.Scanner for reading ULIDs from PostgreSQL TEXT columns.
func (u *ULID) Scan(src interface{}) error {
	if src == nil {
		*u = ULID{}
		return nil
	}
	switch v := src.(type) {
	case string:
		parsed, err := ParseULID(v)
		if err != nil {
			return err
		}
		*u = parsed
		return nil
	case []byte:
		parsed, err := ParseULID(string(v))
		if err != nil {
			return err
		}
		*u = parsed
		return nil
	default:
		return fmt.Errorf("unsupported ULID scan source type: %T", src)
	}
}

// Value implements database/sql/driver.Valuer for writing ULIDs to PostgreSQL TEXT columns.
func (u ULID) Value() (driver.Value, error) {
	if u.IsZero() {
		return nil, nil
	}
	return u.String(), nil
}
