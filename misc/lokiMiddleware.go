package misc

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"time"
)

// LokiMiddlewareConfig configures the Loki logging middleware.
type LokiMiddlewareConfig struct {
	Skip func(c *fiber.Ctx) bool
	// GetUserId extracts the user ID from the request context.
	// This is injected to avoid import cycles with the user package.
	GetUserId func(c *fiber.Ctx) *uint64
	// GetUserRole extracts the user's system role from the request context.
	// Returns nil for regular users, or role string for mods/support/admin.
	GetUserRole func(c *fiber.Ctx) *string
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

		// Capture query parameters.
		queryParams := make(map[string]string)
		c.Request().URI().QueryArgs().VisitAll(func(key, value []byte) {
			queryParams[string(key)] = string(value)
		})

		// Capture request body for POST/PUT/PATCH.
		var requestBody map[string]interface{}
		if method == "POST" || method == "PUT" || method == "PATCH" {
			bodyBytes := c.Body()
			if len(bodyBytes) > 0 {
				json.Unmarshal(bodyBytes, &requestBody)
			}
		}

		// Get user ID from context if available
		var userId *uint64
		if config.GetUserId != nil {
			userId = config.GetUserId(c)
		}

		// Generate unique request_id to correlate API logs with headers logs.
		// Format: timestamp_ms (hex) + random bytes for uniqueness within same ms.
		timestampMs := time.Now().UnixMilli()
		randomBytes := make([]byte, 4)
		rand.Read(randomBytes)
		requestId := fmt.Sprintf("%x%s", timestampMs, hex.EncodeToString(randomBytes))

		// Process request
		err := c.Next()

		// Add X-User-Id header for HAProxy per-user rate limiting.
		if userId != nil {
			c.Set("X-User-Id", fmt.Sprintf("%d", *userId))
		}

		// Add X-User-Role header for HAProxy to exempt mods/support/admin from rate limiting.
		if config.GetUserRole != nil {
			userRole := config.GetUserRole(c)
			if userRole != nil && *userRole != "" {
				c.Set("X-User-Role", *userRole)
			}
		}

		// Capture response headers and status after processing.
		statusCode := c.Response().StatusCode()
		responseHeaders := make(map[string]string)
		c.Response().Header.VisitAll(func(key, value []byte) {
			responseHeaders[string(key)] = string(value)
		})

		// Capture response body.
		var responseBody map[string]interface{}
		respBodyBytes := c.Response().Body()
		if len(respBodyBytes) > 0 {
			json.Unmarshal(respBodyBytes, &responseBody)
		}

		// Log asynchronously using goroutine to avoid blocking response.
		go func() {
			duration := float64(time.Since(start).Milliseconds())

			extra := map[string]string{
				"ip":         ip,
				"request_id": requestId,
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

			// Include Freegle logging context headers.
			if freegleSession, ok := requestHeaders["X-Freegle-Session"]; ok && freegleSession != "" {
				extra["freegle_session"] = freegleSession
			}
			if freeglePage, ok := requestHeaders["X-Freegle-Page"]; ok && freeglePage != "" {
				extra["freegle_page"] = freeglePage
			}
			if freegleModal, ok := requestHeaders["X-Freegle-Modal"]; ok && freegleModal != "" {
				extra["freegle_modal"] = freegleModal
			}
			if freegleSite, ok := requestHeaders["X-Freegle-Site"]; ok && freegleSite != "" {
				extra["freegle_site"] = freegleSite
			}

			// Log with full request/response data.
			loki.LogApiRequestFull("v2", method, path, statusCode, duration, userId, extra, queryParams, requestBody, responseBody)

			// Log headers separately (7-day retention for debugging).
			loki.LogApiHeaders("v2", method, path, requestHeaders, responseHeaders, userId, requestId)
		}()

		return err
	}
}
