package api

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/search"
)

// validIDPattern matches ULID/alphanumeric IDs to prevent filter injection.
var validIDPattern = regexp.MustCompile(`^[A-Za-z0-9]{26}$`)

// handleSearchMessages handles GET /api/v1/search/messages.
// Query params: q (required), channel_id, guild_id, author_id, limit, offset.
func (s *Server) handleSearchMessages(w http.ResponseWriter, r *http.Request) {
	if s.Search == nil {
		WriteError(w, http.StatusServiceUnavailable, "search_disabled", "Search is not enabled on this instance")
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		WriteError(w, http.StatusBadRequest, "missing_query", "Query parameter 'q' is required")
		return
	}

	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	limit, offset := parsePagination(r)

	// Build filter string for Meilisearch with input validation.
	var filters []string
	if channelID := r.URL.Query().Get("channel_id"); channelID != "" {
		if !validIDPattern.MatchString(channelID) {
			WriteError(w, http.StatusBadRequest, "invalid_channel_id", "Invalid channel_id format")
			return
		}
		filters = append(filters, fmt.Sprintf("channel_id = %q", channelID))
	}
	if guildID := r.URL.Query().Get("guild_id"); guildID != "" {
		if !validIDPattern.MatchString(guildID) {
			WriteError(w, http.StatusBadRequest, "invalid_guild_id", "Invalid guild_id format")
			return
		}
		filters = append(filters, fmt.Sprintf("guild_id = %q", guildID))
	}
	if authorID := r.URL.Query().Get("author_id"); authorID != "" {
		if !validIDPattern.MatchString(authorID) {
			WriteError(w, http.StatusBadRequest, "invalid_author_id", "Invalid author_id format")
			return
		}
		filters = append(filters, fmt.Sprintf("author_id = %q", authorID))
	}

	filterStr := ""
	for i, f := range filters {
		if i > 0 {
			filterStr += " AND "
		}
		filterStr += f
	}

	result, err := s.Search.Search(r.Context(), search.SearchRequest{
		Query:   query,
		Index:   search.IndexMessages,
		Filters: filterStr,
		Limit:   limit,
		Offset:  offset,
	})
	if err != nil {
		s.Logger.Error("search messages failed", "error", err.Error())
		WriteError(w, http.StatusInternalServerError, "search_error", "Search query failed")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"hits":              result.Hits,
		"estimated_total":   result.EstimatedTotal,
		"processing_time_ms": result.ProcessingTimeMs,
	})
}

// handleSearchUsers handles GET /api/v1/search/users.
// Query params: q (required), limit, offset.
func (s *Server) handleSearchUsers(w http.ResponseWriter, r *http.Request) {
	if s.Search == nil {
		WriteError(w, http.StatusServiceUnavailable, "search_disabled", "Search is not enabled on this instance")
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		WriteError(w, http.StatusBadRequest, "missing_query", "Query parameter 'q' is required")
		return
	}

	limit, offset := parsePagination(r)

	result, err := s.Search.Search(r.Context(), search.SearchRequest{
		Query:  query,
		Index:  search.IndexUsers,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		s.Logger.Error("search users failed", "error", err.Error())
		WriteError(w, http.StatusInternalServerError, "search_error", "Search query failed")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"hits":              result.Hits,
		"estimated_total":   result.EstimatedTotal,
		"processing_time_ms": result.ProcessingTimeMs,
	})
}

// handleSearchGuilds handles GET /api/v1/search/guilds.
// Query params: q (required), limit, offset.
func (s *Server) handleSearchGuilds(w http.ResponseWriter, r *http.Request) {
	if s.Search == nil {
		WriteError(w, http.StatusServiceUnavailable, "search_disabled", "Search is not enabled on this instance")
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		WriteError(w, http.StatusBadRequest, "missing_query", "Query parameter 'q' is required")
		return
	}

	limit, offset := parsePagination(r)

	result, err := s.Search.Search(r.Context(), search.SearchRequest{
		Query:  query,
		Index:  search.IndexGuilds,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		s.Logger.Error("search guilds failed", "error", err.Error())
		WriteError(w, http.StatusInternalServerError, "search_error", "Search query failed")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"hits":              result.Hits,
		"estimated_total":   result.EstimatedTotal,
		"processing_time_ms": result.ProcessingTimeMs,
	})
}

// parsePagination extracts limit and offset from query parameters with defaults.
func parsePagination(r *http.Request) (int64, int64) {
	limit := int64(20)
	offset := int64(0)

	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.ParseInt(l, 10, 64); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.ParseInt(o, 10, 64); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	return limit, offset
}
