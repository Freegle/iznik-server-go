package test

import (
	json2 "encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

func createTestVisualiseData(t *testing.T, prefix string) (uint64, uint64, uint64) {
	db := database.DBConn

	// Create two users with profiles.
	fromUserID := CreateTestUser(t, prefix+"_from", "User")
	toUserID := CreateTestUser(t, prefix+"_to", "User")

	// Create a message for the visualisation.
	msgID := CreateTestMessage(t, fromUserID, CreateTestGroup(t, prefix), prefix+"_offer", 55.957, -3.205)

	// Create an attachment for the message.
	attID := CreateTestAttachment(t, msgID)

	// Insert into visualise table.
	result := db.Exec("INSERT INTO visualise (msgid, attid, fromuser, touser, fromlat, fromlng, tolat, tolng, distance) "+
		"VALUES (?, ?, ?, ?, 55.957, -3.205, 55.960, -3.200, 500)",
		msgID, attID, fromUserID, toUserID)
	assert.NoError(t, result.Error)

	var visID uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&visID)
	return visID, fromUserID, toUserID
}

func TestGetVisualise(t *testing.T) {
	prefix := uniquePrefix("Visualise")
	visID, _, _ := createTestVisualiseData(t, prefix)
	_ = visID

	// Query with a bounding box that includes Edinburgh.
	req := httptest.NewRequest("GET", "/api/visualise?swlat=55.0&swlng=-4.0&nelat=56.0&nelng=-2.0&limit=10", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	list, ok := result["list"].([]interface{})
	assert.True(t, ok)
	assert.Greater(t, len(list), 0, "Should return at least one visualise entry")

	// Verify structure of first item.
	item := list[0].(map[string]interface{})
	assert.Contains(t, item, "id")
	assert.Contains(t, item, "msgid")
	assert.Contains(t, item, "fromlat")
	assert.Contains(t, item, "fromlng")
	assert.Contains(t, item, "tolat")
	assert.Contains(t, item, "tolng")
	assert.Contains(t, item, "distance")
	assert.Contains(t, item, "attachment")
	assert.Contains(t, item, "from")
	assert.Contains(t, item, "to")
	assert.Contains(t, item, "others")

	// Verify from/to have expected fields.
	from := item["from"].(map[string]interface{})
	assert.Contains(t, from, "id")
	assert.Contains(t, from, "icon")

	att := item["attachment"].(map[string]interface{})
	assert.Contains(t, att, "id")
	assert.Contains(t, att, "path")
	assert.Contains(t, att, "thumb")
}

func TestGetVisualiseEmpty(t *testing.T) {
	// Query with a bounding box that has no data (middle of ocean).
	req := httptest.NewRequest("GET", "/api/visualise?swlat=0.1&swlng=0.1&nelat=0.2&nelng=0.2", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	list := result["list"].([]interface{})
	assert.Equal(t, 0, len(list))
}

func TestGetVisualiseNoCoords(t *testing.T) {
	// Query with no coordinates returns empty list.
	req := httptest.NewRequest("GET", "/api/visualise", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
}

func TestGetVisualisePagination(t *testing.T) {
	prefix := uniquePrefix("VisPag")

	// Create two visualisation entries.
	visID1, _, _ := createTestVisualiseData(t, prefix+"_1")
	visID2, _, _ := createTestVisualiseData(t, prefix+"_2")

	// Use the higher ID as context to get only older entries.
	higherID := visID2
	if visID1 > visID2 {
		higherID = visID1
	}

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/visualise?swlat=55.0&swlng=-4.0&nelat=56.0&nelng=-2.0&limit=100&context=%d", higherID), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	// Context should filter to entries with id < higherID.
	list := result["list"].([]interface{})
	for _, entry := range list {
		item := entry.(map[string]interface{})
		assert.Less(t, item["id"].(float64), float64(higherID))
	}
}

func TestGetVisualiseV2Path(t *testing.T) {
	req := httptest.NewRequest("GET", "/apiv2/visualise?swlat=55.0&swlng=-4.0&nelat=56.0&nelng=-2.0", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)
}
