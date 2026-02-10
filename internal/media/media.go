// Package media handles file uploads, S3 storage operations, image thumbnail
// generation, and media transcoding dispatch. It uses minio-go as a generic S3
// client compatible with Garage, MinIO, AWS S3, and other S3-compatible backends.
package media

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/amityvox/amityvox/internal/auth"
	"github.com/amityvox/amityvox/internal/models"
)

// Config holds the configuration for the media storage service.
type Config struct {
	Endpoint     string
	Bucket       string
	AccessKey    string
	SecretKey    string
	Region       string
	UseSSL       bool
	MaxUploadMB  int64 // maximum file size in megabytes
	Pool         *pgxpool.Pool
	Logger       *slog.Logger
}

// Service provides file upload and S3 storage operations.
type Service struct {
	client      *minio.Client
	bucket      string
	maxUpload   int64 // bytes
	pool        *pgxpool.Pool
	logger      *slog.Logger
}

// New creates a new media service connected to S3-compatible storage.
func New(cfg Config) (*Service, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
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

	return &Service{
		client:    client,
		bucket:    cfg.Bucket,
		maxUpload: maxBytes,
		pool:      cfg.Pool,
		logger:    cfg.Logger,
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

	// Limit request body to max upload size + overhead for multipart headers.
	r.Body = http.MaxBytesReader(w, r.Body, s.maxUpload+4096)

	if err := r.ParseMultipartForm(s.maxUpload); err != nil {
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

	// Validate content type by sniffing the first 512 bytes.
	sniffBuf := make([]byte, 512)
	n, _ := file.Read(sniffBuf)
	contentType := http.DetectContentType(sniffBuf[:n])
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to process file")
		return
	}

	// Use the provided content type if available and reasonable.
	if header.Header.Get("Content-Type") != "" && header.Header.Get("Content-Type") != "application/octet-stream" {
		contentType = header.Header.Get("Content-Type")
	}

	// Generate attachment ID and S3 key.
	attachmentID := models.NewULID().String()
	ext := path.Ext(header.Filename)
	s3Key := fmt.Sprintf("attachments/%s/%s%s", time.Now().UTC().Format("2006/01/02"), attachmentID, ext)

	// Upload to S3.
	_, err = s.client.PutObject(r.Context(), s.bucket, s3Key, file, header.Size,
		minio.PutObjectOptions{
			ContentType: contentType,
			UserMetadata: map[string]string{
				"uploader-id":     userID,
				"original-name":   header.Filename,
				"attachment-id":   attachmentID,
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
	_, err = s.pool.Exec(r.Context(),
		`INSERT INTO files (id, uploader_id, filename, content_type, size_bytes, s3_bucket, s3_key, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		attachmentID, userID, header.Filename, contentType, header.Size,
		s.bucket, s3Key, now,
	)
	if err != nil {
		s.logger.Error("failed to record file in database",
			slog.String("error", err.Error()),
			slog.String("id", attachmentID),
		)
		// File is uploaded but not recorded — log for manual cleanup.
		writeError(w, http.StatusInternalServerError, "internal_error", "File uploaded but metadata save failed")
		return
	}

	// Determine width/height for images using DecodeConfig (reads headers only).
	var width, height *int
	if strings.HasPrefix(contentType, "image/") {
		if _, err := file.Seek(0, io.SeekStart); err == nil {
			if cfg, _, err := image.DecodeConfig(file); err == nil {
				w, h := cfg.Width, cfg.Height
				width = &w
				height = &h
			}
		}
	}

	attachment := models.Attachment{
		ID:          attachmentID,
		UploaderID:  &userID,
		Filename:    header.Filename,
		ContentType: contentType,
		SizeBytes:   header.Size,
		Width:       width,
		Height:      height,
		S3Bucket:    s.bucket,
		S3Key:       s3Key,
		CreatedAt:   now,
	}

	writeJSON(w, http.StatusCreated, attachment)
}

// Delete removes a file from S3 and the database.
func (s *Service) Delete(ctx context.Context, attachmentID string) error {
	var s3Key string
	err := s.pool.QueryRow(ctx,
		`SELECT s3_key FROM files WHERE id = $1`, attachmentID).Scan(&s3Key)
	if err != nil {
		return fmt.Errorf("looking up file %s: %w", attachmentID, err)
	}

	if err := s.client.RemoveObject(ctx, s.bucket, s3Key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("removing S3 object %s: %w", s3Key, err)
	}

	if _, err := s.pool.Exec(ctx, `DELETE FROM files WHERE id = $1`, attachmentID); err != nil {
		return fmt.Errorf("deleting file record %s: %w", attachmentID, err)
	}

	return nil
}

// HealthCheck verifies S3 connectivity.
func (s *Service) HealthCheck(ctx context.Context) error {
	_, err := s.client.BucketExists(ctx, s.bucket)
	return err
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{"code": code, "message": message},
	})
}
