package misc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LokiClient handles asynchronous logging to Grafana Loki.
// Uses goroutines to avoid blocking API responses.
type LokiClient struct {
	enabled     bool
	url         string
	client      *http.Client
	batch       []lokiStream
	batchMutex  sync.Mutex
	batchSize   int
	lastFlush   time.Time
	flushChan   chan struct{}
	closeChan   chan struct{}
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

type lokiPushPayload struct {
	Streams []lokiStream `json:"streams"`
}

var lokiInstance *LokiClient
var lokiOnce sync.Once

// GetLoki returns the singleton Loki client instance.
func GetLoki() *LokiClient {
	lokiOnce.Do(func() {
		enabled := os.Getenv("LOKI_ENABLED") == "true" || os.Getenv("LOKI_ENABLED") == "1"
		url := os.Getenv("LOKI_URL")

		if enabled && url == "" {
			enabled = false
		}

		lokiInstance = &LokiClient{
			enabled:   enabled,
			url:       url,
			client:    &http.Client{Timeout: 2 * time.Second},
			batch:     make([]lokiStream, 0),
			batchSize: 10,
			lastFlush: time.Now(),
			flushChan: make(chan struct{}, 1),
			closeChan: make(chan struct{}),
		}

		if enabled {
			// Start background flusher
			go lokiInstance.backgroundFlusher()
		}
	})
	return lokiInstance
}

// IsEnabled returns whether Loki logging is enabled.
func (l *LokiClient) IsEnabled() bool {
	return l.enabled
}

// LogApiRequest logs an API request to Loki asynchronously.
func (l *LokiClient) LogApiRequest(version, method, endpoint string, statusCode int, durationMs float64, userId *uint64, extra map[string]string) {
	if !l.enabled {
		return
	}

	labels := map[string]string{
		"app":         "freegle",
		"source":      "api",
		"api_version": version,
		"method":      method,
		"status_code": strconv.Itoa(statusCode),
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

// Sensitive header patterns to exclude from logging.
var sensitiveHeaderPatterns = []string{
	"authorization",
	"cookie",
	"set-cookie",
	"x-api-key",
}

// Allowed request headers (allowlist approach).
var allowedRequestHeaders = map[string]bool{
	"user-agent":       true,
	"referer":          true,
	"content-type":     true,
	"accept":           true,
	"accept-language":  true,
	"accept-encoding":  true,
	"x-forwarded-for":  true,
	"x-forwarded-proto": true,
	"x-request-id":     true,
	"x-real-ip":        true,
	"origin":           true,
	"host":             true,
	"content-length":   true,
}

// LogApiHeaders logs API headers to Loki (separate stream with 7-day retention).
func (l *LokiClient) LogApiHeaders(version, method, endpoint string, requestHeaders, responseHeaders map[string]string, userId *uint64) {
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

		// Check against sensitive patterns
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

		// For request headers, use allowlist
		if useAllowlist {
			if allowedRequestHeaders[nameLower] {
				filtered[name] = value
			}
		} else {
			// For response headers, include all non-sensitive
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

	logLine, _ := json.Marshal(logData)
	l.log(labels, string(logLine))
}

// log adds a log entry to the batch.
func (l *LokiClient) log(labels map[string]string, logLine string) {
	if !l.enabled {
		return
	}

	tsNano := strconv.FormatInt(time.Now().UnixNano(), 10)

	l.batchMutex.Lock()
	l.batch = append(l.batch, lokiStream{
		Stream: labels,
		Values: [][]string{{tsNano, logLine}},
	})

	shouldFlush := len(l.batch) >= l.batchSize || time.Since(l.lastFlush) > 5*time.Second
	l.batchMutex.Unlock()

	if shouldFlush {
		select {
		case l.flushChan <- struct{}{}:
		default:
		}
	}
}

// backgroundFlusher periodically flushes logs to Loki.
func (l *LokiClient) backgroundFlusher() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-l.flushChan:
			l.flush()
		case <-ticker.C:
			l.flush()
		case <-l.closeChan:
			l.flush()
			return
		}
	}
}

// flush sends all buffered logs to Loki.
func (l *LokiClient) flush() {
	l.batchMutex.Lock()
	if len(l.batch) == 0 {
		l.batchMutex.Unlock()
		return
	}

	streams := l.batch
	l.batch = make([]lokiStream, 0)
	l.lastFlush = time.Now()
	l.batchMutex.Unlock()

	payload := lokiPushPayload{Streams: streams}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return
	}

	req, err := http.NewRequest("POST", l.url+"/loki/api/v1/push", bytes.NewBuffer(jsonData))
	if err != nil {
		return
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := l.client.Do(req)
	if err != nil {
		fmt.Printf("Loki push error: %v\n", err)
		return
	}
	resp.Body.Close()
}

// Close gracefully shuts down the Loki client.
func (l *LokiClient) Close() {
	if l.enabled {
		close(l.closeChan)
	}
}
