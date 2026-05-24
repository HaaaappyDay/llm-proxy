package server

import (
	"github.com/HaaapyDay/llm-proxy/internal/app"
	"github.com/HaaapyDay/llm-proxy/internal/proxy"
	"github.com/gin-gonic/gin"
)

func NewRouter(application *app.App) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	h := proxy.NewHandlers(application)

	r.GET("/health", h.Health)

	llm := r.Group("")
	llm.Use(proxy.APIKeyMiddleware(application))
	{
		llm.GET("/v1/models", h.GetModels)
		llm.POST("/v1/messages", h.PostMessages)
		llm.POST("/v1/chat/completions", h.PostChatCompletions)
		llm.POST("/v1/responses", h.PostResponses)
	}

	return r
}
