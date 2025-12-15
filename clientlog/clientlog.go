// Package clientlog handles client-side log entries sent from the browser.
package clientlog

import (
	"encoding/json"
	"github.com/freegle/iznik-server-go/misc"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

// LogEntry represents a single client log entry.
type LogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	TraceID   string                 `json:"trace_id"`
	SessionID string                 `json:"session_id"`
	URL       string                 `json:"url"`
	UserAgent string                 `json:"user_agent"`
	EventType string                 `json:"event_type,omitempty"`
	PageName  string                 `json:"page_name,omitempty"`
	Action    string                 `json:"action_name,omitempty"`
	Method    string                 `json:"method,omitempty"`
	Path      string                 `json:"path,omitempty"`
	Duration  float64                `json:"duration_ms,omitempty"`
	Status    int                    `json:"status,omitempty"`
	Extra     map[string]interface{} `json:"-"`
}

// ClientLogRequest represents the request body for client logs.
type ClientLogRequest struct {
	Logs []json.RawMessage `json:"logs"`
}

// ReceiveClientLogs handles POST /clientlog endpoint.
// This endpoint accepts client-side log entries and forwards them to Loki.
// Returns 204 No Content on success to minimize response size.
// Errors are silently ignored to avoid showing errors to users.
//
// @Summary Receive client logs
// @Description Accepts client-side log entries for distributed tracing
// @Tags logging
// @Accept json
// @Produce json
// @Param logs body ClientLogRequest true "Client log entries"
// @Success 204 "No Content"
func ReceiveClientLogs(c *fiber.Ctx) error {
	loki := misc.GetLoki()

	// Always return 204 regardless of errors - fire and forget.
	defer func() {
		// Recover from any panics silently.
		recover()
	}()

	// Parse request body.
	var req ClientLogRequest
	if err := c.BodyParser(&req); err != nil {
		// Silent failure - return 204 anyway.
		return c.SendStatus(fiber.StatusNoContent)
	}

	// Get user ID from context if available.
	var userId *uint64
	userIdInJWT, _, _ := user.GetJWTFromRequest(c)
	if userIdInJWT > 0 {
		userId = &userIdInJWT
	}

	// Extract trace headers from request.
	traceID := c.Get("X-Trace-ID")
	sessionID := c.Get("X-Session-ID")

	// Process each log entry asynchronously.
	go func() {
		for _, rawLog := range req.Logs {
			// Parse into a generic map to preserve all fields.
			var logData map[string]interface{}
			if err := json.Unmarshal(rawLog, &logData); err != nil {
				continue
			}

			// Override trace/session from headers if present.
			if traceID != "" {
				logData["trace_id"] = traceID
			}
			if sessionID != "" {
				logData["session_id"] = sessionID
			}

			// Add user ID if available.
			if userId != nil {
				logData["user_id"] = *userId
			}

			// Determine level for labels.
			level := "info"
			if l, ok := logData["level"].(string); ok && l != "" {
				level = l
			}

			// Determine event type for labels.
			eventType := "log"
			if et, ok := logData["event_type"].(string); ok && et != "" {
				eventType = et
			}

			// Log to Loki with client source.
			loki.LogClientEntry(level, eventType, logData)
		}
	}()

	return c.SendStatus(fiber.StatusNoContent)
}
