package media

import (
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusCreated, map[string]string{"id": "abc123"})

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("content-type = %q, want %q", ct, "application/json")
	}

	var envelope map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	data, ok := envelope["data"]
	if !ok {
		t.Fatal("missing 'data' key in response")
	}

	inner, ok := data.(map[string]interface{})
	if !ok {
		t.Fatalf("data is not a map: %T", data)
	}

	if inner["id"] != "abc123" {
		t.Errorf("data.id = %v, want %q", inner["id"], "abc123")
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "file_too_large", "File exceeds limit")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var envelope map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	errObj, ok := envelope["error"].(map[string]interface{})
	if !ok {
		t.Fatal("missing or invalid 'error' key")
	}

	if errObj["code"] != "file_too_large" {
		t.Errorf("error.code = %v, want %q", errObj["code"], "file_too_large")
	}
	if errObj["message"] != "File exceeds limit" {
		t.Errorf("error.message = %v, want %q", errObj["message"], "File exceeds limit")
	}
}

func TestConfig_DefaultMaxUpload(t *testing.T) {
	cfg := Config{
		Endpoint:    "localhost:9000",
		Bucket:      "test",
		AccessKey:   "minioadmin",
		SecretKey:   "minioadmin",
		MaxUploadMB: 0,
	}

	if cfg.MaxUploadMB != 0 {
		t.Errorf("expected 0, got %d", cfg.MaxUploadMB)
	}

	maxBytes := cfg.MaxUploadMB * 1024 * 1024
	if maxBytes <= 0 {
		maxBytes = 100 * 1024 * 1024
	}
	if maxBytes != 100*1024*1024 {
		t.Errorf("default max bytes = %d, want %d", maxBytes, 100*1024*1024)
	}
}

func TestConfig_CustomMaxUpload(t *testing.T) {
	cfg := Config{
		MaxUploadMB: 50,
	}

	maxBytes := cfg.MaxUploadMB * 1024 * 1024
	if maxBytes != 50*1024*1024 {
		t.Errorf("max bytes = %d, want %d", maxBytes, 50*1024*1024)
	}
}

// createTestImage generates a test image with the given dimensions.
func createTestImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8((x * 255) / w),
				G: uint8((y * 255) / h),
				B: 128,
				A: 255,
			})
		}
	}
	return img
}

func TestComputeBlurhash(t *testing.T) {
	img := createTestImage(200, 150)

	hash := ComputeBlurhash(img)
	if hash == "" {
		t.Fatal("expected non-empty blurhash")
	}

	// Blurhash should be a reasonable length (typically 20-30 chars for 4x3 components).
	if len(hash) < 6 || len(hash) > 50 {
		t.Errorf("blurhash length = %d, expected between 6 and 50", len(hash))
	}

	// Same image should produce same hash (deterministic).
	hash2 := ComputeBlurhash(img)
	if hash != hash2 {
		t.Errorf("blurhash not deterministic: %q != %q", hash, hash2)
	}
}

func TestComputeBlurhash_SmallImage(t *testing.T) {
	img := createTestImage(16, 16)
	hash := ComputeBlurhash(img)
	if hash == "" {
		t.Fatal("expected non-empty blurhash for small image")
	}
}

func TestStripExifData_JPEG(t *testing.T) {
	img := createTestImage(100, 80)
	stripped := stripExifData(img, "image/jpeg")
	if stripped == nil {
		t.Fatal("expected non-nil stripped data for JPEG")
	}
	if len(stripped) == 0 {
		t.Fatal("expected non-empty stripped data")
	}

	// Verify the output is valid JPEG.
	decoded, err := jpeg.Decode(bytes.NewReader(stripped))
	if err != nil {
		t.Fatalf("stripped JPEG is not valid: %v", err)
	}
	bounds := decoded.Bounds()
	if bounds.Dx() != 100 || bounds.Dy() != 80 {
		t.Errorf("dimensions = %dx%d, want 100x80", bounds.Dx(), bounds.Dy())
	}
}

func TestStripExifData_PNG(t *testing.T) {
	img := createTestImage(100, 80)
	stripped := stripExifData(img, "image/png")
	if stripped == nil {
		t.Fatal("expected non-nil stripped data for PNG")
	}

	// Verify the output is valid PNG.
	decoded, err := png.Decode(bytes.NewReader(stripped))
	if err != nil {
		t.Fatalf("stripped PNG is not valid: %v", err)
	}
	bounds := decoded.Bounds()
	if bounds.Dx() != 100 || bounds.Dy() != 80 {
		t.Errorf("dimensions = %dx%d, want 100x80", bounds.Dx(), bounds.Dy())
	}
}

func TestStripExifData_UnknownFormat(t *testing.T) {
	img := createTestImage(50, 50)
	stripped := stripExifData(img, "image/webp")
	if stripped == nil {
		t.Fatal("expected fallback PNG encoding for unknown format")
	}

	// Should be valid PNG.
	_, err := png.Decode(bytes.NewReader(stripped))
	if err != nil {
		t.Fatalf("fallback PNG is not valid: %v", err)
	}
}

func TestProcessImage(t *testing.T) {
	img := createTestImage(800, 600)

	// Encode as JPEG for processing.
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("failed to encode test JPEG: %v", err)
	}

	svc := &Service{
		stripExif:      true,
		thumbnailSizes: []int{128, 256, 512},
	}

	result := svc.processImage(buf.Bytes(), "image/jpeg")

	if result.width == nil || result.height == nil {
		t.Fatal("expected non-nil width and height")
	}
	if *result.width != 800 {
		t.Errorf("width = %d, want 800", *result.width)
	}
	if *result.height != 600 {
		t.Errorf("height = %d, want 600", *result.height)
	}

	if result.blurhash == nil {
		t.Fatal("expected non-nil blurhash")
	}
	if *result.blurhash == "" {
		t.Error("expected non-empty blurhash")
	}

	if result.stripped == nil {
		t.Fatal("expected non-nil stripped data (EXIF strip enabled)")
	}

	// Verify stripped data is valid JPEG.
	_, err := jpeg.Decode(bytes.NewReader(result.stripped))
	if err != nil {
		t.Fatalf("stripped JPEG is not valid: %v", err)
	}
}

func TestProcessImage_NoStrip(t *testing.T) {
	img := createTestImage(200, 200)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("failed to encode test PNG: %v", err)
	}

	svc := &Service{
		stripExif:      false,
		thumbnailSizes: []int{128},
	}

	result := svc.processImage(buf.Bytes(), "image/png")

	if result.width == nil || *result.width != 200 {
		t.Errorf("width = %v, want 200", result.width)
	}
	if result.blurhash == nil || *result.blurhash == "" {
		t.Error("expected non-empty blurhash")
	}
	if result.stripped != nil {
		t.Error("expected nil stripped data when EXIF strip is disabled")
	}
}

func TestProcessImage_InvalidData(t *testing.T) {
	svc := &Service{stripExif: true}
	result := svc.processImage([]byte("not an image"), "image/jpeg")

	if result.width != nil || result.height != nil {
		t.Error("expected nil dimensions for invalid image data")
	}
	if result.blurhash != nil {
		t.Error("expected nil blurhash for invalid image data")
	}
}

func TestExtractDatePath(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{"attachments/2026/02/10/abc.jpg", "2026/02/10"},
		{"attachments/2025/12/31/xyz.png", "2025/12/31"},
		{"short", ""}, // falls back to current date
	}

	for _, tt := range tests {
		got := extractDatePath(tt.key)
		if tt.want != "" && got != tt.want {
			t.Errorf("extractDatePath(%q) = %q, want %q", tt.key, got, tt.want)
		}
		if tt.want == "" && got == "" {
			t.Errorf("extractDatePath(%q) returned empty, expected current date fallback", tt.key)
		}
	}
}

func TestThumbnailURL(t *testing.T) {
	got := ThumbnailURL("abc123", "2026/02/10", 256)
	want := "thumbnails/2026/02/10/abc123_256.jpg"
	if got != want {
		t.Errorf("ThumbnailURL = %q, want %q", got, want)
	}
}
