package misc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LokiClient handles logging to JSON files that Alloy ships to Grafana Loki.
// This approach is resilient (survives Loki downtime) and non-blocking.
type LokiClient struct {
	enabled      bool
	jsonFilePath string
	fileMutex    sync.Mutex
	currentFile  *os.File
	currentDate  string
}

// lokiJsonLogEntry is the JSON structure written to log files for Alloy to pick up.
type lokiJsonLogEntry struct {
	Timestamp string            `json:"timestamp"`
	Labels    map[string]string `json:"labels"`
	Message   json.RawMessage   `json:"message"`
}

var lokiInstance *LokiClient
var lokiOnce sync.Once

// GetLoki returns the singleton Loki client instance.
func GetLoki() *LokiClient {
	lokiOnce.Do(func() {
		enabled := os.Getenv("LOKI_ENABLED") == "true" || os.Getenv("LOKI_ENABLED") == "1"
		jsonFilePath := os.Getenv("LOKI_JSON_PATH")

		if enabled && jsonFilePath == "" {
			fmt.Println("Loki enabled but LOKI_JSON_PATH not set, disabling Loki")
			enabled = false
		}

		lokiInstance = &LokiClient{
			enabled:      enabled,
			jsonFilePath: jsonFilePath,
		}

		if enabled {
			if err := os.MkdirAll(jsonFilePath, 0755); err != nil {
				fmt.Printf("Failed to create Loki log directory %s: %v\n", jsonFilePath, err)
			} else {
				fmt.Printf("Loki JSON file logging enabled, writing to %s\n", jsonFilePath)
			}
		}
	})
	return lokiInstance
}

// IsEnabled returns whether Loki logging is enabled.
func (l *LokiClient) IsEnabled() bool {
	return l.enabled
}

// maxStringLength is the maximum length for logged string values.
const maxStringLength = 32

// truncateString truncates a string to maxStringLength characters.
func truncateString(s string) string {
	if len(s) <= maxStringLength {
		return s
	}
	return s[:maxStringLength] + "..."
}

// truncateMap recursively truncates all string values in a map.
func truncateMap(data map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range data {
		result[k] = truncateValue(v)
	}
	return result
}

// truncateValue truncates a value if it's a string, or recursively processes maps/slices.
func truncateValue(v interface{}) interface{} {
	switch val := v.(type) {
	case string:
		return truncateString(val)
	case map[string]interface{}:
		return truncateMap(val)
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, item := range val {
			result[i] = truncateValue(item)
		}
		return result
	default:
		return v
	}
}

// LogApiRequest logs an API request to Loki.
func (l *LokiClient) LogApiRequest(version, method, endpoint string, statusCode int, durationMs float64, userId *uint64, extra map[string]string) {
	if !l.enabled {
		return
	}

	// Determine log level: only 5xx errors are "error", everything else is "info".
	// 401/403 are normal for unauthenticated requests.
	level := "info"
	if statusCode >= 500 {
		level = "error"
	}

	labels := map[string]string{
		"app":         "freegle",
		"source":      "api",
		"api_version": version,
		"method":      method,
		"status_code": strconv.Itoa(statusCode),
		"level":       level,
	}

	// Add user_id as label for indexed queries.
	if userId != nil && *userId != 0 {
		labels["user_id"] = strconv.FormatUint(*userId, 10)
	}

	logData := map[string]interface{}{
		"endpoint":    endpoint,
		"duration_ms": durationMs,
		"user_id":     userId,
		"timestamp":   time.Now().Format(time.RFC3339),
	}

	for k, v := range extra {
		logData[k] = v
	}

	logLine, _ := json.Marshal(logData)
	l.log(labels, string(logLine))
}

// LogApiRequestFull logs an API request with full request/response data.
func (l *LokiClient) LogApiRequestFull(version, method, endpoint string, statusCode int, durationMs float64, userId *uint64, extra map[string]string, queryParams map[string]string, requestBody, responseBody map[string]interface{}) {
	if !l.enabled {
		return
	}

	// Determine log level: only 5xx errors are "error", everything else is "info".
	// 401/403 are normal for unauthenticated requests.
	level := "info"
	if statusCode >= 500 {
		level = "error"
	}

	labels := map[string]string{
		"app":         "freegle",
		"source":      "api",
		"api_version": version,
		"method":      method,
		"status_code": strconv.Itoa(statusCode),
		"level":       level,
	}

	// Add user_id as label for indexed queries (low-ish cardinality).
	// Note: trace_id and session_id stay in JSON body only (high cardinality).
	if userId != nil && *userId != 0 {
		labels["user_id"] = strconv.FormatUint(*userId, 10)
	}

	logData := map[string]interface{}{
		"endpoint":    endpoint,
		"duration_ms": durationMs,
		"user_id":     userId,
		"timestamp":   time.Now().Format(time.RFC3339),
	}

	for k, v := range extra {
		logData[k] = v
	}

	// Add query parameters (truncated).
	if len(queryParams) > 0 {
		truncatedParams := make(map[string]string)
		for k, v := range queryParams {
			truncatedParams[k] = truncateString(v)
		}
		logData["query_params"] = truncatedParams
	}

	// Add request body (truncated).
	if len(requestBody) > 0 {
		logData["request_body"] = truncateMap(requestBody)
	}

	// Add response body (truncated).
	if len(responseBody) > 0 {
		logData["response_body"] = truncateMap(responseBody)
	}

	logLine, _ := json.Marshal(logData)
	l.log(labels, string(logLine))
}

// Sensitive header patterns to exclude from logging.
var sensitiveHeaderPatterns = []string{
	"authorization",
	"cookie",
	"set-cookie",
	"x-api-key",
}

// Allowed request headers (allowlist approach).
var allowedRequestHeaders = map[string]bool{
	"user-agent":        true,
	"referer":           true,
	"content-type":      true,
	"accept":            true,
	"accept-language":   true,
	"accept-encoding":   true,
	"x-forwarded-for":   true,
	"x-forwarded-proto": true,
	"x-request-id":      true,
	"x-real-ip":         true,
	"origin":            true,
	"host":              true,
	"content-length":    true,
	// Logging context headers.
	"x-freegle-session": true,
	"x-freegle-page":    true,
	"x-freegle-modal":   true,
	"x-freegle-site":    true,
}

// LogApiHeaders logs API headers to Loki (separate stream with 7-day retention).
func (l *LokiClient) LogApiHeaders(version, method, endpoint string, requestHeaders, responseHeaders map[string]string, userId *uint64, requestId string) {
	if !l.enabled {
		return
	}

	labels := map[string]string{
		"app":         "freegle",
		"source":      "api_headers",
		"api_version": version,
		"method":      method,
	}

	logData := map[string]interface{}{
		"endpoint":         endpoint,
		"user_id":          userId,
		"request_id":       requestId,
		"request_headers":  filterHeaders(requestHeaders, true),
		"response_headers": filterHeaders(responseHeaders, false),
		"timestamp":        time.Now().Format(time.RFC3339),
	}

	logLine, _ := json.Marshal(logData)
	l.log(labels, string(logLine))
}

// filterHeaders removes sensitive headers and applies allowlist for request headers.
func filterHeaders(headers map[string]string, useAllowlist bool) map[string]string {
	filtered := make(map[string]string)

	for name, value := range headers {
		nameLower := strings.ToLower(name)

		// Check against sensitive patterns.
		isSensitive := false
		for _, pattern := range sensitiveHeaderPatterns {
			if strings.Contains(nameLower, pattern) {
				isSensitive = true
				break
			}
		}

		if isSensitive {
			continue
		}

		// For request headers, use allowlist.
		if useAllowlist {
			if allowedRequestHeaders[nameLower] {
				filtered[name] = value
			}
		} else {
			// For response headers, include all non-sensitive.
			filtered[name] = value
		}
	}

	return filtered
}

// LogFromLogsTable logs entries that mirror the logs table to Loki.
func (l *LokiClient) LogFromLogsTable(logType, subtype string, groupId, userId, byUser, msgId *uint64, text string) {
	if !l.enabled {
		return
	}

	labels := map[string]string{
		"app":     "freegle",
		"source":  "logs_table",
		"type":    logType,
		"subtype": subtype,
	}

	if groupId != nil {
		labels["groupid"] = strconv.FormatUint(*groupId, 10)
	}
	if userId != nil && *userId != 0 {
		labels["user_id"] = strconv.FormatUint(*userId, 10)
	}

	logData := map[string]interface{}{
		"user_id":   userId,
		"by_user":   byUser,
		"msg_id":    msgId,
		"group_id":  groupId,
		"text":      text,
		"timestamp": time.Now().Format(time.RFC3339),
	}

	logLine, _ := json.Marshal(logData)
	l.log(labels, string(logLine))
}

// LogClientEntry logs entries from the client-side browser to Loki.
func (l *LokiClient) LogClientEntry(level, eventType string, logData map[string]interface{}) {
	if !l.enabled {
		return
	}

	labels := map[string]string{
		"app":        "freegle",
		"source":     "client",
		"level":      level,
		"event_type": eventType,
	}

	// Add user_id as label for indexed queries (low-ish cardinality).
	// Note: trace_id and session_id stay in JSON body only (high cardinality).
	if userID, ok := logData["user_id"].(float64); ok && userID != 0 {
		labels["user_id"] = strconv.FormatInt(int64(userID), 10)
	}

	logLine, _ := json.Marshal(logData)
	l.log(labels, string(logLine))
}

// log writes a log entry to a JSON file for Alloy to ship.
func (l *LokiClient) log(labels map[string]string, logLine string) {
	if !l.enabled {
		return
	}

	now := time.Now()
	timestamp := now.Format(time.RFC3339Nano)
	dateStr := now.Format("2006-01-02")

	entry := lokiJsonLogEntry{
		Timestamp: timestamp,
		Labels:    labels,
		Message:   json.RawMessage(logLine),
	}

	jsonBytes, err := json.Marshal(entry)
	if err != nil {
		fmt.Printf("Loki JSON marshal error: %v\n", err)
		return
	}

	// Add newline for JSON lines format.
	jsonBytes = append(jsonBytes, '\n')

	l.fileMutex.Lock()
	defer l.fileMutex.Unlock()

	// Rotate file daily.
	if l.currentFile == nil || l.currentDate != dateStr {
		if l.currentFile != nil {
			l.currentFile.Close()
		}

		filename := filepath.Join(l.jsonFilePath, fmt.Sprintf("go-api-%s.log", dateStr))
		file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Printf("Loki file open error: %v\n", err)
			return
		}

		l.currentFile = file
		l.currentDate = dateStr
	}

	if _, err := l.currentFile.Write(jsonBytes); err != nil {
		fmt.Printf("Loki file write error: %v\n", err)
	}
}

// LogChatReply logs a chat reply event with source tracking for dashboard analytics.
// Sources: "amp" (AMP email form), "email" (email reply), "website" (web interface)
func (l *LokiClient) LogChatReply(source string, chatID, userID uint64, messageID *uint64, emailTrackingID *uint64) {
	if !l.enabled {
		return
	}

	labels := map[string]string{
		"app":          "freegle",
		"source":       "chat_reply",
		"reply_source": source,
		"user_id":      strconv.FormatUint(userID, 10),
	}

	logData := map[string]interface{}{
		"reply_source":      source,
		"chat_id":           chatID,
		"user_id":           userID,
		"message_id":        messageID,
		"email_tracking_id": emailTrackingID,
		"timestamp":         time.Now().Format(time.RFC3339),
	}

	logLine, _ := json.Marshal(logData)
	l.log(labels, string(logLine))
}

// Close gracefully shuts down the Loki client.
func (l *LokiClient) Close() {
	if l.enabled {
		l.fileMutex.Lock()
		if l.currentFile != nil {
			l.currentFile.Close()
			l.currentFile = nil
		}
		l.fileMutex.Unlock()
	}
}
