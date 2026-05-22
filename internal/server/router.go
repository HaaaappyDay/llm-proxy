package server

import (
	"github.com/gin-gonic/gin"
	"github.com/lotus/llm-proxy/internal/app"
	"github.com/lotus/llm-proxy/internal/proxy"
)

func NewRouter(application *app.App) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	h := proxy.NewHandlers(application)

	r.GET("/health", h.Health)

	authAPI := r.Group("/api/v1")
	{
		authAPI.POST("/auth/codex/device", h.CodexDevice)
		authAPI.POST("/auth/codex/poll", h.CodexPoll)
		authAPI.POST("/auth/copilot/device", h.CopilotDevice)
		authAPI.POST("/auth/copilot/poll", h.CopilotPoll)
		authAPI.POST("/keys", h.CreateKey)
		authAPI.GET("/keys", h.ListKeys)
		authAPI.DELETE("/keys/:id", h.DeleteKey)
	}

	llm := r.Group("")
	llm.Use(proxy.APIKeyMiddleware(application))
	{
		llm.POST("/v1/messages", h.PostMessages)
		llm.POST("/v1/chat/completions", h.PostChatCompletions)
		llm.POST("/v1/responses", h.PostResponses)
	}

	return r
}
