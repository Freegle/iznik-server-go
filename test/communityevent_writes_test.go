package test

import (
	"bytes"
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestCommunityEventCreate(t *testing.T) {
	prefix := uniquePrefix("cewr_create")
	userID := CreateTestUser(t, prefix, "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	body := fmt.Sprintf(`{"title":"Test Event %s","location":"Edinburgh","description":"A test community event","contactname":"Test","contactemail":"test@test.com","groupid":%d}`, prefix, groupID)
	req := httptest.NewRequest("POST", "/api/communityevent?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Greater(t, result["id"], float64(0))
}

func TestCommunityEventCreateUnauthorized(t *testing.T) {
	body := `{"title":"Test","location":"Edinburgh","description":"Test"}`
	req := httptest.NewRequest("POST", "/api/communityevent", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestCommunityEventCreateMissingFields(t *testing.T) {
	prefix := uniquePrefix("cewr_miss")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	// Missing title
	body := `{"location":"Edinburgh","description":"Test"}`
	req := httptest.NewRequest("POST", "/api/communityevent?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)

	// Missing location
	body = `{"title":"Test","description":"Test"}`
	req = httptest.NewRequest("POST", "/api/communityevent?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)

	// Missing description
	body = `{"title":"Test","location":"Edinburgh"}`
	req = httptest.NewRequest("POST", "/api/communityevent?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestCommunityEventSave(t *testing.T) {
	prefix := uniquePrefix("cewr_save")
	userID := CreateTestUser(t, prefix, "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Member")
	eventID := CreateTestCommunityEvent(t, userID, groupID)
	_, token := CreateTestSession(t, userID)

	// Update title and description
	body := fmt.Sprintf(`{"id":%d,"title":"Updated Title","description":"Updated Description"}`, eventID)
	req := httptest.NewRequest("PATCH", "/api/communityevent?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, true, result["success"])

	// Verify the update by fetching
	resp, _ = getApp().Test(httptest.NewRequest("GET", fmt.Sprintf("/api/communityevent/%d", eventID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var event map[string]interface{}
	json2.Unmarshal(rsp(resp), &event)
	assert.Equal(t, "Updated Title", event["title"])
	assert.Equal(t, "Updated Description", event["description"])
}

func TestCommunityEventSaveUnauthorized(t *testing.T) {
	body := `{"id":1,"title":"Updated"}`
	req := httptest.NewRequest("PATCH", "/api/communityevent", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestCommunityEventSaveNonOwner(t *testing.T) {
	prefix := uniquePrefix("cewr_noown")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	otherID := CreateTestUser(t, prefix+"_other", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, ownerID, groupID, "Member")
	CreateTestMembership(t, otherID, groupID, "Member")
	eventID := CreateTestCommunityEvent(t, ownerID, groupID)
	_, otherToken := CreateTestSession(t, otherID)

	body := fmt.Sprintf(`{"id":%d,"title":"Hacked"}`, eventID)
	req := httptest.NewRequest("PATCH", "/api/communityevent?jwt="+otherToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestCommunityEventSaveByModerator(t *testing.T) {
	prefix := uniquePrefix("cewr_mod")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, ownerID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	eventID := CreateTestCommunityEvent(t, ownerID, groupID)
	_, modToken := CreateTestSession(t, modID)

	body := fmt.Sprintf(`{"id":%d,"title":"Mod Updated"}`, eventID)
	req := httptest.NewRequest("PATCH", "/api/communityevent?jwt="+modToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestCommunityEventAddGroup(t *testing.T) {
	prefix := uniquePrefix("cewr_addg")
	userID := CreateTestUser(t, prefix, "User")
	groupID := CreateTestGroup(t, prefix)
	group2ID := CreateTestGroup(t, prefix+"_2")
	CreateTestMembership(t, userID, groupID, "Member")
	eventID := CreateTestCommunityEvent(t, userID, groupID)
	_, token := CreateTestSession(t, userID)

	body := fmt.Sprintf(`{"id":%d,"action":"AddGroup","groupid":%d}`, eventID, group2ID)
	req := httptest.NewRequest("PATCH", "/api/communityevent?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify background task was queued for push_notify_group_mods
	db := database.DBConn
	var taskCount int64
	db.Raw("SELECT COUNT(*) FROM background_tasks WHERE task_type = 'push_notify_group_mods' AND processed_at IS NULL AND data LIKE ?",
		fmt.Sprintf("%%\"group_id\": %d%%", group2ID)).Scan(&taskCount)
	assert.Equal(t, int64(1), taskCount, "Expected push_notify_group_mods task to be queued for communityevent AddGroup")
}

func TestCommunityEventRemoveGroup(t *testing.T) {
	prefix := uniquePrefix("cewr_remg")
	userID := CreateTestUser(t, prefix, "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Member")
	eventID := CreateTestCommunityEvent(t, userID, groupID)
	_, token := CreateTestSession(t, userID)

	body := fmt.Sprintf(`{"id":%d,"action":"RemoveGroup","groupid":%d}`, eventID, groupID)
	req := httptest.NewRequest("PATCH", "/api/communityevent?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestCommunityEventAddDate(t *testing.T) {
	prefix := uniquePrefix("cewr_addd")
	userID := CreateTestUser(t, prefix, "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Member")
	eventID := CreateTestCommunityEvent(t, userID, groupID)
	_, token := CreateTestSession(t, userID)

	body := fmt.Sprintf(`{"id":%d,"action":"AddDate","start":"2026-03-01T10:00:00Z","end":"2026-03-01T18:00:00Z"}`, eventID)
	req := httptest.NewRequest("PATCH", "/api/communityevent?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestCommunityEventRemoveDate(t *testing.T) {
	prefix := uniquePrefix("cewr_remd")
	userID := CreateTestUser(t, prefix, "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Member")
	eventID := CreateTestCommunityEvent(t, userID, groupID)
	_, token := CreateTestSession(t, userID)

	// Get existing dates
	db := database.DBConn
	var dateID uint64
	db.Raw("SELECT id FROM communityevents_dates WHERE eventid = ? LIMIT 1", eventID).Scan(&dateID)

	body := fmt.Sprintf(`{"id":%d,"action":"RemoveDate","dateid":%d}`, eventID, dateID)
	req := httptest.NewRequest("PATCH", "/api/communityevent?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestCommunityEventSetPhoto(t *testing.T) {
	prefix := uniquePrefix("cewr_photo")
	userID := CreateTestUser(t, prefix, "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Member")
	eventID := CreateTestCommunityEvent(t, userID, groupID)
	_, token := CreateTestSession(t, userID)

	// Create a test image
	db := database.DBConn
	db.Exec("INSERT INTO communityevents_images (eventid, contenttype) VALUES (?, 'image/jpeg')", eventID)
	var photoID uint64
	db.Raw("SELECT id FROM communityevents_images WHERE eventid = ? ORDER BY id DESC LIMIT 1", eventID).Scan(&photoID)

	body := fmt.Sprintf(`{"id":%d,"action":"SetPhoto","photoid":%d}`, eventID, photoID)
	req := httptest.NewRequest("PATCH", "/api/communityevent?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestCommunityEventHold(t *testing.T) {
	prefix := uniquePrefix("cewr_hold")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, ownerID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	eventID := CreateTestCommunityEvent(t, ownerID, groupID)
	_, modToken := CreateTestSession(t, modID)

	body := fmt.Sprintf(`{"id":%d,"action":"Hold"}`, eventID)
	req := httptest.NewRequest("PATCH", "/api/communityevent?jwt="+modToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify heldby is set
	db := database.DBConn
	var heldby *uint64
	db.Raw("SELECT heldby FROM communityevents WHERE id = ?", eventID).Scan(&heldby)
	assert.NotNil(t, heldby)
	assert.Equal(t, modID, *heldby)
}

func TestCommunityEventRelease(t *testing.T) {
	prefix := uniquePrefix("cewr_rel")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, ownerID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	eventID := CreateTestCommunityEvent(t, ownerID, groupID)
	_, modToken := CreateTestSession(t, modID)

	// Hold first
	db := database.DBConn
	db.Exec("UPDATE communityevents SET heldby = ? WHERE id = ?", modID, eventID)

	// Release
	body := fmt.Sprintf(`{"id":%d,"action":"Release"}`, eventID)
	req := httptest.NewRequest("PATCH", "/api/communityevent?jwt="+modToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify heldby is cleared
	var heldby *uint64
	db.Raw("SELECT heldby FROM communityevents WHERE id = ?", eventID).Scan(&heldby)
	assert.Nil(t, heldby)
}

func TestCommunityEventDelete(t *testing.T) {
	prefix := uniquePrefix("cewr_del")
	userID := CreateTestUser(t, prefix, "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Member")
	eventID := CreateTestCommunityEvent(t, userID, groupID)
	_, token := CreateTestSession(t, userID)

	resp, _ := getApp().Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/communityevent/%d?jwt=%s", eventID, token), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, true, result["success"])

	// Verify soft-deleted
	db := database.DBConn
	var deleted int
	db.Raw("SELECT deleted FROM communityevents WHERE id = ?", eventID).Scan(&deleted)
	assert.Equal(t, 1, deleted)
}

func TestCommunityEventDeleteUnauthorized(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("DELETE", "/api/communityevent/1", nil))
	assert.Equal(t, 401, resp.StatusCode)
}

func TestCommunityEventDeleteNonOwner(t *testing.T) {
	prefix := uniquePrefix("cewr_dno")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	otherID := CreateTestUser(t, prefix+"_other", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, ownerID, groupID, "Member")
	CreateTestMembership(t, otherID, groupID, "Member")
	eventID := CreateTestCommunityEvent(t, ownerID, groupID)
	_, otherToken := CreateTestSession(t, otherID)

	resp, _ := getApp().Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/communityevent/%d?jwt=%s", eventID, otherToken), nil))
	assert.Equal(t, 403, resp.StatusCode)
}

func TestCommunityEventDeleteByModerator(t *testing.T) {
	prefix := uniquePrefix("cewr_dmod")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, ownerID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	eventID := CreateTestCommunityEvent(t, ownerID, groupID)
	_, modToken := CreateTestSession(t, modID)

	resp, _ := getApp().Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/communityevent/%d?jwt=%s", eventID, modToken), nil))
	assert.Equal(t, 200, resp.StatusCode)
}
