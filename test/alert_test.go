package test

import (
	json2 "encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

func createTestAlert(t *testing.T, userID uint64, subject string) uint64 {
	db := database.DBConn
	result := db.Exec("INSERT INTO alerts (createdby, `from`, `to`, subject, text, html, created) VALUES (?, 'Test', 'Mods', ?, 'Test text', '<p>Test</p>', NOW())", userID, subject)
	assert.NoError(t, result.Error)

	var id uint64
	db.Raw("SELECT id FROM alerts WHERE createdby = ? ORDER BY id DESC LIMIT 1", userID).Scan(&id)
	assert.Greater(t, id, uint64(0))
	return id
}

func createTestAlertTracking(t *testing.T, alertID uint64, userID uint64) uint64 {
	db := database.DBConn
	result := db.Exec("INSERT INTO alerts_tracking (alertid, userid, shown, clicked) VALUES (?, ?, 1, 0)", alertID, userID)
	assert.NoError(t, result.Error)

	var id uint64
	db.Raw("SELECT id FROM alerts_tracking WHERE alertid = ? AND userid = ? ORDER BY id DESC LIMIT 1", alertID, userID).Scan(&id)
	assert.Greater(t, id, uint64(0))
	return id
}

func TestGetAlert(t *testing.T) {
	prefix := uniquePrefix("AlertGet")
	userID := CreateTestUser(t, prefix, "User")
	alertID := createTestAlert(t, userID, prefix+"_subject")

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/alert/%d", alertID), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	a := result["alert"].(map[string]interface{})
	assert.Equal(t, float64(alertID), a["id"])
	assert.Equal(t, prefix+"_subject", a["subject"])
	assert.Equal(t, "Test", a["from"])
	assert.Equal(t, "Mods", a["to"])
	assert.Equal(t, "Test text", a["text"])
	assert.Equal(t, "<p>Test</p>", a["html"])

	// Non-admin should not get stats.
	assert.NotContains(t, a, "stats")
}

func TestGetAlertWithAdminStats(t *testing.T) {
	prefix := uniquePrefix("AlertAdmStat")
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, adminID)

	alertID := createTestAlert(t, adminID, prefix+"_subject")
	trackingUserID := CreateTestUser(t, prefix+"_tracked", "User")
	createTestAlertTracking(t, alertID, trackingUserID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/alert/%d?jwt=%s", alertID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	a := result["alert"].(map[string]interface{})
	assert.Equal(t, float64(alertID), a["id"])

	// Admin should get stats.
	assert.Contains(t, a, "stats")
	stats := a["stats"].(map[string]interface{})
	assert.Contains(t, stats, "reached")
	assert.Contains(t, stats, "shown")
	assert.Contains(t, stats, "clicked")
	assert.Equal(t, float64(1), stats["reached"])
}

func TestGetAlertNotFound(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/alert/999999999", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestListAlerts(t *testing.T) {
	prefix := uniquePrefix("AlertList")
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, adminID)

	createTestAlert(t, adminID, prefix+"_alert1")
	createTestAlert(t, adminID, prefix+"_alert2")

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/alert?jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	alerts := result["alerts"].([]interface{})
	assert.GreaterOrEqual(t, len(alerts), 2)
}

func TestListAlertsNotAdmin(t *testing.T) {
	prefix := uniquePrefix("AlertListNA")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/alert?jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestListAlertsNotLoggedIn(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/alert", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestCreateAlert(t *testing.T) {
	prefix := uniquePrefix("AlertCreate")
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, adminID)

	body := fmt.Sprintf(`{"from":"Admin","to":"Mods","subject":"%s_subject","text":"Alert body text"}`, prefix)
	req := httptest.NewRequest("PUT", fmt.Sprintf("/api/alert?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Greater(t, result["id"].(float64), float64(0))

	// Verify the alert was created with correct defaults.
	db := database.DBConn
	alertID := uint64(result["id"].(float64))
	var a struct {
		Subject  string
		Text     string
		Html     string
		Askclick int
		Tryhard  int
	}
	db.Raw("SELECT subject, text, html, askclick, tryhard FROM alerts WHERE id = ?", alertID).Scan(&a)
	assert.Equal(t, prefix+"_subject", a.Subject)
	assert.Equal(t, "Alert body text", a.Text)
	assert.Equal(t, 1, a.Askclick)
	assert.Equal(t, 1, a.Tryhard)
}

func TestCreateAlertDefaults(t *testing.T) {
	prefix := uniquePrefix("AlertDef")
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, adminID)

	// Create with minimal fields - to and html should get defaults.
	body := fmt.Sprintf(`{"from":"Admin","subject":"%s_subject","text":"Line1\nLine2"}`, prefix)
	req := httptest.NewRequest("PUT", fmt.Sprintf("/api/alert?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify defaults.
	db := database.DBConn
	alertID := uint64(result["id"].(float64))
	var a struct {
		To   string
		Html string
	}
	db.Raw("SELECT `to`, html FROM alerts WHERE id = ?", alertID).Scan(&a)
	assert.Equal(t, "Mods", a.To)
	assert.Equal(t, "Line1<br>Line2", a.Html)
}

func TestCreateAlertNotAdmin(t *testing.T) {
	prefix := uniquePrefix("AlertCreateNA")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	body := `{"from":"User","subject":"Test","text":"Test text"}`
	req := httptest.NewRequest("PUT", fmt.Sprintf("/api/alert?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestCreateAlertNotLoggedIn(t *testing.T) {
	body := `{"from":"Anon","subject":"Test","text":"Test text"}`
	req := httptest.NewRequest("PUT", "/api/alert", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestRecordAlertClick(t *testing.T) {
	prefix := uniquePrefix("AlertClick")
	userID := CreateTestUser(t, prefix, "User")
	alertID := createTestAlert(t, userID, prefix+"_subject")
	trackID := createTestAlertTracking(t, alertID, userID)

	body := fmt.Sprintf(`{"action":"clicked","trackid":%d}`, trackID)
	req := httptest.NewRequest("POST", "/api/alert", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify clicked was incremented.
	db := database.DBConn
	var clicked int64
	db.Raw("SELECT clicked FROM alerts_tracking WHERE id = ?", trackID).Scan(&clicked)
	assert.Equal(t, int64(1), clicked)
}

func TestRecordAlertClickNoAction(t *testing.T) {
	// POST without action should still return 200.
	body := `{}`
	req := httptest.NewRequest("POST", "/api/alert", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
}

func TestGetAlertV2Path(t *testing.T) {
	prefix := uniquePrefix("AlertV2")
	userID := CreateTestUser(t, prefix, "User")
	alertID := createTestAlert(t, userID, prefix+"_subject")

	req := httptest.NewRequest("GET", fmt.Sprintf("/apiv2/alert/%d", alertID), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
}
