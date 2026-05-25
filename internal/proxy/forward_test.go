package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckUpstreamStatusAllowsSuccess(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("ok")),
	}
	if err := checkUpstreamStatus(resp); err != nil {
		t.Fatalf("checkUpstreamStatus: %v", err)
	}
}

func TestCheckUpstreamStatusTruncatesErrorBody(t *testing.T) {
	body := strings.Repeat("x", int(maxUpstreamErrorBodyPreviewBytes)+100)
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	err := checkUpstreamStatus(resp)
	if err == nil {
		t.Fatal("expected upstream error")
	}
	upstream, ok := err.(*UpstreamStatusError)
	if !ok {
		t.Fatalf("err = %#v, want *UpstreamStatusError", err)
	}
	if upstream.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d", upstream.StatusCode)
	}
	if !upstream.Truncated {
		t.Fatal("expected truncation marker")
	}
	if len(upstream.Preview) > int(maxUpstreamErrorBodyPreviewBytes) {
		t.Fatalf("preview was not bounded: len=%d", len(upstream.Preview))
	}
}

func TestCopyUpstreamIncludesSanitizedErrorPreview(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Content-Type": []string{"text/plain"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":"rate limited","access_token":"secret-token"}`)),
	}
	rec := httptest.NewRecorder()

	copyUpstream(rec, resp)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d", rec.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not json: %v", err)
	}
	errObj, ok := out["error"].(map[string]any)
	if !ok {
		t.Fatalf("missing error object: %#v", out)
	}
	if errObj["type"] != "upstream_error" {
		t.Fatalf("error type = %v", errObj["type"])
	}
	preview, ok := errObj["body_preview"].(string)
	if !ok {
		t.Fatalf("missing body_preview: %#v", errObj)
	}
	if !strings.Contains(preview, "rate limited") {
		t.Fatalf("body_preview = %q", preview)
	}
	if strings.Contains(preview, "secret-token") {
		t.Fatalf("leaked sensitive preview: %s", rec.Body.String())
	}
}

func TestSanitizeUpstreamErrorPreviewRedactsTokenPatterns(t *testing.T) {
	raw := `{"authorization":"Bearer abc.def","api_key":"lpk_123456789","token":"plain-token","message":"bad"} ghp_123456789012345678901234`
	got := sanitizeUpstreamErrorPreview(raw)

	for _, leaked := range []string{"abc.def", "lpk_123456789", "plain-token", "ghp_123456789012345678901234"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("sanitizeUpstreamErrorPreview leaked %q in %q", leaked, got)
		}
	}
	if !strings.Contains(got, `"message":"bad"`) {
		t.Fatalf("sanitizeUpstreamErrorPreview removed non-sensitive content: %q", got)
	}
}

func TestOpenAIModelListShape(t *testing.T) {
	out := openAIModelList([]string{"model-a", "model-b"})
	if out["object"] != "list" {
		t.Fatalf("object = %v", out["object"])
	}
	data, ok := out["data"].([]map[string]any)
	if !ok {
		t.Fatalf("data = %#v", out["data"])
	}
	if len(data) != 2 {
		t.Fatalf("len(data) = %d", len(data))
	}
	if data[0]["id"] != "model-a" || data[0]["object"] != "model" {
		t.Fatalf("first model = %#v", data[0])
	}
}

func TestSafeLogValue(t *testing.T) {
	if got := safeLogValue(""); got != "-" {
		t.Fatalf("safeLogValue(empty) = %q", got)
	}
	if got := safeLogValue("hello\nworld"); got != "hello world" {
		t.Fatalf("safeLogValue(newline) = %q", got)
	}
	if got := safeLogValue(strings.Repeat("x", 140)); !strings.Contains(got, "truncated") {
		t.Fatalf("safeLogValue(long) = %q", got)
	}
}
