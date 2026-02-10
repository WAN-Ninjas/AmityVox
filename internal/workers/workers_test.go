package workers

import (
	"encoding/json"
	"testing"

	"github.com/amityvox/amityvox/internal/events"
)

func TestEventData_ValidJSON(t *testing.T) {
	raw, _ := json.Marshal(map[string]interface{}{
		"id":         "msg_001",
		"channel_id": "ch_001",
		"content":    "hello world",
	})

	event := events.Event{
		Type: "MESSAGE_CREATE",
		Data: raw,
	}

	data := eventData(event)
	if data == nil {
		t.Fatal("eventData returned nil for valid JSON")
	}

	if data["id"] != "msg_001" {
		t.Errorf("id = %v, want %q", data["id"], "msg_001")
	}
	if data["channel_id"] != "ch_001" {
		t.Errorf("channel_id = %v, want %q", data["channel_id"], "ch_001")
	}
	if data["content"] != "hello world" {
		t.Errorf("content = %v, want %q", data["content"], "hello world")
	}
}

func TestEventData_InvalidJSON(t *testing.T) {
	event := events.Event{
		Type: "MESSAGE_CREATE",
		Data: json.RawMessage(`not valid json`),
	}

	data := eventData(event)
	if data != nil {
		t.Errorf("expected nil for invalid JSON, got %v", data)
	}
}

func TestEventData_EmptyData(t *testing.T) {
	event := events.Event{
		Type: "MESSAGE_CREATE",
		Data: json.RawMessage(`{}`),
	}

	data := eventData(event)
	if data == nil {
		t.Fatal("eventData returned nil for empty object")
	}
	if len(data) != 0 {
		t.Errorf("expected empty map, got %v", data)
	}
}

func TestEventData_NilData(t *testing.T) {
	event := events.Event{
		Type: "MESSAGE_CREATE",
		Data: nil,
	}

	data := eventData(event)
	if data != nil {
		t.Errorf("expected nil for nil data, got %v", data)
	}
}

func TestNew(t *testing.T) {
	cfg := Config{
		Pool:   nil,
		Bus:    nil,
		Search: nil,
		Logger: nil,
	}

	m := New(cfg)
	if m == nil {
		t.Fatal("New returned nil")
	}

	if m.pool != nil {
		t.Error("pool should be nil")
	}
	if m.bus != nil {
		t.Error("bus should be nil")
	}
	if m.search != nil {
		t.Error("search should be nil")
	}
}
