package proxy

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lotus/llm-proxy/internal/app"
	"github.com/lotus/llm-proxy/internal/auth"
)

const apiKeyContextKey = "apiKeyRecord"

func APIKeyMiddleware(application *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		authz := c.GetHeader("Authorization")
		if authz == "" {
			authz = c.GetHeader("x-api-key")
		}
		token := strings.TrimSpace(authz)
		if strings.HasPrefix(strings.ToLower(token), "bearer ") {
			token = strings.TrimSpace(token[7:])
		}
		rec, err := application.APIKeys.Resolve(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or missing api key"})
			return
		}
		c.Set(apiKeyContextKey, rec)
		c.Next()
	}
}

func APIKeyFromContext(c *gin.Context) (*auth.APIKeyRecord, bool) {
	v, ok := c.Get(apiKeyContextKey)
	if !ok {
		return nil, false
	}
	rec, ok := v.(*auth.APIKeyRecord)
	return rec, ok
}
