// Package systemlogs provides an API endpoint for querying logs from Grafana Loki.
package systemlogs

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

// LogEntry represents a single log entry returned from the API.
type LogEntry struct {
	ID        string                 `json:"id"`
	Timestamp string                 `json:"timestamp"`
	Source    string                 `json:"source"`
	Type      string                 `json:"type,omitempty"`
	Subtype   string                 `json:"subtype,omitempty"`
	Level     string                 `json:"level,omitempty"`
	UserID    *uint64                `json:"user_id,omitempty"`
	ByUserID  *uint64                `json:"byuser_id,omitempty"`
	GroupID   *uint64                `json:"group_id,omitempty"`
	MessageID *uint64                `json:"message_id,omitempty"`
	Text      string                 `json:"text,omitempty"`
	TraceID   string                 `json:"trace_id,omitempty"`
	SessionID string                 `json:"session_id,omitempty"`
	Raw       map[string]interface{} `json:"raw,omitempty"`
}

// LogsResponse is the API response structure.
type LogsResponse struct {
	Logs  []LogEntry `json:"logs"`
	Stats struct {
		TotalReturned int            `json:"total_returned"`
		QueryTimeMs   int64          `json:"query_time_ms"`
		Sources       map[string]int `json:"sources"`
	} `json:"stats"`
}

// TraceSummary represents a collapsed trace group for summary mode.
type TraceSummary struct {
	TraceID        string   `json:"trace_id"`
	FirstLog       LogEntry `json:"first_log"`
	ChildCount     int      `json:"child_count"`
	Sources        []string `json:"sources"`
	RouteSummary   []string `json:"route_summary,omitempty"`
	FirstTimestamp string   `json:"first_timestamp"`
	LastTimestamp  string   `json:"last_timestamp"`
}

// SummaryResponse is the API response for summary mode.
type SummaryResponse struct {
	Summaries []TraceSummary `json:"summaries"`
	Stats     struct {
		TotalTraces   int   `json:"total_traces"`
		TotalLogs     int   `json:"total_logs"`
		QueryTimeMs   int64 `json:"query_time_ms"`
	} `json:"stats"`
}

// LokiQueryResponse represents Loki's query_range response.
type LokiQueryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Stream map[string]string `json:"stream"`
			Values [][]string        `json:"values"` // [timestamp_ns, log_line]
		} `json:"result"`
	} `json:"data"`
}

// RequireModeratorMiddleware checks that the user has at least Mod role on some group.
func RequireModeratorMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID, sessionID, _ := user.GetJWTFromRequest(c)
		if userID == 0 {
			return fiber.NewError(fiber.StatusUnauthorized, "Authentication required")
		}

		db := database.DBConn

		// Check user exists and session is valid.
		var userInfo struct {
			ID         uint64 `json:"id"`
			Systemrole string `json:"systemrole"`
		}

		db.Raw("SELECT users.id, users.systemrole FROM sessions INNER JOIN users ON users.id = sessions.userid WHERE sessions.id = ? AND users.id = ? LIMIT 1", sessionID, userID).Scan(&userInfo)

		if userInfo.ID == 0 {
			return fiber.NewError(fiber.StatusUnauthorized, "Invalid session")
		}

		// Admin and Support can access everything.
		if userInfo.Systemrole == "Support" || userInfo.Systemrole == "Admin" {
			c.Locals("systemrole", userInfo.Systemrole)
			c.Locals("userid", userID)
			return c.Next()
		}

		// Check if user is a moderator of any group.
		var modCount int64
		db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND role IN ('Moderator', 'Owner')", userID).Scan(&modCount)

		if modCount == 0 {
			return fiber.NewError(fiber.StatusForbidden, "Moderator role required")
		}

		c.Locals("systemrole", userInfo.Systemrole)
		c.Locals("userid", userID)
		return c.Next()
	}
}

// GetLogs handles GET /api/systemlogs.
func GetLogs(c *fiber.Ctx) error {
	startTime := time.Now()

	lokiURL := os.Getenv("LOKI_URL")
	if lokiURL == "" {
		lokiURL = "http://loki:3100"
	}

	// Parse query parameters.
	sources := c.Query("sources", "") // Empty = no source filter, show all
	types := c.Query("types", "")
	subtypes := c.Query("subtypes", "")
	levels := c.Query("levels", "")
	search := c.Query("search", "")
	start := c.Query("start", "1m")
	end := c.Query("end", "now")
	limitStr := c.Query("limit", "100")
	direction := c.Query("direction", "backward")
	userIDStr := c.Query("userid", "")
	groupIDStr := c.Query("groupid", "")
	msgIDStr := c.Query("msgid", "")
	traceID := c.Query("trace_id", "")
	sessionID := c.Query("session_id", "")
	ipAddress := c.Query("ip", "")
	email := c.Query("email", "")
	summaryMode := c.Query("summary", "") == "true"

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 1000 {
		limit = 100
	}

	// Access control: check if user can view these logs.
	currentUserID := c.Locals("userid").(uint64)
	systemRole, _ := c.Locals("systemrole").(string)

	// If filtering by specific user, check access.
	if userIDStr != "" {
		targetUserID, _ := strconv.ParseUint(userIDStr, 10, 64)
		if !canViewUserLogs(currentUserID, targetUserID, systemRole) {
			return fiber.NewError(fiber.StatusForbidden, "Cannot view logs for this user")
		}
	}

	// If filtering by specific group, check access.
	if groupIDStr != "" {
		targetGroupID, _ := strconv.ParseUint(groupIDStr, 10, 64)
		if !canViewGroupLogs(currentUserID, targetGroupID, systemRole) {
			return fiber.NewError(fiber.StatusForbidden, "Cannot view logs for this group")
		}
	}

	// Parse time range.
	startTs, endTs := parseTimeRange(start, end)

	// In summary mode, fetch more logs to ensure we get enough unique traces.
	fetchLimit := limit
	if summaryMode {
		fetchLimit = limit * 10 // Fetch more to get enough unique traces
		if fetchLimit > 5000 {
			fetchLimit = 5000
		}
	}

	// Query Loki.
	var logs []LogEntry

	// When both user_id AND email are provided, we want an OR search:
	// Find logs matching user_id OR logs containing the email.
	// Run two queries in parallel and merge results.
	if userIDStr != "" && email != "" {
		logs = queryLokiUserOrEmail(lokiURL, sources, types, subtypes, levels, search, userIDStr, groupIDStr, msgIDStr, traceID, sessionID, ipAddress, email, startTs, endTs, fetchLimit, direction)
	} else {
		// Standard query with all filters as AND conditions.
		query := buildLogQLQuery(sources, types, subtypes, levels, search, userIDStr, groupIDStr, msgIDStr, traceID, sessionID, ipAddress, email)

		// Use parallel source queries when we have multiple sources and need balanced results:
		// - Summary mode with entity filters (user, group, message, IP)
		// - Trace-specific queries (fetching all logs for a trace)
		// This ensures we get representative samples from all sources, not just the most frequent.
		hasEntityFilter := userIDStr != "" || groupIDStr != "" || msgIDStr != "" || ipAddress != ""
		hasTraceFilter := traceID != ""
		needsBalancedSources := (summaryMode && hasEntityFilter) || hasTraceFilter

		if needsBalancedSources && sources != "" {
			sourceList := strings.Split(sources, ",")
			limitPerSource := fetchLimit / len(sourceList)
			if limitPerSource < 100 {
				limitPerSource = 100
			}
			logs = queryLokiMultipleSources(lokiURL, query, sourceList, startTs, endTs, limitPerSource, direction)
		} else {
			logs, err = queryLoki(lokiURL, query, startTs, endTs, fetchLimit, direction)
			if err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("Failed to query Loki: %v", err))
			}
		}
	}

	// Summary mode: group by trace_id and return summaries.
	if summaryMode {
		summaries := buildTraceSummaries(logs, limit)
		response := SummaryResponse{
			Summaries: summaries,
		}
		response.Stats.TotalTraces = len(summaries)
		response.Stats.TotalLogs = len(logs)
		response.Stats.QueryTimeMs = time.Since(startTime).Milliseconds()
		return c.JSON(response)
	}

	// Normal mode: return all logs.
	response := LogsResponse{
		Logs: logs,
	}
	response.Stats.TotalReturned = len(logs)
	response.Stats.QueryTimeMs = time.Since(startTime).Milliseconds()
	response.Stats.Sources = countBySources(logs)

	return c.JSON(response)
}

// buildLogQLQuery constructs a LogQL query from parameters.
func buildLogQLQuery(sources, types, subtypes, levels, search, userID, groupID, msgID, traceID, sessionID, ipAddress, email string) string {
	// Build label selector.
	labelParts := []string{`app="freegle"`}

	// Source filter.
	if sources != "" {
		sourceList := strings.Split(sources, ",")
		if len(sourceList) == 1 {
			labelParts = append(labelParts, fmt.Sprintf(`source="%s"`, sourceList[0]))
		} else {
			labelParts = append(labelParts, fmt.Sprintf(`source=~"%s"`, strings.Join(sourceList, "|")))
		}
	}

	// Note: trace_id and session_id are stored in the JSON body, not as Loki labels.
	// They are filtered below using JSON field matching after the `| json` stage.

	// Type filter (for logs_table source).
	if types != "" {
		typeList := strings.Split(types, ",")
		if len(typeList) == 1 {
			labelParts = append(labelParts, fmt.Sprintf(`type="%s"`, typeList[0]))
		} else {
			labelParts = append(labelParts, fmt.Sprintf(`type=~"%s"`, strings.Join(typeList, "|")))
		}
	}

	// Subtype filter.
	if subtypes != "" {
		subtypeList := strings.Split(subtypes, ",")
		if len(subtypeList) == 1 {
			labelParts = append(labelParts, fmt.Sprintf(`subtype="%s"`, subtypeList[0]))
		} else {
			labelParts = append(labelParts, fmt.Sprintf(`subtype=~"%s"`, strings.Join(subtypeList, "|")))
		}
	}

	// Level filter (for client source).
	if levels != "" {
		levelList := strings.Split(levels, ",")
		if len(levelList) == 1 {
			labelParts = append(labelParts, fmt.Sprintf(`level="%s"`, levelList[0]))
		} else {
			labelParts = append(labelParts, fmt.Sprintf(`level=~"%s"`, strings.Join(levelList, "|")))
		}
	}

	// Group ID label filter.
	if groupID != "" {
		labelParts = append(labelParts, fmt.Sprintf(`groupid="%s"`, groupID))
	}

	// User ID label filter (indexed, fast).
	if userID != "" {
		labelParts = append(labelParts, fmt.Sprintf(`user_id="%s"`, userID))
	}

	query := "{" + strings.Join(labelParts, ", ") + "}"

	// Add JSON parsing pipeline.
	query += " | json"

	if msgID != "" {
		query += fmt.Sprintf(` | msgid = %s or msg_id = %s`, msgID, msgID)
	}

	// Trace ID filter (string field in JSON body).
	if traceID != "" {
		query += fmt.Sprintf(` | trace_id = "%s"`, traceID)
	}

	// Session ID filter (string field in JSON body).
	if sessionID != "" {
		query += fmt.Sprintf(` | session_id = "%s"`, sessionID)
	}

	if ipAddress != "" {
		query += fmt.Sprintf(` | ip = "%s" or ip_address = "%s" or client_ip = "%s"`, ipAddress, ipAddress, ipAddress)
	}

	// Email filter - search across common email-related fields.
	if email != "" {
		escapedEmail := escapeRegex(email)
		query += fmt.Sprintf(` |~ "(?i)%s"`, escapedEmail)
	}

	// Text search.
	if search != "" {
		query += fmt.Sprintf(` |~ "(?i)%s"`, escapeRegex(search))
	}

	return query
}

// escapeRegex escapes special regex characters in search string.
func escapeRegex(s string) string {
	special := []string{"\\", ".", "+", "*", "?", "^", "$", "(", ")", "[", "]", "{", "}", "|"}
	for _, char := range special {
		s = strings.ReplaceAll(s, char, "\\"+char)
	}
	return s
}

// parseTimeRange converts time range parameters to Unix timestamps.
func parseTimeRange(start, end string) (int64, int64) {
	now := time.Now()
	var startTs, endTs int64

	// Parse end time.
	if end == "now" {
		endTs = now.UnixNano()
	} else {
		t, err := time.Parse(time.RFC3339, end)
		if err != nil {
			endTs = now.UnixNano()
		} else {
			endTs = t.UnixNano()
		}
	}

	// Parse start time (relative or absolute).
	if strings.HasSuffix(start, "m") {
		mins, _ := strconv.Atoi(strings.TrimSuffix(start, "m"))
		startTs = now.Add(-time.Duration(mins) * time.Minute).UnixNano()
	} else if strings.HasSuffix(start, "h") {
		hours, _ := strconv.Atoi(strings.TrimSuffix(start, "h"))
		startTs = now.Add(-time.Duration(hours) * time.Hour).UnixNano()
	} else if strings.HasSuffix(start, "d") {
		days, _ := strconv.Atoi(strings.TrimSuffix(start, "d"))
		startTs = now.Add(-time.Duration(days) * 24 * time.Hour).UnixNano()
	} else {
		t, err := time.Parse(time.RFC3339, start)
		if err != nil {
			startTs = now.Add(-1 * time.Minute).UnixNano()
		} else {
			startTs = t.UnixNano()
		}
	}

	return startTs, endTs
}

// queryLoki queries the Loki API and returns parsed log entries.
func queryLoki(lokiURL, query string, startNs, endNs int64, limit int, direction string) ([]LogEntry, error) {
	// Build query URL.
	queryURL := fmt.Sprintf("%s/loki/api/v1/query_range", lokiURL)

	params := url.Values{}
	params.Set("query", query)
	params.Set("start", strconv.FormatInt(startNs, 10))
	params.Set("end", strconv.FormatInt(endNs, 10))
	params.Set("limit", strconv.Itoa(limit))
	params.Set("direction", direction)

	fullURL := queryURL + "?" + params.Encode()

	// Make HTTP request.
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(fullURL)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Loki returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response.
	var lokiResp LokiQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&lokiResp); err != nil {
		return nil, fmt.Errorf("failed to parse Loki response: %w", err)
	}

	if lokiResp.Status != "success" {
		return nil, fmt.Errorf("Loki query failed with status: %s", lokiResp.Status)
	}

	// Convert to LogEntry slice.
	var logs []LogEntry
	entryIndex := 0
	for _, stream := range lokiResp.Data.Result {
		for _, value := range stream.Values {
			if len(value) < 2 {
				continue
			}

			timestampNs, _ := strconv.ParseInt(value[0], 10, 64)
			logLine := value[1]

			entry := parseLogEntry(timestampNs, logLine, stream.Stream, entryIndex)
			logs = append(logs, entry)
			entryIndex++
		}
	}

	return logs, nil
}

// queryLokiMultipleSources queries each source separately in parallel and merges results.
// This ensures we get representative samples from all sources, not just the most frequent one.
// The baseQuery should contain the source regex filter which will be replaced for each source.
func queryLokiMultipleSources(lokiURL string, baseQuery string, sources []string, startNs, endNs int64, limitPerSource int, direction string) []LogEntry {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var allLogs []LogEntry

	// Build regex pattern to match in the query.
	regexPattern := `source=~"` + strings.Join(sources, "|") + `"`

	for _, source := range sources {
		wg.Add(1)
		go func(src string) {
			defer wg.Done()

			// Replace regex source filter with exact source filter.
			query := strings.Replace(baseQuery, regexPattern, `source="`+src+`"`, 1)

			logs, err := queryLoki(lokiURL, query, startNs, endNs, limitPerSource, direction)
			if err != nil {
				// Log error but continue with other sources.
				fmt.Printf("Error querying source %s: %v\n", src, err)
				return
			}

			mu.Lock()
			allLogs = append(allLogs, logs...)
			mu.Unlock()
		}(source)
	}

	wg.Wait()

	// Sort merged results by timestamp (descending for backward, ascending for forward).
	if direction == "backward" {
		sort.Slice(allLogs, func(i, j int) bool {
			return allLogs[i].Timestamp > allLogs[j].Timestamp
		})
	} else {
		sort.Slice(allLogs, func(i, j int) bool {
			return allLogs[i].Timestamp < allLogs[j].Timestamp
		})
	}

	return allLogs
}

// queryLokiUserOrEmail runs two queries in parallel: one filtering by user_id, one by email.
// Results are merged and deduplicated, supporting OR logic for user/email searches.
func queryLokiUserOrEmail(lokiURL, sources, types, subtypes, levels, search, userID, groupID, msgID, traceID, sessionID, ipAddress, email string, startNs, endNs int64, limit int, direction string) []LogEntry {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var allLogs []LogEntry
	seenIDs := make(map[string]bool)

	// Query 1: Filter by user_id (without email text search)
	wg.Add(1)
	go func() {
		defer wg.Done()
		query := buildLogQLQuery(sources, types, subtypes, levels, search, userID, groupID, msgID, traceID, sessionID, ipAddress, "")
		logs, err := queryLoki(lokiURL, query, startNs, endNs, limit, direction)
		if err != nil {
			fmt.Printf("Error querying by user_id: %v\n", err)
			return
		}
		mu.Lock()
		for _, log := range logs {
			if !seenIDs[log.ID] {
				seenIDs[log.ID] = true
				allLogs = append(allLogs, log)
			}
		}
		mu.Unlock()
	}()

	// Query 2: Filter by email text (without user_id filter)
	wg.Add(1)
	go func() {
		defer wg.Done()
		query := buildLogQLQuery(sources, types, subtypes, levels, search, "", groupID, msgID, traceID, sessionID, ipAddress, email)
		logs, err := queryLoki(lokiURL, query, startNs, endNs, limit, direction)
		if err != nil {
			fmt.Printf("Error querying by email: %v\n", err)
			return
		}
		mu.Lock()
		for _, log := range logs {
			if !seenIDs[log.ID] {
				seenIDs[log.ID] = true
				allLogs = append(allLogs, log)
			}
		}
		mu.Unlock()
	}()

	wg.Wait()

	// Sort merged results by timestamp (descending for backward, ascending for forward).
	if direction == "backward" {
		sort.Slice(allLogs, func(i, j int) bool {
			return allLogs[i].Timestamp > allLogs[j].Timestamp
		})
	} else {
		sort.Slice(allLogs, func(i, j int) bool {
			return allLogs[i].Timestamp < allLogs[j].Timestamp
		})
	}

	// Apply limit after merging
	if len(allLogs) > limit {
		allLogs = allLogs[:limit]
	}

	return allLogs
}

// parseLogEntry converts a Loki log line to a LogEntry.
func parseLogEntry(timestampNs int64, logLine string, labels map[string]string, index int) LogEntry {
	timestamp := time.Unix(0, timestampNs).Format(time.RFC3339Nano)

	entry := LogEntry{
		ID:        fmt.Sprintf("%d-%s-%d", timestampNs, labels["source"], index),
		Timestamp: timestamp,
		Source:    labels["source"],
		Type:      labels["type"],
		Subtype:   labels["subtype"],
		Level:     labels["level"],
	}

	// For client logs, event_type is a label, not type/subtype.
	// Copy it to Type so the frontend can use it consistently.
	if labels["source"] == "client" && labels["event_type"] != "" {
		entry.Type = labels["event_type"]
	}

	// Parse JSON log line.
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(logLine), &raw); err == nil {
		entry.Raw = raw

		// Extract common fields.
		if v, ok := raw["user_id"]; ok {
			if uid, ok := v.(float64); ok {
				u := uint64(uid)
				entry.UserID = &u
			}
		}
		if v, ok := raw["by_user"]; ok {
			if uid, ok := v.(float64); ok {
				u := uint64(uid)
				entry.ByUserID = &u
			}
		}
		if v, ok := raw["byuser"]; ok {
			if uid, ok := v.(float64); ok {
				u := uint64(uid)
				entry.ByUserID = &u
			}
		}
		if v, ok := raw["group_id"]; ok {
			if gid, ok := v.(float64); ok {
				g := uint64(gid)
				entry.GroupID = &g
			}
		}
		if v, ok := raw["groupid"]; ok {
			if gid, ok := v.(float64); ok {
				g := uint64(gid)
				entry.GroupID = &g
			}
		}
		if v, ok := raw["msg_id"]; ok {
			if mid, ok := v.(float64); ok {
				m := uint64(mid)
				entry.MessageID = &m
			}
		}
		if v, ok := raw["msgid"]; ok {
			if mid, ok := v.(float64); ok {
				m := uint64(mid)
				entry.MessageID = &m
			}
		}
		if v, ok := raw["text"].(string); ok {
			entry.Text = v
		}
		if v, ok := raw["trace_id"].(string); ok {
			entry.TraceID = v
		}
		if v, ok := raw["session_id"].(string); ok {
			entry.SessionID = v
		}

		// For client logs, inject event_type from labels into raw if not already present.
		if labels["source"] == "client" && labels["event_type"] != "" {
			if _, exists := raw["event_type"]; !exists {
				raw["event_type"] = labels["event_type"]
			}
		}
	} else {
		// Not JSON, store as text.
		entry.Text = logLine
	}

	return entry
}

// canViewUserLogs checks if the current user can view logs for the target user.
func canViewUserLogs(currentUserID, targetUserID uint64, systemRole string) bool {
	if systemRole == "Support" || systemRole == "Admin" {
		return true
	}

	db := database.DBConn

	// Check if current user moderates any group that target user is a member of.
	var count int64
	db.Raw(`
		SELECT COUNT(*) FROM memberships m1
		INNER JOIN memberships m2 ON m1.groupid = m2.groupid
		WHERE m1.userid = ? AND m1.role IN ('Moderator', 'Owner')
		AND m2.userid = ?
	`, currentUserID, targetUserID).Scan(&count)

	return count > 0
}

// canViewGroupLogs checks if the current user can view logs for the target group.
func canViewGroupLogs(currentUserID, targetGroupID uint64, systemRole string) bool {
	if systemRole == "Support" || systemRole == "Admin" {
		return true
	}

	db := database.DBConn

	// Check if current user moderates the target group.
	var count int64
	db.Raw(`
		SELECT COUNT(*) FROM memberships
		WHERE userid = ? AND groupid = ? AND role IN ('Moderator', 'Owner')
	`, currentUserID, targetGroupID).Scan(&count)

	return count > 0
}

// countBySources counts logs by source for stats.
func countBySources(logs []LogEntry) map[string]int {
	counts := make(map[string]int)
	for _, log := range logs {
		counts[log.Source]++
	}
	return counts
}

// buildTraceSummaries groups logs by trace_id and returns summary entries.
func buildTraceSummaries(logs []LogEntry, limit int) []TraceSummary {
	// Group logs by trace_id.
	traceGroups := make(map[string][]LogEntry)
	var standaloneGroups []TraceSummary

	for _, log := range logs {
		if log.TraceID != "" {
			traceGroups[log.TraceID] = append(traceGroups[log.TraceID], log)
		} else {
			// Logs without trace_id become their own "group".
			standaloneGroups = append(standaloneGroups, TraceSummary{
				TraceID:        "",
				FirstLog:       log,
				ChildCount:     1,
				Sources:        []string{log.Source},
				FirstTimestamp: log.Timestamp,
				LastTimestamp:  log.Timestamp,
			})
		}
	}

	// Build summaries for trace groups.
	var summaries []TraceSummary
	for traceID, traceLogs := range traceGroups {
		summary := buildSingleTraceSummary(traceID, traceLogs)
		summaries = append(summaries, summary)
	}

	// Combine and sort by first timestamp (most recent first for backward direction).
	summaries = append(summaries, standaloneGroups...)

	// Sort by first timestamp descending.
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].FirstTimestamp > summaries[j].FirstTimestamp
	})

	// Apply limit.
	if len(summaries) > limit {
		summaries = summaries[:limit]
	}

	return summaries
}

// buildSingleTraceSummary creates a summary for a single trace group.
func buildSingleTraceSummary(traceID string, logs []LogEntry) TraceSummary {
	// Sort logs by timestamp to find first/last and build route summary.
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].Timestamp < logs[j].Timestamp
	})

	// Collect unique sources.
	sourceSet := make(map[string]bool)
	for _, log := range logs {
		sourceSet[log.Source] = true
	}
	var sources []string
	for source := range sourceSet {
		sources = append(sources, source)
	}

	// Build route summary from client events that have page_name/url.
	// Include page_view events and other client events with route info.
	var routeSummary []string
	for _, log := range logs {
		if log.Source == "client" && log.Raw != nil {
			pageName := ""
			if pn, ok := log.Raw["page_name"].(string); ok {
				pageName = pn
			} else if rawURL, ok := log.Raw["url"].(string); ok {
				pageName = rawURL
			}
			if pageName != "" {
				// Extract just the path from full URLs.
				if strings.HasPrefix(pageName, "http://") || strings.HasPrefix(pageName, "https://") {
					if parsed, err := url.Parse(pageName); err == nil {
						pageName = parsed.Path
						if parsed.RawQuery != "" {
							pageName += "?" + parsed.RawQuery
						}
					}
				}
				// Ensure path starts with /.
				if !strings.HasPrefix(pageName, "/") {
					pageName = "/" + pageName
				}
				// Avoid consecutive duplicates.
				if len(routeSummary) == 0 || routeSummary[len(routeSummary)-1] != pageName {
					routeSummary = append(routeSummary, pageName)
				}
			}
		}
	}

	// Find the best "parent" log to display.
	// Priority: client page_view > client > api > others.
	var parentLog LogEntry
	for _, log := range logs {
		if log.Source == "client" {
			if log.Raw != nil {
				if eventType, ok := log.Raw["event_type"].(string); ok && eventType == "page_view" {
					parentLog = log
					break
				}
			}
			if parentLog.ID == "" {
				parentLog = log
			}
		}
	}
	if parentLog.ID == "" {
		for _, log := range logs {
			if log.Source == "api" {
				parentLog = log
				break
			}
		}
	}
	if parentLog.ID == "" && len(logs) > 0 {
		parentLog = logs[0]
	}

	return TraceSummary{
		TraceID:        traceID,
		FirstLog:       parentLog,
		ChildCount:     len(logs),
		Sources:        sources,
		RouteSummary:   routeSummary,
		FirstTimestamp: logs[0].Timestamp,
		LastTimestamp:  logs[len(logs)-1].Timestamp,
	}
}
