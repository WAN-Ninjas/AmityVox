package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

// federationMediaClient is a shared HTTP client with a 30-second timeout for
// fetching media from remote federated instances. The timeout is generous
// because remote media (avatars, attachments) can be large.
var federationMediaClient = &http.Client{Timeout: 30 * time.Second}

// fedMediaCacheTTL is the DragonflyDB cache TTL for small federation media files.
const fedMediaCacheTTL = 1 * time.Hour

// fedMediaMaxCacheableSize is the maximum file size cached in DragonflyDB (1MB).
const fedMediaMaxCacheableSize = 1 << 20

// fedMediaMeta stores content-type metadata alongside cached media.
type fedMediaMeta struct {
	ContentType   string `json:"ct"`
	ContentLength int    `json:"cl"`
}

// handleFederationMediaProxy proxies media from federated instances so browsers
// don't hit CORS errors when loading remote avatars, attachments, and embeds.
// The instanceID is validated against the instances table before any external
// request is made, preventing SSRF via arbitrary host access.
//
// Caching: small files (≤1MB) are cached in DragonflyDB with 1h TTL.
// Large files (>1MB) are cached on disk with LRU eviction.
// Range requests are passed through without caching.
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
	if strings.Contains(instanceID, "..") || strings.Contains(instanceID, "/") || strings.Contains(instanceID, "\\") {
		WriteError(w, http.StatusBadRequest, "invalid_instance_id", "instanceId must not contain path traversal characters")
		return
	}

	isRangeRequest := r.Header.Get("Range") != ""
	cacheKey := fmt.Sprintf("fed:media:%s:%s", instanceID, fileID)

	// Try DragonflyDB cache first (only for non-range requests).
	if !isRangeRequest {
		if served := s.serveFedMediaFromCache(w, r.Context(), cacheKey); served {
			return
		}
	}

	// Try disk cache (only for non-range requests).
	if !isRangeRequest {
		cacheDir := s.Config.Federation.MediaCacheDir
		if cacheDir != "" {
			if served := s.serveFedMediaFromDisk(w, cacheDir, instanceID, fileID); served {
				return
			}
		}
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
	if isRangeRequest {
		req.Header.Set("Range", r.Header.Get("Range"))
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

	contentType := resp.Header.Get("Content-Type")
	contentLength, _ := strconv.Atoi(resp.Header.Get("Content-Length"))

	// Set response headers.
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if contentLength > 0 {
		w.Header().Set("Content-Length", strconv.Itoa(contentLength))
	}
	if cr := resp.Header.Get("Content-Range"); cr != "" {
		w.Header().Set("Content-Range", cr)
	}
	if ar := resp.Header.Get("Accept-Ranges"); ar != "" {
		w.Header().Set("Accept-Ranges", ar)
	}
	w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	w.Header().Set("X-Federation-Instance", domain)

	// Range requests or unknown-size responses: stream directly, no caching.
	if isRangeRequest || resp.StatusCode == http.StatusPartialContent {
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
		return
	}

	// Cacheable: read body and cache based on size.
	if contentLength > 0 && contentLength <= fedMediaMaxCacheableSize {
		// Small file — cache in DragonflyDB.
		body, err := io.ReadAll(io.LimitReader(resp.Body, fedMediaMaxCacheableSize+1))
		if err != nil {
			s.Logger.Warn("federation media proxy: failed to read body for caching",
				slog.String("error", err.Error()))
			WriteError(w, http.StatusBadGateway, "bad_gateway", "Failed to read remote media")
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(body)
		// Cache asynchronously to not block response.
		go s.cacheFedMediaInRedis(cacheKey, contentType, body)
		return
	}

	// Large file or unknown size — stream to client + cache on disk.
	cacheDir := s.Config.Federation.MediaCacheDir
	if cacheDir != "" && contentLength > fedMediaMaxCacheableSize {
		w.WriteHeader(http.StatusOK)
		s.streamAndCacheFedMediaToDisk(w, resp.Body, cacheDir, instanceID, fileID, contentType, contentLength)
		return
	}

	// Fallback: stream directly.
	w.WriteHeader(http.StatusOK)
	io.Copy(w, resp.Body)
}

// serveFedMediaFromCache serves a federation media file from DragonflyDB if cached.
func (s *Server) serveFedMediaFromCache(w http.ResponseWriter, ctx context.Context, cacheKey string) bool {
	rdb := s.Cache.Client()

	// Check metadata.
	metaKey := cacheKey + ":meta"
	metaJSON, err := rdb.Get(ctx, metaKey).Bytes()
	if err == redis.Nil || err != nil {
		return false
	}

	var meta fedMediaMeta
	if json.Unmarshal(metaJSON, &meta) != nil {
		return false
	}

	// Get body.
	bodyKey := cacheKey + ":body"
	body, err := rdb.Get(ctx, bodyKey).Bytes()
	if err != nil {
		return false
	}

	if meta.ContentType != "" {
		w.Header().Set("Content-Type", meta.ContentType)
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	w.Header().Set("X-Federation-Cache", "hit-redis")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
	return true
}

// cacheFedMediaInRedis stores a small file in DragonflyDB with TTL.
func (s *Server) cacheFedMediaInRedis(cacheKey, contentType string, body []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rdb := s.Cache.Client()
	meta := fedMediaMeta{ContentType: contentType, ContentLength: len(body)}
	metaJSON, _ := json.Marshal(meta)

	pipe := rdb.Pipeline()
	pipe.Set(ctx, cacheKey+":meta", metaJSON, fedMediaCacheTTL)
	pipe.Set(ctx, cacheKey+":body", body, fedMediaCacheTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		s.Logger.Warn("federation media proxy: failed to cache in DragonflyDB",
			slog.String("error", err.Error()))
	}
}

// serveFedMediaFromDisk serves a federation media file from disk cache.
func (s *Server) serveFedMediaFromDisk(w http.ResponseWriter, cacheDir, instanceID, fileID string) bool {
	filePath := filepath.Join(cacheDir, instanceID, fileID)
	metaPath := filePath + ".meta"

	metaJSON, err := os.ReadFile(metaPath)
	if err != nil {
		return false
	}
	var meta fedMediaMeta
	if json.Unmarshal(metaJSON, &meta) != nil {
		return false
	}

	f, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer f.Close()

	// Touch file mtime for LRU tracking.
	now := time.Now()
	os.Chtimes(filePath, now, now)

	if meta.ContentType != "" {
		w.Header().Set("Content-Type", meta.ContentType)
	}
	if meta.ContentLength > 0 {
		w.Header().Set("Content-Length", strconv.Itoa(meta.ContentLength))
	}
	w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	w.Header().Set("X-Federation-Cache", "hit-disk")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, f)
	return true
}

// streamAndCacheFedMediaToDisk streams the response body to the client while
// simultaneously writing it to disk cache.
func (s *Server) streamAndCacheFedMediaToDisk(w http.ResponseWriter, body io.Reader, cacheDir, instanceID, fileID, contentType string, contentLength int) {
	dir := filepath.Join(cacheDir, instanceID)
	os.MkdirAll(dir, 0o755)

	filePath := filepath.Join(dir, fileID)
	tmpPath := filePath + ".tmp"

	f, err := os.Create(tmpPath)
	if err != nil {
		// Can't create cache file — stream directly.
		s.Logger.Warn("federation media proxy: failed to create disk cache file",
			slog.String("error", err.Error()))
		io.Copy(w, body)
		return
	}

	// TeeReader: every byte read for the client is also written to the file.
	tee := io.TeeReader(body, f)
	_, copyErr := io.Copy(w, tee)
	f.Close()

	if copyErr != nil {
		os.Remove(tmpPath)
		return
	}

	// Atomic rename to final path.
	os.Rename(tmpPath, filePath)

	// Write metadata.
	meta := fedMediaMeta{ContentType: contentType, ContentLength: contentLength}
	metaJSON, _ := json.Marshal(meta)
	os.WriteFile(filePath+".meta", metaJSON, 0o644)

	// Async LRU eviction check.
	maxSizeMB := s.Config.Federation.MediaCacheMaxSizeMB
	if maxSizeMB > 0 {
		go s.evictFedMediaDiskCache(cacheDir, int64(maxSizeMB)*1024*1024)
	}
}

// evictFedMediaDiskCache removes oldest files from the disk cache when the
// total size exceeds maxBytes. Files are sorted by mtime (oldest first).
func (s *Server) evictFedMediaDiskCache(cacheDir string, maxBytes int64) {
	type cachedFile struct {
		path  string
		size  int64
		mtime time.Time
	}
	var files []cachedFile
	var totalSize int64

	filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || strings.HasSuffix(path, ".meta") || strings.HasSuffix(path, ".tmp") {
			return nil
		}
		totalSize += info.Size()
		files = append(files, cachedFile{path: path, size: info.Size(), mtime: info.ModTime()})
		return nil
	})

	if totalSize <= maxBytes {
		return
	}

	// Sort oldest first.
	sort.Slice(files, func(i, j int) bool { return files[i].mtime.Before(files[j].mtime) })

	for _, f := range files {
		if totalSize <= maxBytes {
			break
		}
		os.Remove(f.path)
		os.Remove(f.path + ".meta")
		totalSize -= f.size
	}
}
