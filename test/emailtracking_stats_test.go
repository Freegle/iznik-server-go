package test

import (
	json2 "encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/emailtracking"
	"github.com/stretchr/testify/assert"
)

// =============================================================================
// Helper functions for email stats tests
// =============================================================================

// createTestTrackingRecordWithType creates a tracking record with a specific email type and dates.
func createTestTrackingRecordWithType(t *testing.T, emailType string, sentAt time.Time, opened bool, clicked bool) uint64 {
	db := database.DBConn

	tracking := &emailtracking.EmailTracking{
		TrackingID:     uniquePrefix("stats") + "-" + emailType,
		EmailType:      emailType,
		RecipientEmail: "stats@example.com",
		SentAt:         &sentAt,
	}

	if opened {
		now := sentAt.Add(time.Hour)
		tracking.OpenedAt = &now
	}
	if clicked {
		now := sentAt.Add(2 * time.Hour)
		tracking.ClickedAt = &now
	}

	result := db.Create(tracking)
	assert.NoError(t, result.Error)

	return tracking.ID
}

// createTestClickRecord creates a click record linked to a tracking record.
func createTestClickRecord(t *testing.T, trackingID uint64, linkURL string, clickedAt time.Time) uint64 {
	db := database.DBConn

	click := &emailtracking.EmailTrackingClick{
		EmailTrackingID: trackingID,
		LinkURL:         linkURL,
		ClickedAt:       clickedAt,
	}

	result := db.Create(click)
	assert.NoError(t, result.Error)

	return click.ID
}

// cleanupTestTrackingByID removes tracking and related click records by tracking table ID.
func cleanupTestTrackingByID(ids []uint64) {
	db := database.DBConn
	for _, id := range ids {
		db.Exec("DELETE FROM email_tracking_clicks WHERE email_tracking_id = ?", id)
		db.Exec("DELETE FROM email_tracking WHERE id = ?", id)
	}
}

// =============================================================================
// Tests for GET /api/email/stats/timeseries
// =============================================================================

func TestTimeSeries_Unauthorized(t *testing.T) {
	// No auth.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/email/stats/timeseries", nil))
	assert.Equal(t, 401, resp.StatusCode)
}

func TestTimeSeries_ForbiddenForRegularUser(t *testing.T) {
	prefix := uniquePrefix("tsforbid")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/email/stats/timeseries?jwt="+token, nil))
	assert.Equal(t, 403, resp.StatusCode)
}

func TestTimeSeries_SupportUserAccess(t *testing.T) {
	prefix := uniquePrefix("tssupport")
	userID := CreateTestUser(t, prefix, "Support")
	_, token := CreateTestSession(t, userID)

	// Create some test tracking data.
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)
	ids := []uint64{
		createTestTrackingRecordWithType(t, "digest", yesterday, true, false),
		createTestTrackingRecordWithType(t, "digest", yesterday, false, false),
		createTestTrackingRecordWithType(t, "notification", now, true, true),
	}
	defer cleanupTestTrackingByID(ids)

	start := now.AddDate(0, 0, -7).Format("2006-01-02")
	end := now.Format("2006-01-02")

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/email/stats/timeseries?jwt="+token+"&start="+start+"&end="+end, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	// Should have data and period keys.
	assert.NotNil(t, result["data"])
	assert.NotNil(t, result["period"])

	period := result["period"].(map[string]interface{})
	assert.Equal(t, start, period["start"])
	assert.Equal(t, end, period["end"])
}

func TestTimeSeries_DefaultDateRange(t *testing.T) {
	prefix := uniquePrefix("tsdefault")
	userID := CreateTestUser(t, prefix, "Support")
	_, token := CreateTestSession(t, userID)

	// No start/end parameters - should default to last 30 days.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/email/stats/timeseries?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.NotNil(t, result["period"])
}

func TestTimeSeries_TypeFilter(t *testing.T) {
	prefix := uniquePrefix("tstype")
	userID := CreateTestUser(t, prefix, "Support")
	_, token := CreateTestSession(t, userID)

	now := time.Now()
	ids := []uint64{
		createTestTrackingRecordWithType(t, "digest", now, false, false),
		createTestTrackingRecordWithType(t, "notification", now, false, false),
	}
	defer cleanupTestTrackingByID(ids)

	start := now.AddDate(0, 0, -1).Format("2006-01-02")
	end := now.AddDate(0, 0, 1).Format("2006-01-02")

	// Filter by type.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/email/stats/timeseries?jwt="+token+"&type=digest&start="+start+"&end="+end, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	period := result["period"].(map[string]interface{})
	assert.Equal(t, "digest", period["type"])
}

// =============================================================================
// Tests for GET /api/email/stats/bytype
// =============================================================================

func TestStatsByType_Unauthorized(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/email/stats/bytype", nil))
	assert.Equal(t, 401, resp.StatusCode)
}

func TestStatsByType_ForbiddenForRegularUser(t *testing.T) {
	prefix := uniquePrefix("sbtforbid")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/email/stats/bytype?jwt="+token, nil))
	assert.Equal(t, 403, resp.StatusCode)
}

func TestStatsByType_SupportUserAccess(t *testing.T) {
	prefix := uniquePrefix("sbtsupport")
	userID := CreateTestUser(t, prefix, "Support")
	_, token := CreateTestSession(t, userID)

	now := time.Now()
	ids := []uint64{
		createTestTrackingRecordWithType(t, "digest", now, true, true),
		createTestTrackingRecordWithType(t, "digest", now, true, false),
		createTestTrackingRecordWithType(t, "notification", now, false, false),
	}
	defer cleanupTestTrackingByID(ids)

	start := now.AddDate(0, 0, -1).Format("2006-01-02")
	end := now.AddDate(0, 0, 1).Format("2006-01-02")

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/email/stats/bytype?jwt="+token+"&start="+start+"&end="+end, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	assert.NotNil(t, result["data"])
	assert.NotNil(t, result["period"])

	// Check that data is an array with type stats.
	data := result["data"].([]interface{})
	assert.Greater(t, len(data), 0)

	// Verify structure of first entry.
	first := data[0].(map[string]interface{})
	assert.NotNil(t, first["email_type"])
	assert.NotNil(t, first["total_sent"])
}

func TestStatsByType_NoDateRange(t *testing.T) {
	prefix := uniquePrefix("sbtnodate")
	userID := CreateTestUser(t, prefix, "Support")
	_, token := CreateTestSession(t, userID)

	// Without date range should return all stats.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/email/stats/bytype?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
}

// =============================================================================
// Tests for GET /api/email/stats/clicks
// =============================================================================

func TestTopClickedLinks_Unauthorized(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/email/stats/clicks", nil))
	assert.Equal(t, 401, resp.StatusCode)
}

func TestTopClickedLinks_ForbiddenForRegularUser(t *testing.T) {
	prefix := uniquePrefix("tclforbid")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/email/stats/clicks?jwt="+token, nil))
	assert.Equal(t, 403, resp.StatusCode)
}

func TestTopClickedLinks_SupportUserAccess(t *testing.T) {
	prefix := uniquePrefix("tclsupport")
	userID := CreateTestUser(t, prefix, "Support")
	_, token := CreateTestSession(t, userID)

	now := time.Now()
	trackingID := createTestTrackingRecordWithType(t, "digest", now, false, true)
	clickID1 := createTestClickRecord(t, trackingID, "https://www.ilovefreegle.org/message/12345", now)
	clickID2 := createTestClickRecord(t, trackingID, "https://www.ilovefreegle.org/message/67890", now)
	clickID3 := createTestClickRecord(t, trackingID, "https://www.ilovefreegle.org/give", now)
	defer func() {
		db := database.DBConn
		db.Exec("DELETE FROM email_tracking_clicks WHERE id IN (?, ?, ?)", clickID1, clickID2, clickID3)
		cleanupTestTrackingByID([]uint64{trackingID})
	}()

	start := now.AddDate(0, 0, -1).Format("2006-01-02")
	end := now.AddDate(0, 0, 1).Format("2006-01-02")

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/email/stats/clicks?jwt="+token+"&start="+start+"&end="+end, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	assert.NotNil(t, result["data"])
	assert.NotNil(t, result["total"])
	assert.NotNil(t, result["aggregate"])
	assert.NotNil(t, result["period"])
}

func TestTopClickedLinks_Aggregated(t *testing.T) {
	prefix := uniquePrefix("tclagg")
	userID := CreateTestUser(t, prefix, "Support")
	_, token := CreateTestSession(t, userID)

	now := time.Now()
	trackingID := createTestTrackingRecordWithType(t, "digest", now, false, true)
	// Two URLs with same pattern but different IDs.
	clickID1 := createTestClickRecord(t, trackingID, "https://www.ilovefreegle.org/message/111", now)
	clickID2 := createTestClickRecord(t, trackingID, "https://www.ilovefreegle.org/message/222", now)
	defer func() {
		db := database.DBConn
		db.Exec("DELETE FROM email_tracking_clicks WHERE id IN (?, ?)", clickID1, clickID2)
		cleanupTestTrackingByID([]uint64{trackingID})
	}()

	start := now.AddDate(0, 0, -1).Format("2006-01-02")
	end := now.AddDate(0, 0, 1).Format("2006-01-02")

	// Aggregated mode (default).
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/email/stats/clicks?jwt="+token+"&start="+start+"&end="+end+"&aggregate=true", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, true, result["aggregate"])
}

func TestTopClickedLinks_NotAggregated(t *testing.T) {
	prefix := uniquePrefix("tclnoagg")
	userID := CreateTestUser(t, prefix, "Support")
	_, token := CreateTestSession(t, userID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/email/stats/clicks?jwt="+token+"&aggregate=false", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, false, result["aggregate"])
}

func TestTopClickedLinks_LimitParam(t *testing.T) {
	prefix := uniquePrefix("tcllimit")
	userID := CreateTestUser(t, prefix, "Support")
	_, token := CreateTestSession(t, userID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/email/stats/clicks?jwt="+token+"&limit=2", nil))
	assert.Equal(t, 200, resp.StatusCode)
}

func TestTopClickedLinks_AdminUserAccess(t *testing.T) {
	prefix := uniquePrefix("tcladmin")
	userID := CreateTestUser(t, prefix, "Admin")
	_, token := CreateTestSession(t, userID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/email/stats/clicks?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
}
