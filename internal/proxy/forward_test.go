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

func TestCopyUpstreamSanitizesErrorBody(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Content-Type": []string{"text/plain"}},
		Body:       io.NopCloser(strings.NewReader("secret upstream account detail")),
	}
	rec := httptest.NewRecorder()

	copyUpstream(rec, resp)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "secret upstream account detail") {
		t.Fatalf("leaked upstream body: %s", rec.Body.String())
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
