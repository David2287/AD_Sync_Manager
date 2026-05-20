package middleware

import (
	"time"

	"github.com/gin-gonic/gin"

	"ad-sync-manager/internal/auth"
)

// RequestLogger emits a structured log line for every HTTP request.
// It logs method, path, status, latency, client IP, and request ID.
func (b *Bundle) RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		fields := []any{
			"method", c.Request.Method,
			"path", path,
			"status", status,
			"latency_ms", latency.Milliseconds(),
			"ip", c.ClientIP(),
		}
		if query != "" {
			fields = append(fields, "query", query)
		}
		// Include the authenticated username when a valid JWT was presented.
		if v, exists := c.Get(ginClaimsKey); exists {
			if claims, ok := v.(*auth.Claims); ok && claims != nil {
				fields = append(fields, "user", claims.Username)
			}
		}
		if errMsg := c.Errors.ByType(gin.ErrorTypePrivate).String(); errMsg != "" {
			fields = append(fields, "errors", errMsg)
		}

		switch {
		case status >= 500:
			b.log.Error("request", fields...)
		case status >= 400:
			b.log.Warn("request", fields...)
		default:
			b.log.Info("request", fields...)
		}
	}
}
