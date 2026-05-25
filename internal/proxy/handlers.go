package proxy

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/HaaapyDay/llm-proxy/internal/app"
	"github.com/HaaapyDay/llm-proxy/internal/transform"
	"github.com/gin-gonic/gin"
)

const MaxRequestBodyBytes int64 = 32 << 20

type Handlers struct {
	app       *app.App
	forwarder *Forwarder
}

func NewHandlers(application *app.App) *Handlers {
	return &Handlers{
		app:       application,
		forwarder: NewForwarder(application),
	}
}

func (h *Handlers) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handlers) GetModels(c *gin.Context) {
	rec, ok := APIKeyFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, newStatusErrorEnvelope(http.StatusUnauthorized, "invalid_api_key", "invalid or missing api key"))
		return
	}
	if err := h.forwarder.HandleModels(c.Writer, rec); err != nil {
		if !c.Writer.Written() {
			writeProxyError(c, err)
		}
		return
	}
}

func (h *Handlers) PostMessages(c *gin.Context) {
	rec, ok := APIKeyFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, newStatusErrorEnvelope(http.StatusUnauthorized, "invalid_api_key", "invalid or missing api key"))
		return
	}
	raw, ok := readRequestBody(c)
	if !ok {
		return
	}
	if err := h.forwarder.HandleAnthropicMessages(c.Writer, rec, raw); err != nil {
		if !c.Writer.Written() {
			writeProxyError(c, err)
		}
		return
	}
}

func (h *Handlers) PostChatCompletions(c *gin.Context) {
	rec, ok := APIKeyFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, newStatusErrorEnvelope(http.StatusUnauthorized, "invalid_api_key", "invalid or missing api key"))
		return
	}
	raw, ok := readRequestBody(c)
	if !ok {
		return
	}
	if err := h.forwarder.HandleOpenAIChat(c.Writer, rec, raw); err != nil {
		if !c.Writer.Written() {
			writeProxyError(c, err)
		}
		return
	}
}

func (h *Handlers) PostResponses(c *gin.Context) {
	rec, ok := APIKeyFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, newStatusErrorEnvelope(http.StatusUnauthorized, "invalid_api_key", "invalid or missing api key"))
		return
	}
	raw, ok := readRequestBody(c)
	if !ok {
		return
	}
	if err := h.forwarder.HandleOpenAIResponses(c.Writer, rec, raw); err != nil {
		if !c.Writer.Written() {
			writeProxyError(c, err)
		}
		return
	}
}

func readRequestBody(c *gin.Context) ([]byte, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxRequestBodyBytes)
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "request body too large") {
			status = http.StatusRequestEntityTooLarge
		}
		errorType := "invalid_request"
		if status == http.StatusRequestEntityTooLarge {
			errorType = "request_too_large"
		}
		c.JSON(status, newStatusErrorEnvelope(status, errorType, err.Error()))
		return nil, false
	}
	return raw, true
}

func writeProxyError(c *gin.Context, err error) {
	if unsupported, ok := err.(*transform.UnsupportedFeatureError); ok {
		out := newErrorEnvelope("unsupported_feature", unsupported.Error())
		out.Error.SourceFormat = string(unsupported.Source)
		out.Error.TargetFormat = string(unsupported.Target)
		out.Error.UnsupportedFeature = unsupported.Feature
		c.JSON(http.StatusBadRequest, out)
		return
	}
	if upstream, ok := err.(*UpstreamStatusError); ok {
		if upstream.RetryAfter != "" {
			c.Header("Retry-After", upstream.RetryAfter)
		}
		c.JSON(upstream.StatusCode, upstreamErrorResponse(upstream))
		return
	}
	status := http.StatusBadGateway
	errorType := "proxy_error"
	var invalidReq *invalidRequestError
	if errors.As(err, &invalidReq) {
		status = http.StatusBadRequest
		errorType = "invalid_request"
	}
	c.JSON(status, newStatusErrorEnvelope(status, errorType, err.Error()))
}
