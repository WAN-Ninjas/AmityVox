package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/amityvox/amityvox/internal/auth"
)

// handleGiphySearch handles GET /api/v1/giphy/search?q=...&limit=...&offset=...
// This proxies the Giphy API so the API key is never exposed to the client.
func (s *Server) handleGiphySearch(w http.ResponseWriter, r *http.Request) {
	if !s.Config.Giphy.Enabled || s.Config.Giphy.APIKey == "" {
		WriteError(w, http.StatusServiceUnavailable, "giphy_disabled", "Giphy integration is not enabled on this instance")
		return
	}

	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		WriteError(w, http.StatusBadRequest, "missing_query", "Query parameter 'q' is required")
		return
	}

	limit := r.URL.Query().Get("limit")
	if limit == "" {
		limit = "25"
	}
	offset := r.URL.Query().Get("offset")
	if offset == "" {
		offset = "0"
	}

	params := url.Values{}
	params.Set("api_key", s.Config.Giphy.APIKey)
	params.Set("q", query)
	params.Set("limit", limit)
	params.Set("offset", offset)
	params.Set("rating", "pg-13")
	params.Set("lang", "en")

	giphyURL := fmt.Sprintf("https://api.giphy.com/v1/gifs/search?%s", params.Encode())
	resp, err := http.Get(giphyURL)
	if err != nil {
		s.Logger.Error("giphy API request failed", "error", err)
		WriteError(w, http.StatusBadGateway, "giphy_error", "Failed to reach Giphy API")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB max
	if err != nil {
		WriteError(w, http.StatusBadGateway, "giphy_error", "Failed to read Giphy response")
		return
	}

	// Parse and re-emit only the data we need (strip API key from any echoed params).
	var giphyResp struct {
		Data []struct {
			ID     string `json:"id"`
			Title  string `json:"title"`
			URL    string `json:"url"`
			Images struct {
				FixedHeight struct {
					URL    string `json:"url"`
					Width  string `json:"width"`
					Height string `json:"height"`
				} `json:"fixed_height"`
				FixedHeightSmall struct {
					URL    string `json:"url"`
					Width  string `json:"width"`
					Height string `json:"height"`
				} `json:"fixed_height_small"`
				Original struct {
					URL    string `json:"url"`
					Width  string `json:"width"`
					Height string `json:"height"`
				} `json:"original"`
			} `json:"images"`
		} `json:"data"`
		Pagination struct {
			TotalCount int `json:"total_count"`
			Count      int `json:"count"`
			Offset     int `json:"offset"`
		} `json:"pagination"`
	}

	if err := json.Unmarshal(body, &giphyResp); err != nil {
		WriteError(w, http.StatusBadGateway, "giphy_error", "Failed to parse Giphy response")
		return
	}

	WriteJSON(w, http.StatusOK, giphyResp)
}

// handleGiphyCategories handles GET /api/v1/giphy/categories?limit=...
// Returns Giphy GIF categories with representative thumbnails.
func (s *Server) handleGiphyCategories(w http.ResponseWriter, r *http.Request) {
	if !s.Config.Giphy.Enabled || s.Config.Giphy.APIKey == "" {
		WriteError(w, http.StatusServiceUnavailable, "giphy_disabled", "Giphy integration is not enabled on this instance")
		return
	}

	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	limit := r.URL.Query().Get("limit")
	if limit == "" {
		limit = "15"
	}

	params := url.Values{}
	params.Set("api_key", s.Config.Giphy.APIKey)
	params.Set("limit", limit)

	giphyURL := fmt.Sprintf("https://api.giphy.com/v1/gifs/categories?%s", params.Encode())
	resp, err := http.Get(giphyURL)
	if err != nil {
		s.Logger.Error("giphy categories API request failed", "error", err)
		WriteError(w, http.StatusBadGateway, "giphy_error", "Failed to reach Giphy API")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		WriteError(w, http.StatusBadGateway, "giphy_error", "Failed to read Giphy response")
		return
	}

	var giphyResp struct {
		Data []struct {
			Name        string `json:"name"`
			NameEncoded string `json:"name_encoded"`
			Gif         struct {
				ID     string `json:"id"`
				Title  string `json:"title"`
				Images struct {
					FixedHeightSmall struct {
						URL    string `json:"url"`
						Width  string `json:"width"`
						Height string `json:"height"`
					} `json:"fixed_height_small"`
				} `json:"images"`
			} `json:"gif"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &giphyResp); err != nil {
		WriteError(w, http.StatusBadGateway, "giphy_error", "Failed to parse Giphy response")
		return
	}

	WriteJSON(w, http.StatusOK, giphyResp)
}

// handleGiphyTrending handles GET /api/v1/giphy/trending?limit=...&offset=...
func (s *Server) handleGiphyTrending(w http.ResponseWriter, r *http.Request) {
	if !s.Config.Giphy.Enabled || s.Config.Giphy.APIKey == "" {
		WriteError(w, http.StatusServiceUnavailable, "giphy_disabled", "Giphy integration is not enabled on this instance")
		return
	}

	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	limit := r.URL.Query().Get("limit")
	if limit == "" {
		limit = "25"
	}
	offset := r.URL.Query().Get("offset")
	if offset == "" {
		offset = "0"
	}

	params := url.Values{}
	params.Set("api_key", s.Config.Giphy.APIKey)
	params.Set("limit", limit)
	params.Set("offset", offset)
	params.Set("rating", "pg-13")

	giphyURL := fmt.Sprintf("https://api.giphy.com/v1/gifs/trending?%s", params.Encode())
	resp, err := http.Get(giphyURL)
	if err != nil {
		s.Logger.Error("giphy trending API request failed", "error", err)
		WriteError(w, http.StatusBadGateway, "giphy_error", "Failed to reach Giphy API")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		WriteError(w, http.StatusBadGateway, "giphy_error", "Failed to read Giphy response")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}
