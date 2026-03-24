package test

import (
	json2 "encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	flog "github.com/freegle/iznik-server-go/log"
	"github.com/stretchr/testify/assert"
)

func TestGetLogsMessages(t *testing.T) {
	prefix := uniquePrefix("LogsMsg")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Owner")
	_, token := CreateTestSession(t, userID)

	// Create a log entry.
	db := database.DBConn
	db.Exec("INSERT INTO logs (type, subtype, groupid, user, timestamp, text) VALUES (?, ?, ?, ?, NOW(), 'test log')",
		flog.LOG_TYPE_MESSAGE, flog.LOG_SUBTYPE_RECEIVED, groupID, userID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/modtools/logs?logtype=messages&groupid=%d&jwt=%s", groupID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Contains(t, result, "logs")
	assert.Contains(t, result, "context")
}

func TestGetLogsMemberships(t *testing.T) {
	prefix := uniquePrefix("LogsMem")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Owner")
	_, token := CreateTestSession(t, userID)

	db := database.DBConn
	db.Exec("INSERT INTO logs (type, subtype, groupid, user, timestamp, text) VALUES (?, ?, ?, ?, NOW(), 'test join')",
		flog.LOG_TYPE_GROUP, flog.LOG_SUBTYPE_JOINED, groupID, userID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/modtools/logs?logtype=memberships&groupid=%d&jwt=%s", groupID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
}

func TestGetLogsNotModerator(t *testing.T) {
	prefix := uniquePrefix("LogsNoMod")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/modtools/logs?logtype=messages&groupid=%d&jwt=%s", groupID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(2), result["ret"])
}

func TestGetLogsNotLoggedIn(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/modtools/logs?logtype=messages&groupid=1", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(2), result["ret"])
}

func TestGetLogsPagination(t *testing.T) {
	prefix := uniquePrefix("LogsPag")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Owner")
	_, token := CreateTestSession(t, userID)

	db := database.DBConn
	for i := 0; i < 5; i++ {
		db.Exec("INSERT INTO logs (type, subtype, groupid, user, timestamp, text) VALUES (?, ?, ?, ?, NOW(), ?)",
			flog.LOG_TYPE_MESSAGE, flog.LOG_SUBTYPE_RECEIVED, groupID, userID, fmt.Sprintf("page test %d", i))
	}

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/modtools/logs?logtype=messages&groupid=%d&limit=2&jwt=%s", groupID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	logs := result["logs"].([]interface{})
	assert.LessOrEqual(t, len(logs), 2)

	ctx := result["context"].(map[string]interface{})
	assert.Contains(t, ctx, "id")
}

func TestGetLogsV2Path(t *testing.T) {
	req := httptest.NewRequest("GET", "/apiv2/modtools/logs", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestGetLogsUserReturnsAllTypes(t *testing.T) {
	// Verify that logtype=user returns logs of ALL types (not just Message/User),
	// but excludes User/Created and User/Merged subtypes.
	prefix := uniquePrefix("LogsUserAll")
	groupID := CreateTestGroup(t, prefix)
	modID := CreateTestUser(t, prefix+"_mod", "User")
	targetUserID := CreateTestUser(t, prefix+"_target", "User")
	CreateTestMembership(t, modID, groupID, "Owner")
	CreateTestMembership(t, targetUserID, groupID, "Member")
	_, token := CreateTestSession(t, modID)

	db := database.DBConn

	// 1. Group/Joined log — previously excluded by the type filter bug.
	db.Exec("INSERT INTO logs (type, subtype, groupid, user, timestamp, text) VALUES (?, ?, ?, ?, NOW(), 'joined group')",
		flog.LOG_TYPE_GROUP, flog.LOG_SUBTYPE_JOINED, groupID, targetUserID)

	// 2. Message/Received log — always included.
	db.Exec("INSERT INTO logs (type, subtype, groupid, user, timestamp, text) VALUES (?, ?, ?, ?, NOW(), 'received msg')",
		flog.LOG_TYPE_MESSAGE, flog.LOG_SUBTYPE_RECEIVED, groupID, targetUserID)

	// 3. User/Created log — should be EXCLUDED by the fix.
	db.Exec("INSERT INTO logs (type, subtype, groupid, user, timestamp, text) VALUES (?, ?, ?, ?, NOW(), 'user created')",
		flog.LOG_TYPE_USER, flog.LOG_SUBTYPE_CREATED, groupID, targetUserID)

	// 4. User/Merged log — should be EXCLUDED by the fix.
	db.Exec("INSERT INTO logs (type, subtype, groupid, user, timestamp, text) VALUES (?, ?, ?, ?, NOW(), 'user merged')",
		flog.LOG_TYPE_USER, flog.LOG_SUBTYPE_MERGED, groupID, targetUserID)

	// 5. Config/Edit log (byuser) — should be included via the byuser match.
	db.Exec("INSERT INTO logs (type, subtype, groupid, byuser, timestamp, text) VALUES (?, ?, ?, ?, NOW(), 'config edited')",
		flog.LOG_TYPE_CONFIG, flog.LOG_SUBTYPE_EDIT, groupID, targetUserID)

	// Query with logtype=user for the target user.
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/modtools/logs?logtype=user&userid=%d&groupid=%d&limit=100&jwt=%s",
		targetUserID, groupID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	logs, ok := result["logs"].([]interface{})
	assert.True(t, ok, "logs should be an array")

	// Collect the type/subtype pairs returned.
	type logKey struct {
		Type    string
		Subtype string
	}
	found := map[logKey]bool{}
	for _, entry := range logs {
		e := entry.(map[string]interface{})
		logType, _ := e["type"].(string)
		logSubtype := ""
		if s, ok := e["subtype"].(string); ok {
			logSubtype = s
		}
		found[logKey{logType, logSubtype}] = true
	}

	// Group/Joined MUST be returned (this was the bug — it was previously filtered out).
	assert.True(t, found[logKey{flog.LOG_TYPE_GROUP, flog.LOG_SUBTYPE_JOINED}],
		"Group/Joined log should be returned for logtype=user")

	// Message/Received MUST be returned.
	assert.True(t, found[logKey{flog.LOG_TYPE_MESSAGE, flog.LOG_SUBTYPE_RECEIVED}],
		"Message/Received log should be returned for logtype=user")

	// Config/Edit MUST be returned (matched via byuser).
	assert.True(t, found[logKey{flog.LOG_TYPE_CONFIG, flog.LOG_SUBTYPE_EDIT}],
		"Config/Edit log should be returned for logtype=user")

	// User/Created MUST NOT be returned (excluded by the fix).
	assert.False(t, found[logKey{flog.LOG_TYPE_USER, flog.LOG_SUBTYPE_CREATED}],
		"User/Created log should NOT be returned for logtype=user")

	// User/Merged MUST NOT be returned (excluded by the fix).
	assert.False(t, found[logKey{flog.LOG_TYPE_USER, flog.LOG_SUBTYPE_MERGED}],
		"User/Merged log should NOT be returned for logtype=user")
}
