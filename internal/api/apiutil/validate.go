package apiutil

import (
	"fmt"
	"net/http"
	"unicode/utf8"
)

// RequireNonEmpty checks that s is not empty. On failure it writes a 400 error
// with message "<field> is required" and returns false.
func RequireNonEmpty(w http.ResponseWriter, field, s string) bool {
	if s == "" {
		WriteError(w, http.StatusBadRequest, "invalid_body", field+" is required")
		return false
	}
	return true
}

// ValidateStringLength checks that s has between min and max runes (inclusive).
// Pass min=0 to skip the minimum check. On failure it writes a 400 error and
// returns false.
func ValidateStringLength(w http.ResponseWriter, field, s string, min, max int) bool {
	n := utf8.RuneCountInString(s)
	if min > 0 && n < min {
		WriteError(w, http.StatusBadRequest, "invalid_body",
			fmt.Sprintf("%s must be at least %d characters", field, min))
		return false
	}
	if max > 0 && n > max {
		WriteError(w, http.StatusBadRequest, "invalid_body",
			fmt.Sprintf("%s must be at most %d characters", field, max))
		return false
	}
	return true
}

// ValidateEnum checks that value is one of the allowed values. On failure it
// writes a 400 error listing the valid options and returns false.
func ValidateEnum(w http.ResponseWriter, field, value string, allowed []string) bool {
	for _, a := range allowed {
		if value == a {
			return true
		}
	}
	WriteError(w, http.StatusBadRequest, "invalid_body",
		fmt.Sprintf("Invalid %s (allowed: %v)", field, allowed))
	return false
}
