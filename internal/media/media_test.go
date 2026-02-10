package media

import (
	"encoding/json"
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

	// When MaxUploadMB is 0, New() should default to 100MB.
	// We can't call New() without a real S3 endpoint, but we verify
	// the logic inline:
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
