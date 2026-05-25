package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
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
		Header: http.Header{
			"Content-Type": []string{"text/plain"},
			"Retry-After":  []string{"12"},
		},
		Body: io.NopCloser(strings.NewReader(`{"error":"rate limited","access_token":"secret-token"}`)),
	}
	rec := httptest.NewRecorder()

	copyUpstream(rec, resp)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "12" {
		t.Fatalf("Retry-After = %q, want 12", got)
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
	if errObj["retry_after"] != "12" {
		t.Fatalf("retry_after = %v, want 12", errObj["retry_after"])
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

func TestUpstreamErrorResponseOmitsEmptyRetryAfter(t *testing.T) {
	out := upstreamErrorResponse(&UpstreamStatusError{
		StatusCode: http.StatusInternalServerError,
		Preview:    "server error",
	})
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "retry_after") {
		t.Fatalf("unexpected retry_after field: %s", raw)
	}
}

func TestWriteProxyErrorForUpstreamSetsRetryAfterHeader(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	writeProxyError(c, &UpstreamStatusError{
		StatusCode: http.StatusTooManyRequests,
		RetryAfter: "Wed, 21 Oct 2015 07:28:00 GMT",
		Preview:    "rate limited",
	})

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "Wed, 21 Oct 2015 07:28:00 GMT" {
		t.Fatalf("Retry-After = %q", got)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not json: %v", err)
	}
	errObj := out["error"].(map[string]any)
	if errObj["retry_after"] != "Wed, 21 Oct 2015 07:28:00 GMT" {
		t.Fatalf("retry_after = %v", errObj["retry_after"])
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
