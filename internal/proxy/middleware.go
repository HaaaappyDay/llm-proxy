package proxy

import (
	"net/http"
	"strings"

	"github.com/HaaapyDay/llm-proxy/internal/app"
	"github.com/HaaapyDay/llm-proxy/internal/auth"
	"github.com/gin-gonic/gin"
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
			c.AbortWithStatusJSON(http.StatusUnauthorized, newStatusErrorEnvelope(http.StatusUnauthorized, "invalid_api_key", "invalid or missing api key"))
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
