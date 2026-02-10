package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"key": "value"})

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q, want %q", ct, "application/json")
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data object in response")
	}
	if data["key"] != "value" {
		t.Errorf("data.key = %q, want %q", data["key"], "value")
	}
}

func TestWriteError(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		code    string
		message string
	}{
		{"forbidden", http.StatusForbidden, "forbidden", "Admin access required"},
		{"not found", http.StatusNotFound, "not_found", "Resource not found"},
		{"bad request", http.StatusBadRequest, "invalid_body", "Invalid request body"},
		{"internal error", http.StatusInternalServerError, "internal_error", "Something went wrong"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeError(w, tt.status, tt.code, tt.message)

			if w.Code != tt.status {
				t.Errorf("status = %d, want %d", w.Code, tt.status)
			}

			var body map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}

			errObj, ok := body["error"].(map[string]interface{})
			if !ok {
				t.Fatalf("expected error object in response")
			}
			if errObj["code"] != tt.code {
				t.Errorf("error.code = %q, want %q", errObj["code"], tt.code)
			}
			if errObj["message"] != tt.message {
				t.Errorf("error.message = %q, want %q", errObj["message"], tt.message)
			}
		})
	}
}

func TestUpdateInstanceRequest_Unmarshal(t *testing.T) {
	raw := `{"name": "My Instance", "description": "A cool server", "federation_mode": "open"}`
	var req updateInstanceRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if req.Name == nil || *req.Name != "My Instance" {
		t.Errorf("name = %v, want %q", req.Name, "My Instance")
	}
	if req.Description == nil || *req.Description != "A cool server" {
		t.Errorf("description = %v, want %q", req.Description, "A cool server")
	}
	if req.FederationMode == nil || *req.FederationMode != "open" {
		t.Errorf("federation_mode = %v, want %q", req.FederationMode, "open")
	}
}

func TestUpdateInstanceRequest_PartialFields(t *testing.T) {
	raw := `{"name": "Updated Name"}`
	var req updateInstanceRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if req.Name == nil || *req.Name != "Updated Name" {
		t.Errorf("name = %v, want %q", req.Name, "Updated Name")
	}
	if req.Description != nil {
		t.Errorf("description = %v, want nil", req.Description)
	}
	if req.FederationMode != nil {
		t.Errorf("federation_mode = %v, want nil", req.FederationMode)
	}
}

func TestAddPeerRequest_Unmarshal(t *testing.T) {
	raw := `{"domain": "example.com"}`
	var req addPeerRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if req.Domain != "example.com" {
		t.Errorf("domain = %q, want %q", req.Domain, "example.com")
	}
}

func TestAddPeerRequest_EmptyDomain(t *testing.T) {
	raw := `{}`
	var req addPeerRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if req.Domain != "" {
		t.Errorf("domain = %q, want empty", req.Domain)
	}
}

func TestWriteJSON_Array(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, []string{"a", "b", "c"})

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	data, ok := body["data"].([]interface{})
	if !ok {
		t.Fatalf("expected data array in response")
	}
	if len(data) != 3 {
		t.Errorf("len(data) = %d, want 3", len(data))
	}
}

func TestWriteJSON_StatusCodes(t *testing.T) {
	codes := []int{
		http.StatusOK,
		http.StatusCreated,
		http.StatusAccepted,
	}

	for _, code := range codes {
		w := httptest.NewRecorder()
		writeJSON(w, code, "test")
		if w.Code != code {
			t.Errorf("status = %d, want %d", w.Code, code)
		}
	}
}
