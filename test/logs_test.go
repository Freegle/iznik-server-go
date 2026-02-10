package test

import (
	json2 "encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
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
	db.Exec("INSERT INTO logs (type, subtype, groupid, user, timestamp, text) VALUES ('Message', 'Received', ?, ?, NOW(), 'test log')",
		groupID, userID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/logs?logtype=messages&groupid=%d&jwt=%s", groupID, token), nil)
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
	db.Exec("INSERT INTO logs (type, subtype, groupid, user, timestamp, text) VALUES ('Group', 'Joined', ?, ?, NOW(), 'test join')",
		groupID, userID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/logs?logtype=memberships&groupid=%d&jwt=%s", groupID, token), nil)
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

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/logs?logtype=messages&groupid=%d&jwt=%s", groupID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(2), result["ret"])
}

func TestGetLogsNotLoggedIn(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/logs?logtype=messages&groupid=1", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

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
		db.Exec("INSERT INTO logs (type, subtype, groupid, user, timestamp, text) VALUES ('Message', 'Received', ?, ?, NOW(), ?)",
			groupID, userID, fmt.Sprintf("page test %d", i))
	}

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/logs?logtype=messages&groupid=%d&limit=2&jwt=%s", groupID, token), nil)
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
	req := httptest.NewRequest("GET", "/apiv2/logs", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)
}
