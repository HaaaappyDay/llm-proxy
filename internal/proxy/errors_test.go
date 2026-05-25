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
