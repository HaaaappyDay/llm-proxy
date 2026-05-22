package proxy

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lotus/llm-proxy/internal/app"
	"github.com/lotus/llm-proxy/internal/auth"
)

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

func (h *Handlers) PostMessages(c *gin.Context) {
	rec, ok := APIKeyFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.forwarder.HandleAnthropicMessages(c.Writer, rec, raw); err != nil {
		if !c.Writer.Written() {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		}
		return
	}
}

func (h *Handlers) PostChatCompletions(c *gin.Context) {
	rec, ok := APIKeyFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.forwarder.HandleOpenAIChat(c.Writer, rec, raw); err != nil {
		if !c.Writer.Written() {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		}
		return
	}
}

func (h *Handlers) PostResponses(c *gin.Context) {
	rec, ok := APIKeyFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.forwarder.HandleOpenAIResponses(c.Writer, rec, raw); err != nil {
		if !c.Writer.Written() {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		}
		return
	}
}

// --- Auth API ---

func (h *Handlers) CodexDevice(c *gin.Context) {
	out, err := h.app.Codex.StartDeviceFlow()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handlers) CodexPoll(c *gin.Context) {
	var req struct {
		DeviceCode string `json:"device_code"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	acc, err := h.app.Codex.PollForToken(req.DeviceCode)
	if err == auth.ErrAuthPending {
		c.JSON(http.StatusAccepted, gin.H{"status": "pending"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"account": acc})
}

func (h *Handlers) CopilotDevice(c *gin.Context) {
	out, err := h.app.Copilot.StartDeviceFlow()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handlers) CopilotPoll(c *gin.Context) {
	var req struct {
		DeviceCode string `json:"device_code"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	acc, err := h.app.Copilot.PollForToken(req.DeviceCode)
	if err == auth.ErrAuthPending {
		c.JSON(http.StatusAccepted, gin.H{"status": "pending"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"account": acc})
}

func (h *Handlers) CreateKey(c *gin.Context) {
	var req struct {
		Label     string `json:"label"`
		Provider  string `json:"provider"`
		AccountID string `json:"account_id"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Label == "" {
		req.Label = "default"
	}
	result, err := h.app.APIKeys.Create(auth.CreateKeyInput{
		Label:     req.Label,
		Provider:  req.Provider,
		AccountID: req.AccountID,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"api_key": result.Plaintext,
		"record":  result.Record,
	})
}

func (h *Handlers) ListKeys(c *gin.Context) {
	c.JSON(http.StatusOK, h.app.APIKeys.List())
}

func (h *Handlers) DeleteKey(c *gin.Context) {
	if err := h.app.APIKeys.Delete(c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}
