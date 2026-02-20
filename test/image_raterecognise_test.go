package test

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

func TestImageRateRecognise(t *testing.T) {
	prefix := uniquePrefix("img_rr")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "Member")
	CreateTestMembership(t, userID, groupID, "Member")

	// Create a message and attachment
	msgID := CreateTestMessage(t, userID, groupID, "Rate recognise test "+prefix, 55.9533, -3.1883)
	attID := CreateTestAttachment(t, msgID)

	// Insert a recognise row for the attachment
	db := database.DBConn
	db.Exec("INSERT INTO messages_attachments_recognise (attid, info) VALUES (?, '{}')", attID)

	// Rate the recognition result via query params
	url := fmt.Sprintf("/api/image?raterecognise=Good&id=%d", attID)
	req := httptest.NewRequest("POST", url, nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Equal(t, "Success", result["status"])

	// Verify the rating was updated
	var rating string
	db.Raw("SELECT rating FROM messages_attachments_recognise WHERE attid = ?", attID).Scan(&rating)
	assert.Equal(t, "Good", rating)
}

func TestImageRateRecogniseMissingParams(t *testing.T) {
	// Missing id param - should fall through to normal body parsing (and fail because no body)
	req := httptest.NewRequest("POST", "/api/image?raterecognise=Good", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	// Without id, the raterecognise path is skipped. Empty body will be parsed
	// and fail with 400 (missing externaluid) since there's no rotate id either.
	assert.Equal(t, 400, resp.StatusCode)
}

func TestImageRateRecogniseMissingRating(t *testing.T) {
	// Missing raterecognise param - should fall through to normal body parsing
	req := httptest.NewRequest("POST", "/api/image?id=123", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	// Without raterecognise, it falls through to normal parsing.
	// Empty body will fail with 400.
	assert.Equal(t, 400, resp.StatusCode)
}
