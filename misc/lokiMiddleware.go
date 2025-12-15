package misc

import (
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
	"time"
)

// LokiMiddlewareConfig configures the Loki logging middleware.
type LokiMiddlewareConfig struct {
	Skip func(c *fiber.Ctx) bool
}

// NewLokiMiddleware creates a Fiber middleware that logs requests to Loki.
// Logging is done asynchronously to avoid impacting request latency.
func NewLokiMiddleware(config LokiMiddlewareConfig) fiber.Handler {
	loki := GetLoki()

	return func(c *fiber.Ctx) error {
		// Skip if Loki is not enabled
		if !loki.IsEnabled() {
			return c.Next()
		}

		// Skip if configured to skip this request
		if config.Skip != nil && config.Skip(c) {
			return c.Next()
		}

		start := time.Now()

		// Capture request headers before processing (context may be reused).
		requestHeaders := make(map[string]string)
		c.Request().Header.VisitAll(func(key, value []byte) {
			requestHeaders[string(key)] = string(value)
		})

		// Capture request info before c.Next()
		method := c.Method()
		path := c.Path()
		ip := c.IP()

		// Get user ID from context if available
		var userId *uint64
		userIdInJWT, _, _ := user.GetJWTFromRequest(c)
		if userIdInJWT > 0 {
			userId = &userIdInJWT
		}

		// Process request
		err := c.Next()

		// Capture response headers and status after processing.
		statusCode := c.Response().StatusCode()
		responseHeaders := make(map[string]string)
		c.Response().Header.VisitAll(func(key, value []byte) {
			responseHeaders[string(key)] = string(value)
		})

		// Log asynchronously using goroutine to avoid blocking response.
		go func() {
			duration := float64(time.Since(start).Milliseconds())

			extra := map[string]string{
				"ip": ip,
			}

			// Include trace headers for distributed tracing correlation.
			if traceId, ok := requestHeaders["X-Trace-Id"]; ok && traceId != "" {
				extra["trace_id"] = traceId
			}
			if sessionId, ok := requestHeaders["X-Session-Id"]; ok && sessionId != "" {
				extra["session_id"] = sessionId
			}
			if clientTimestamp, ok := requestHeaders["X-Client-Timestamp"]; ok && clientTimestamp != "" {
				extra["client_timestamp"] = clientTimestamp
			}

			loki.LogApiRequest("v2", method, path, statusCode, duration, userId, extra)

			// Log headers separately (7-day retention for debugging).
			loki.LogApiHeaders("v2", method, path, requestHeaders, responseHeaders, userId)
		}()

		return err
	}
}
