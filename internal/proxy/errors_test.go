package proxy

import (
	"encoding/json"
	"testing"
)

func TestNewErrorEnvelope(t *testing.T) {
	out := newErrorEnvelope("invalid_request", "bad request")

	if out.Error.Type != "invalid_request" {
		t.Fatalf("type = %q", out.Error.Type)
	}
	if out.Error.Message != "bad request" {
		t.Fatalf("message = %q", out.Error.Message)
	}

	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"error":{"type":"invalid_request","message":"bad request"}}`
	if string(raw) != want {
		t.Fatalf("json = %s, want %s", raw, want)
	}
}

func TestNewStatusErrorEnvelope(t *testing.T) {
	out := newStatusErrorEnvelope(413, "request_too_large", "request body too large")

	if out.Error.Status != 413 {
		t.Fatalf("status = %d", out.Error.Status)
	}

	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"error":{"type":"request_too_large","message":"request body too large","status":413}}`
	if string(raw) != want {
		t.Fatalf("json = %s, want %s", raw, want)
	}
}
