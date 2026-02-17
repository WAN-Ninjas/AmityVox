// Package media handles file uploads, S3 storage operations, image thumbnail
// generation, blurhash computation, EXIF stripping, and media transcoding dispatch.
// It uses minio-go as a generic S3 client compatible with Garage, MinIO, AWS S3,
// and other S3-compatible backends.
package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/buckket/go-blurhash"
	"github.com/go-chi/chi/v5"
	xdraw "golang.org/x/image/draw"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/models"
	"github.com/amityvox/amityvox/internal/permissions"
)

// Config holds the configuration for the media storage service.
type Config struct {
	Endpoint       string
	Bucket         string
	AccessKey      string
	SecretKey      string
	Region         string
	UseSSL         bool
	MaxUploadMB    int64 // maximum file size in megabytes
	ThumbnailSizes []int // e.g. [128, 256, 512]
	StripExif      bool
	Pool           *pgxpool.Pool
	Logger         *slog.Logger
}

// Service provides file upload, image processing, and S3 storage operations.
type Service struct {
	client         *minio.Client
	bucket         string
	maxUpload      int64 // bytes
	thumbnailSizes []int
	stripExif      bool
	pool           *pgxpool.Pool
	logger         *slog.Logger
}

// New creates a new media service connected to S3-compatible storage.
func New(cfg Config) (*Service, error) {
	// minio.New expects host:port without scheme; strip http:// or https:// if present.
	endpoint := cfg.Endpoint
	if strings.HasPrefix(endpoint, "http://") {
		endpoint = strings.TrimPrefix(endpoint, "http://")
	} else if strings.HasPrefix(endpoint, "https://") {
		endpoint = strings.TrimPrefix(endpoint, "https://")
		cfg.UseSSL = true
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("creating S3 client: %w", err)
	}

	maxBytes := cfg.MaxUploadMB * 1024 * 1024
	if maxBytes <= 0 {
		maxBytes = 100 * 1024 * 1024 // default 100MB
	}

	thumbSizes := cfg.ThumbnailSizes
	if len(thumbSizes) == 0 {
		thumbSizes = []int{128, 256, 512}
	}

	return &Service{
		client:         client,
		bucket:         cfg.Bucket,
		maxUpload:      maxBytes,
		thumbnailSizes: thumbSizes,
		stripExif:      cfg.StripExif,
		pool:           cfg.Pool,
		logger:         cfg.Logger,
	}, nil
}

// EnsureBucket creates the storage bucket if it doesn't exist.
func (s *Service) EnsureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("checking bucket existence: %w", err)
	}
	if !exists {
		if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
			return fmt.Errorf("creating bucket %q: %w", s.bucket, err)
		}
		s.logger.Info("created S3 bucket", slog.String("bucket", s.bucket))
	}
	return nil
}

// HandleUpload handles POST /api/v1/files/upload.
// Accepts multipart/form-data with a "file" field.
func (s *Service) HandleUpload(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	// Limit request body to max upload size + 1MB overhead for multipart headers/boundaries.
	r.Body = http.MaxBytesReader(w, r.Body, s.maxUpload+1024*1024)

	if err := r.ParseMultipartForm(s.maxUpload); err != nil {
		s.logger.Warn("multipart parse failed",
			slog.String("error", err.Error()),
			slog.String("user_id", userID),
			slog.Int64("max_upload_bytes", s.maxUpload),
			slog.String("content_length", r.Header.Get("Content-Length")))
		writeError(w, http.StatusBadRequest, "file_too_large",
			fmt.Sprintf("File exceeds maximum upload size (%dMB)", s.maxUpload/(1024*1024)))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing_file", "No file provided in 'file' form field")
		return
	}
	defer file.Close()

	// Read optional alt text for accessibility.
	altText := r.FormValue("alt_text")

	// Read entire file into memory for processing.
	fileData, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read file")
		return
	}

	// Determine content type by sniffing the first 512 bytes (authoritative).
	contentType := http.DetectContentType(fileData)

	// Only allow user-provided content type for safe, non-scriptable media types.
	if ct := header.Header.Get("Content-Type"); ct != "" && ct != "application/octet-stream" {
		// Allow user type only for image/audio/video subtypes, never text/html, application/*, svg, etc.
		if strings.HasPrefix(ct, "image/") && ct != "image/svg+xml" {
			contentType = ct
		} else if strings.HasPrefix(ct, "audio/") || strings.HasPrefix(ct, "video/") {
			contentType = ct
		}
	}

	// Generate attachment ID and S3 key.
	attachmentID := models.NewULID().String()
	ext := path.Ext(header.Filename)
	datePath := time.Now().UTC().Format("2006/01/02")
	s3Key := fmt.Sprintf("attachments/%s/%s%s", datePath, attachmentID, ext)

	isImage := strings.HasPrefix(contentType, "image/")

	// Strip EXIF metadata from images by re-encoding.
	var width, height *int
	var bhash *string
	uploadData := fileData

	if isImage {
		result := s.processImage(fileData, contentType)
		width = result.width
		height = result.height
		bhash = result.blurhash
		if result.stripped != nil {
			uploadData = result.stripped
		}
	}

	// Upload to S3.
	uploadSize := int64(len(uploadData))
	_, err = s.client.PutObject(r.Context(), s.bucket, s3Key,
		bytes.NewReader(uploadData), uploadSize,
		minio.PutObjectOptions{
			ContentType: contentType,
			UserMetadata: map[string]string{
				"uploader-id":   userID,
				"original-name": header.Filename,
				"attachment-id": attachmentID,
			},
		})
	if err != nil {
		s.logger.Error("S3 upload failed",
			slog.String("error", err.Error()),
			slog.String("key", s3Key),
		)
		writeError(w, http.StatusInternalServerError, "upload_failed", "Failed to upload file to storage")
		return
	}

	// Record in database.
	now := time.Now().UTC()
	var altTextPtr *string
	if altText != "" {
		altTextPtr = &altText
	}
	_, err = s.pool.Exec(r.Context(),
		`INSERT INTO attachments (id, uploader_id, filename, content_type, size_bytes, width, height, blurhash, s3_bucket, s3_key, alt_text, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		attachmentID, userID, header.Filename, contentType, uploadSize,
		width, height, bhash, s.bucket, s3Key, altTextPtr, now,
	)
	if err != nil {
		s.logger.Error("failed to record file in database",
			slog.String("error", err.Error()),
			slog.String("id", attachmentID),
		)
		writeError(w, http.StatusInternalServerError, "internal_error", "File uploaded but metadata save failed")
		return
	}

	// Generate thumbnails asynchronously (non-blocking).
	if isImage && width != nil {
		go s.generateThumbnails(context.Background(), fileData, attachmentID, datePath)
	}

	attachment := models.Attachment{
		ID:          attachmentID,
		UploaderID:  &userID,
		Filename:    header.Filename,
		ContentType: contentType,
		SizeBytes:   uploadSize,
		Width:       width,
		Height:      height,
		Blurhash:    bhash,
		S3Bucket:    s.bucket,
		S3Key:       s3Key,
		AltText:     altTextPtr,
		CreatedAt:   now,
	}

	writeJSON(w, http.StatusCreated, attachment)
}

// imageResult holds the output of image processing.
type imageResult struct {
	width    *int
	height   *int
	blurhash *string
	stripped []byte // EXIF-stripped re-encoded image (nil if unchanged)
}

// processImage decodes an image, computes dimensions and blurhash, and optionally
// strips EXIF metadata by re-encoding. This is done synchronously during upload
// since it only requires decoding the image once.
func (s *Service) processImage(data []byte, contentType string) imageResult {
	var result imageResult

	// Decode the image.
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		if s.logger != nil {
			s.logger.Debug("failed to decode image for processing", slog.String("error", err.Error()))
		}
		return result
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	result.width = &w
	result.height = &h

	// Compute blurhash from a downscaled version for performance.
	bhash := ComputeBlurhash(img)
	if bhash != "" {
		result.blurhash = &bhash
	}

	// Strip EXIF by re-encoding (only pixel data is preserved).
	if s.stripExif {
		stripped := stripExifData(img, contentType)
		if stripped != nil {
			result.stripped = stripped
		}
	}

	return result
}

// resizeWidth scales an image to the given width, preserving aspect ratio,
// using high-quality CatmullRom interpolation.
func resizeWidth(src image.Image, width int) image.Image {
	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW == 0 || width <= 0 {
		return src
	}
	height := (srcH * width) / srcW
	if height == 0 {
		height = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, xdraw.Over, nil)
	return dst
}

// ComputeBlurhash generates a blurhash string from an image.
// Uses 4x3 components for a good balance of quality and string length.
func ComputeBlurhash(img image.Image) string {
	// Downscale to 64px wide for fast blurhash computation.
	small := resizeWidth(img, 64)

	// Convert to NRGBA for consistent pixel access.
	bounds := small.Bounds()
	nrgba := image.NewNRGBA(bounds)
	xdraw.Draw(nrgba, bounds, small, bounds.Min, xdraw.Src)

	hash, err := blurhash.Encode(4, 3, nrgba)
	if err != nil {
		return ""
	}
	return hash
}

// stripExifData re-encodes an image to strip EXIF metadata.
// Returns the re-encoded bytes, or nil if re-encoding fails.
func stripExifData(img image.Image, contentType string) []byte {
	var buf bytes.Buffer

	switch contentType {
	case "image/jpeg":
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 92}); err != nil {
			return nil
		}
	case "image/png":
		if err := png.Encode(&buf, img); err != nil {
			return nil
		}
	default:
		// For other formats (GIF, WebP, etc.), encode as PNG to strip metadata.
		if err := png.Encode(&buf, img); err != nil {
			return nil
		}
	}

	return buf.Bytes()
}

// generateThumbnails creates resized versions of an image at configured sizes
// and uploads them to S3. Runs in a background goroutine.
func (s *Service) generateThumbnails(ctx context.Context, data []byte, attachmentID, datePath string) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		s.logger.Error("failed to decode image for thumbnails", slog.String("error", err.Error()))
		return
	}

	bounds := img.Bounds()
	origW := bounds.Dx()

	for _, size := range s.thumbnailSizes {
		// Skip thumbnail sizes larger than the original.
		if size >= origW {
			continue
		}

		thumb := resizeWidth(img, size)

		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, thumb, &jpeg.Options{Quality: 85}); err != nil {
			s.logger.Error("failed to encode thumbnail",
				slog.String("attachment_id", attachmentID),
				slog.Int("size", size),
				slog.String("error", err.Error()),
			)
			continue
		}

		thumbKey := fmt.Sprintf("thumbnails/%s/%s_%d.jpg", datePath, attachmentID, size)
		_, err := s.client.PutObject(ctx, s.bucket, thumbKey,
			bytes.NewReader(buf.Bytes()), int64(buf.Len()),
			minio.PutObjectOptions{
				ContentType: "image/jpeg",
				UserMetadata: map[string]string{
					"attachment-id":  attachmentID,
					"thumbnail-size": fmt.Sprintf("%d", size),
				},
			})
		if err != nil {
			s.logger.Error("failed to upload thumbnail",
				slog.String("attachment_id", attachmentID),
				slog.Int("size", size),
				slog.String("error", err.Error()),
			)
			continue
		}

		s.logger.Debug("thumbnail generated",
			slog.String("attachment_id", attachmentID),
			slog.Int("size", size),
			slog.String("key", thumbKey),
		)
	}
}

// ThumbnailURL returns the S3 key for a specific thumbnail size.
func ThumbnailURL(attachmentID, datePath string, size int) string {
	return fmt.Sprintf("thumbnails/%s/%s_%d.jpg", datePath, attachmentID, size)
}

// Delete removes a file and its thumbnails from S3 and the database.
func (s *Service) Delete(ctx context.Context, attachmentID string) error {
	var s3Key string
	err := s.pool.QueryRow(ctx,
		`SELECT s3_key FROM attachments WHERE id = $1`, attachmentID).Scan(&s3Key)
	if err != nil {
		return fmt.Errorf("looking up file %s: %w", attachmentID, err)
	}

	if err := s.client.RemoveObject(ctx, s.bucket, s3Key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("removing S3 object %s: %w", s3Key, err)
	}

	// Clean up thumbnails (best-effort, they share a date path).
	datePath := extractDatePath(s3Key)
	for _, size := range s.thumbnailSizes {
		thumbKey := ThumbnailURL(attachmentID, datePath, size)
		_ = s.client.RemoveObject(ctx, s.bucket, thumbKey, minio.RemoveObjectOptions{})
	}

	if _, err := s.pool.Exec(ctx, `DELETE FROM attachments WHERE id = $1`, attachmentID); err != nil {
		return fmt.Errorf("deleting file record %s: %w", attachmentID, err)
	}

	return nil
}

// HealthCheck verifies S3 connectivity.
func (s *Service) HealthCheck(ctx context.Context) error {
	_, err := s.client.BucketExists(ctx, s.bucket)
	return err
}

// extractDatePath extracts the YYYY/MM/DD portion from an S3 key like "attachments/2026/02/10/xxx.jpg".
func extractDatePath(s3Key string) string {
	parts := strings.Split(s3Key, "/")
	if len(parts) >= 4 {
		return strings.Join(parts[1:4], "/")
	}
	return time.Now().UTC().Format("2006/01/02")
}

// HandleGetFile serves a file by its attachment ID.
// GET /api/v1/files/{fileID}
func (s *Service) HandleGetFile(w http.ResponseWriter, r *http.Request) {
	fileID := r.PathValue("fileID")
	if fileID == "" {
		// Fallback for chi router.
		fileID = chi.URLParam(r, "fileID")
	}

	var filename, contentType, s3Key string
	var sizeBytes int64
	err := s.pool.QueryRow(r.Context(),
		`SELECT filename, content_type, size_bytes, s3_key FROM attachments WHERE id = $1`, fileID,
	).Scan(&filename, &contentType, &sizeBytes, &s3Key)
	if err != nil {
		writeError(w, http.StatusNotFound, "file_not_found", "File not found")
		return
	}

	obj, err := s.client.GetObject(r.Context(), s.bucket, s3Key, minio.GetObjectOptions{})
	if err != nil {
		s.logger.Error("failed to get file from S3",
			slog.String("error", err.Error()),
			slog.String("key", s3Key),
		)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to retrieve file")
		return
	}
	defer obj.Close()

	// Sanitize filename: strip path separators and control characters, replace quotes.
	safeFilename := sanitizeFilename(filename)

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, safeFilename))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	io.Copy(w, obj)
}

// HandleUpdateAttachment updates metadata on an attachment (nsfw, alt_text, description).
// PATCH /api/v1/files/{fileID}
func (s *Service) HandleUpdateAttachment(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	fileID := chi.URLParam(r, "fileID")

	// Look up the attachment and its associated channel for permission checks.
	var uploaderID *string
	var channelID *string
	var guildID *string
	err := s.pool.QueryRow(r.Context(),
		`SELECT a.uploader_id, m.channel_id, c.guild_id
		 FROM attachments a
		 LEFT JOIN messages m ON m.id = a.message_id
		 LEFT JOIN channels c ON c.id = m.channel_id
		 WHERE a.id = $1`, fileID,
	).Scan(&uploaderID, &channelID, &guildID)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "file_not_found", "Attachment not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to look up attachment")
		return
	}

	// Verify requester is the uploader or has ManageMessages permission.
	isOwner := uploaderID != nil && *uploaderID == userID
	if !isOwner && guildID != nil {
		if !s.hasGuildPermission(r.Context(), *guildID, userID, permissions.ManageMessages) {
			writeError(w, http.StatusForbidden, "forbidden", "You can only edit your own attachments or need ManageMessages permission")
			return
		}
	} else if !isOwner {
		writeError(w, http.StatusForbidden, "forbidden", "You can only edit your own attachments")
		return
	}

	var req struct {
		NSFW        *bool   `json:"nsfw"`
		AltText     *string `json:"alt_text"`
		Description *string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	_, err = s.pool.Exec(r.Context(),
		`UPDATE attachments SET
		   nsfw = COALESCE($1, nsfw),
		   alt_text = COALESCE($2, alt_text),
		   description = COALESCE($3, description)
		 WHERE id = $4`,
		req.NSFW, req.AltText, req.Description, fileID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update attachment")
		return
	}

	// Return the updated attachment.
	var a models.Attachment
	err = s.pool.QueryRow(r.Context(),
		`SELECT id, message_id, uploader_id, filename, content_type, size_bytes,
		        width, height, duration_seconds, s3_bucket, s3_key, blurhash,
		        alt_text, nsfw, description, created_at
		 FROM attachments WHERE id = $1`, fileID,
	).Scan(
		&a.ID, &a.MessageID, &a.UploaderID, &a.Filename, &a.ContentType, &a.SizeBytes,
		&a.Width, &a.Height, &a.DurationSeconds, &a.S3Bucket, &a.S3Key, &a.Blurhash,
		&a.AltText, &a.NSFW, &a.Description, &a.CreatedAt,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to read updated attachment")
		return
	}

	writeJSON(w, http.StatusOK, a)
}

// HandleDeleteAttachment deletes an attachment from the database and S3.
// DELETE /api/v1/files/{fileID}
func (s *Service) HandleDeleteAttachment(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	fileID := chi.URLParam(r, "fileID")

	// Look up the attachment and its associated channel for permission checks.
	var uploaderID *string
	var guildID *string
	err := s.pool.QueryRow(r.Context(),
		`SELECT a.uploader_id, c.guild_id
		 FROM attachments a
		 LEFT JOIN messages m ON m.id = a.message_id
		 LEFT JOIN channels c ON c.id = m.channel_id
		 WHERE a.id = $1`, fileID,
	).Scan(&uploaderID, &guildID)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "file_not_found", "Attachment not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to look up attachment")
		return
	}

	// Verify requester is the uploader or has ManageMessages permission.
	isOwner := uploaderID != nil && *uploaderID == userID
	if !isOwner && guildID != nil {
		if !s.hasGuildPermission(r.Context(), *guildID, userID, permissions.ManageMessages) {
			writeError(w, http.StatusForbidden, "forbidden", "You can only delete your own attachments or need ManageMessages permission")
			return
		}
	} else if !isOwner {
		writeError(w, http.StatusForbidden, "forbidden", "You can only delete your own attachments")
		return
	}

	// Delete from S3 and database using the existing Delete method.
	if err := s.Delete(r.Context(), fileID); err != nil {
		s.logger.Error("failed to delete attachment", slog.String("error", err.Error()), slog.String("id", fileID))
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete attachment")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleTagAttachment adds a tag to an attachment.
// PUT /api/v1/files/{fileID}/tags/{tagID}
func (s *Service) HandleTagAttachment(w http.ResponseWriter, r *http.Request) {
	fileID := chi.URLParam(r, "fileID")
	tagID := chi.URLParam(r, "tagID")

	// Verify attachment exists.
	var exists bool
	s.pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM attachments WHERE id = $1)`, fileID).Scan(&exists)
	if !exists {
		writeError(w, http.StatusNotFound, "file_not_found", "Attachment not found")
		return
	}

	// Verify tag exists.
	s.pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM media_tags WHERE id = $1)`, tagID).Scan(&exists)
	if !exists {
		writeError(w, http.StatusNotFound, "tag_not_found", "Media tag not found")
		return
	}

	_, err := s.pool.Exec(r.Context(),
		`INSERT INTO attachment_tags (attachment_id, tag_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		fileID, tagID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to tag attachment")
		return
	}

	writeJSON(w, http.StatusOK, models.AttachmentTag{AttachmentID: fileID, TagID: tagID})
}

// HandleUntagAttachment removes a tag from an attachment.
// DELETE /api/v1/files/{fileID}/tags/{tagID}
func (s *Service) HandleUntagAttachment(w http.ResponseWriter, r *http.Request) {
	fileID := chi.URLParam(r, "fileID")
	tagID := chi.URLParam(r, "tagID")

	_, err := s.pool.Exec(r.Context(),
		`DELETE FROM attachment_tags WHERE attachment_id = $1 AND tag_id = $2`,
		fileID, tagID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to untag attachment")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// hasGuildPermission checks if a user has a specific permission in a guild.
func (s *Service) hasGuildPermission(ctx context.Context, guildID, userID string, perm uint64) bool {
	// Check if user is the guild owner (owner has all permissions).
	var ownerID string
	err := s.pool.QueryRow(ctx, `SELECT owner_id FROM guilds WHERE id = $1`, guildID).Scan(&ownerID)
	if err != nil {
		return false
	}
	if ownerID == userID {
		return true
	}

	// Get guild default permissions.
	var defaultPerms int64
	s.pool.QueryRow(ctx, `SELECT default_permissions FROM guilds WHERE id = $1`, guildID).Scan(&defaultPerms)
	computedPerms := uint64(defaultPerms)

	// Apply member's role permissions.
	rows, _ := s.pool.Query(ctx,
		`SELECT r.permissions_allow, r.permissions_deny
		 FROM roles r
		 JOIN member_roles mr ON r.id = mr.role_id
		 WHERE mr.guild_id = $1 AND mr.user_id = $2
		 ORDER BY r.position DESC`,
		guildID, userID,
	)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var allow, deny int64
			rows.Scan(&allow, &deny)
			computedPerms |= uint64(allow)
			computedPerms &^= uint64(deny)
		}
	}

	if computedPerms&permissions.Administrator != 0 {
		return true
	}
	return computedPerms&perm != 0
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
}

// sanitizeFilename removes path separators, control characters, and quotes from
// a filename to prevent header injection in Content-Disposition.
func sanitizeFilename(name string) string {
	// Extract just the base filename (no directory traversal).
	name = path.Base(name)
	// Replace quotes and backslashes that could break the header value.
	replacer := strings.NewReplacer(`"`, "_", `\`, "_", "\n", "", "\r", "")
	name = replacer.Replace(name)
	// Remove any remaining control characters.
	var safe strings.Builder
	for _, r := range name {
		if r >= 32 {
			safe.WriteRune(r)
		}
	}
	result := safe.String()
	if result == "" || result == "." || result == ".." {
		return "file"
	}
	return result
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{"code": code, "message": message},
	})
}
