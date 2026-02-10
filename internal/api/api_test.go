package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"name": "test"}

	WriteJSON(w, http.StatusOK, data)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var envelope SuccessResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	m, ok := envelope.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data is %T, want map", envelope.Data)
	}
	if m["name"] != "test" {
		t.Errorf("data.name = %v, want %q", m["name"], "test")
	}
}

func TestWriteJSON_Created(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusCreated, "created")

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, http.StatusBadRequest, "bad_input", "Invalid input")

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if errResp.Error.Code != "bad_input" {
		t.Errorf("error.code = %q, want %q", errResp.Error.Code, "bad_input")
	}
	if errResp.Error.Message != "Invalid input" {
		t.Errorf("error.message = %q, want %q", errResp.Error.Message, "Invalid input")
	}
}

func TestWriteNoContent(t *testing.T) {
	w := httptest.NewRecorder()
	WriteNoContent(w)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
	if w.Body.Len() != 0 {
		t.Errorf("body should be empty, got %d bytes", w.Body.Len())
	}
}

func TestWriteJSONRaw(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"raw": "data"}
	WriteJSONRaw(w, http.StatusOK, data)

	var result map[string]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if result["raw"] != "data" {
		t.Errorf("raw = %q, want %q", result["raw"], "data")
	}
	// Should NOT be wrapped in {"data": ...} envelope.
	if _, ok := result["data"]; ok {
		t.Error("WriteJSONRaw should not wrap in envelope")
	}
}

func TestStubHandler(t *testing.T) {
	handler := stubHandler("test_endpoint")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotImplemented)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if errResp.Error.Code != "not_implemented" {
		t.Errorf("error.code = %q, want %q", errResp.Error.Code, "not_implemented")
	}
}

func TestCorsMiddleware(t *testing.T) {
	handler := corsMiddleware([]string{"https://example.com"})(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	// Allowed origin.
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if acao := w.Header().Get("Access-Control-Allow-Origin"); acao != "https://example.com" {
		t.Errorf("ACAO = %q, want %q", acao, "https://example.com")
	}

	// Disallowed origin.
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.Header.Set("Origin", "https://evil.com")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	if acao := w2.Header().Get("Access-Control-Allow-Origin"); acao != "" {
		t.Errorf("ACAO should be empty for disallowed origin, got %q", acao)
	}

	// Wildcard.
	handler2 := corsMiddleware([]string{"*"})(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)
	req3 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req3.Header.Set("Origin", "https://anything.com")
	w3 := httptest.NewRecorder()
	handler2.ServeHTTP(w3, req3)

	if acao := w3.Header().Get("Access-Control-Allow-Origin"); acao != "https://anything.com" {
		t.Errorf("wildcard ACAO = %q, want %q", acao, "https://anything.com")
	}
}

func TestCorsMiddleware_Preflight(t *testing.T) {
	handler := corsMiddleware([]string{"*"})(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("OPTIONS status = %d, want %d", w.Code, http.StatusNoContent)
	}
}

func TestCorsMiddleware_NoOrigin(t *testing.T) {
	called := false
	handler := corsMiddleware([]string{"*"})(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Error("handler should be called when no origin header")
	}
	if acao := w.Header().Get("Access-Control-Allow-Origin"); acao != "" {
		t.Errorf("ACAO should be empty when no origin, got %q", acao)
	}
}

func TestParsePagination(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantLimit  int64
		wantOffset int64
	}{
		{"defaults", "", 20, 0},
		{"custom limit", "limit=50", 50, 0},
		{"custom offset", "offset=10", 20, 10},
		{"both", "limit=30&offset=5", 30, 5},
		{"limit too high", "limit=200", 20, 0},
		{"limit zero", "limit=0", 20, 0},
		{"negative limit", "limit=-5", 20, 0},
		{"negative offset", "offset=-1", 20, 0},
		{"invalid limit", "limit=abc", 20, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test?"+tc.query, nil)
			gotLimit, gotOffset := parsePagination(req)
			if gotLimit != tc.wantLimit {
				t.Errorf("limit = %d, want %d", gotLimit, tc.wantLimit)
			}
			if gotOffset != tc.wantOffset {
				t.Errorf("offset = %d, want %d", gotOffset, tc.wantOffset)
			}
		})
	}
}
