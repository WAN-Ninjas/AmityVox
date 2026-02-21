package api

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// federationMediaClient is a shared HTTP client with a 30-second timeout for
// fetching media from remote federated instances. The timeout is generous
// because remote media (avatars, attachments) can be large.
var federationMediaClient = &http.Client{Timeout: 30 * time.Second}

// handleFederationMediaProxy proxies media from federated instances so browsers
// don't hit CORS errors when loading remote avatars, attachments, and embeds.
// The instanceID is validated against the instances table before any external
// request is made, preventing SSRF via arbitrary host access.
//
// GET /api/v1/federation/media/{instanceId}/{fileId}
func (s *Server) handleFederationMediaProxy(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "instanceId")
	fileID := chi.URLParam(r, "fileId")

	if instanceID == "" || fileID == "" {
		WriteError(w, http.StatusBadRequest, "missing_params", "Both instanceId and fileId are required")
		return
	}

	// Reject fileId values that contain path traversal sequences or directory
	// separators. Even though the instanceID is validated against the database
	// (preventing SSRF against arbitrary hosts), a crafted fileId could still
	// manipulate the remote path and reach unintended endpoints.
	if strings.Contains(fileID, "..") || strings.Contains(fileID, "/") || strings.Contains(fileID, "\\") {
		WriteError(w, http.StatusBadRequest, "invalid_file_id", "fileId must not contain path traversal characters")
		return
	}

	// Look up the instance domain from the database. This validates that the
	// instanceID is a known federation peer, preventing SSRF against arbitrary hosts.
	var domain string
	err := s.DB.Pool.QueryRow(r.Context(),
		`SELECT domain FROM instances WHERE id = $1`, instanceID).Scan(&domain)
	if err != nil {
		if err == pgx.ErrNoRows {
			WriteError(w, http.StatusNotFound, "not_found", "Unknown instance")
		} else {
			s.Logger.Error("federation media proxy: failed to look up instance",
				slog.String("instance_id", instanceID),
				slog.String("error", err.Error()))
			WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to look up instance")
		}
		return
	}

	// Fetch the file from the remote instance's public file endpoint.
	remoteURL := fmt.Sprintf("https://%s/api/v1/files/%s", domain, fileID)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, remoteURL, nil)
	if err != nil {
		s.Logger.Error("federation media proxy: failed to create request",
			slog.String("url", remoteURL),
			slog.String("error", err.Error()))
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create remote request")
		return
	}
	req.Header.Set("User-Agent", "AmityVox/1.0 (+federation-media-proxy)")

	// Forward Range header from the client for partial content requests.
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}

	resp, err := federationMediaClient.Do(req)
	if err != nil {
		s.Logger.Error("federation media proxy: fetch failed",
			slog.String("url", remoteURL),
			slog.String("error", err.Error()))
		WriteError(w, http.StatusBadGateway, "bad_gateway", "Failed to fetch remote media")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		// Map remote 404 to local 404; anything else becomes 502.
		if resp.StatusCode == http.StatusNotFound {
			WriteError(w, http.StatusNotFound, "not_found", "Remote media not found")
		} else {
			s.Logger.Warn("federation media proxy: remote returned non-200",
				slog.String("url", remoteURL),
				slog.Int("status", resp.StatusCode))
			WriteError(w, http.StatusBadGateway, "bad_gateway", "Remote instance returned an error")
		}
		return
	}

	// Stream the response to the client with appropriate headers.
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		w.Header().Set("Content-Length", cl)
	}

	// Forward range-related headers for partial content support.
	if cr := resp.Header.Get("Content-Range"); cr != "" {
		w.Header().Set("Content-Range", cr)
	}
	if ar := resp.Header.Get("Accept-Ranges"); ar != "" {
		w.Header().Set("Accept-Ranges", ar)
	}

	// Cache aggressively — media files are identified by ULID and are immutable.
	w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	w.Header().Set("X-Federation-Instance", domain)

	// Use the upstream status code (200 or 206 for partial content).
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
