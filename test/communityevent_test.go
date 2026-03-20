package test

import (
	"bytes"
	json2 "encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/communityevent"
	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

func TestCommunityEvent(t *testing.T) {
	// Create test data for this test
	prefix := uniquePrefix("event")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")
	eventID := CreateTestCommunityEvent(t, userID, groupID)

	// Get non-existent event - should return 404
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/communityevents/1", nil))
	assert.Equal(t, 404, resp.StatusCode)

	// Get the event we created
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/communityevent/"+fmt.Sprint(eventID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var event communityevent.CommunityEvent
	json2.Unmarshal(rsp(resp), &event)
	assert.Greater(t, event.ID, uint64(0))
	assert.Greater(t, len(event.Title), 0)
	assert.Greater(t, len(event.Dates), 0)

	// List events requires auth
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/communityevent", nil))
	assert.Equal(t, 401, resp.StatusCode)

	// Create a full test user with all relationships for authenticated requests
	_, token := CreateFullTestUser(t, prefix+"_auth")
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/communityevent?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var ids []uint64
	json2.Unmarshal(rsp(resp), &ids)
	assert.Greater(t, len(ids), 0)

	// Get events by group
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/communityevent/group/"+fmt.Sprint(groupID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	json2.Unmarshal(rsp(resp), &ids)
	assert.Greater(t, len(ids), 0)
}

func TestCommunityEvent_InvalidID(t *testing.T) {
	// Non-integer ID should return 404
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/communityevent/notanint", nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestCommunityEvent_InvalidGroupID(t *testing.T) {
	// Non-existent group should return empty array
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/communityevent/group/999999999", nil))
	assert.Equal(t, 200, resp.StatusCode)
}

func TestCommunityEvent_V2Path(t *testing.T) {
	// Verify v2 paths work
	prefix := uniquePrefix("eventv2")
	_, token := CreateFullTestUser(t, prefix)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/apiv2/communityevent?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
}

func TestCommunityEvent_PendingList(t *testing.T) {
	prefix := uniquePrefix("eventpend")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	// Create a regular user who creates a pending event
	creatorID := CreateTestUser(t, prefix+"_creator", "User")
	CreateTestMembership(t, creatorID, groupID, "Member")

	// Create a pending event directly (not using helper which creates non-pending)
	db.Exec("INSERT INTO communityevents (userid, title, description, pending, deleted) VALUES (?, 'Pending Event', 'Pending description', 1, 0)", creatorID)
	var pendingID uint64
	db.Raw("SELECT id FROM communityevents WHERE userid = ? AND pending = 1 ORDER BY id DESC LIMIT 1", creatorID).Scan(&pendingID)
	assert.Greater(t, pendingID, uint64(0))
	db.Exec("INSERT INTO communityevents_groups (eventid, groupid) VALUES (?, ?)", pendingID, groupID)
	db.Exec("INSERT INTO communityevents_dates (eventid, `start`, `end`) VALUES (?, DATE_ADD(NOW(), INTERVAL 1 DAY), DATE_ADD(NOW(), INTERVAL 2 DAY))", pendingID)

	// Create a moderator user for the same group
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	// Moderator should see pending events
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/communityevent?pending=true&jwt="+modToken, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var ids []uint64
	json2.Unmarshal(rsp(resp), &ids)
	assert.Contains(t, ids, pendingID)

	// Regular member should NOT see pending events (they're not a mod)
	memberID := CreateTestUser(t, prefix+"_member", "User")
	CreateTestMembership(t, memberID, groupID, "Member")
	_, memberToken := CreateTestSession(t, memberID)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/communityevent?pending=true&jwt="+memberToken, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var memberIds []uint64
	json2.Unmarshal(rsp(resp), &memberIds)
	assert.NotContains(t, memberIds, pendingID)

	// Verify single fetch returns pending field
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/communityevent/"+fmt.Sprint(pendingID), nil))
	assert.Equal(t, 200, resp.StatusCode)
	var event communityevent.CommunityEvent
	json2.Unmarshal(rsp(resp), &event)
	assert.True(t, event.Pending)
}

func TestCommunityEvent_PendingListAdmin(t *testing.T) {
	prefix := uniquePrefix("eventadm")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	// Create a pending event on groupID.
	creatorID := CreateTestUser(t, prefix+"_creator", "User")
	CreateTestMembership(t, creatorID, groupID, "Member")
	db.Exec("INSERT INTO communityevents (userid, title, description, pending, deleted) VALUES (?, 'Admin Pending Event', 'Admin test', 1, 0)", creatorID)
	var pendingID uint64
	db.Raw("SELECT id FROM communityevents WHERE userid = ? AND pending = 1 ORDER BY id DESC LIMIT 1", creatorID).Scan(&pendingID)
	db.Exec("INSERT INTO communityevents_groups (eventid, groupid) VALUES (?, ?)", pendingID, groupID)
	db.Exec("INSERT INTO communityevents_dates (eventid, `start`, `end`) VALUES (?, DATE_ADD(NOW(), INTERVAL 1 DAY), DATE_ADD(NOW(), INTERVAL 2 DAY))", pendingID)

	// V1 parity: Admin who is also a Moderator on the group should see the event.
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	CreateTestMembership(t, adminID, groupID, "Moderator")
	_, adminToken := CreateTestSession(t, adminID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/communityevent?pending=true&jwt="+adminToken, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var ids []uint64
	json2.Unmarshal(rsp(resp), &ids)
	assert.Contains(t, ids, pendingID)
}

func TestCommunityEvent_PendingListAdminNotOnGroup(t *testing.T) {
	prefix := uniquePrefix("eventadm2")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)
	otherGroupID := CreateTestGroup(t, prefix+"_other")

	// Create a pending event on groupID.
	creatorID := CreateTestUser(t, prefix+"_creator", "User")
	CreateTestMembership(t, creatorID, groupID, "Member")
	db.Exec("INSERT INTO communityevents (userid, title, description, pending, deleted) VALUES (?, 'Other Group Event', 'Test', 1, 0)", creatorID)
	var pendingID uint64
	db.Raw("SELECT id FROM communityevents WHERE userid = ? AND pending = 1 ORDER BY id DESC LIMIT 1", creatorID).Scan(&pendingID)
	db.Exec("INSERT INTO communityevents_groups (eventid, groupid) VALUES (?, ?)", pendingID, groupID)
	db.Exec("INSERT INTO communityevents_dates (eventid, `start`, `end`) VALUES (?, DATE_ADD(NOW(), INTERVAL 1 DAY), DATE_ADD(NOW(), INTERVAL 2 DAY))", pendingID)

	// V1 parity: Admin who moderates a DIFFERENT group should NOT see events on groupID.
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	CreateTestMembership(t, adminID, otherGroupID, "Moderator")
	_, adminToken := CreateTestSession(t, adminID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/communityevent?pending=true&jwt="+adminToken, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var ids []uint64
	json2.Unmarshal(rsp(resp), &ids)
	assert.NotContains(t, ids, pendingID, "Admin should NOT see pending events from groups they don't moderate")
}

func TestCommunityEvent_PendingListExcludesExpired(t *testing.T) {
	// Pending events with past end dates should NOT appear in the listing.
	prefix := uniquePrefix("eventexp")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	creatorID := CreateTestUser(t, prefix+"_creator", "User")
	CreateTestMembership(t, creatorID, groupID, "Member")

	// Create a pending event with a PAST end date.
	db.Exec("INSERT INTO communityevents (userid, title, description, pending, deleted) VALUES (?, 'Expired Event', 'Past', 1, 0)", creatorID)
	var expiredID uint64
	db.Raw("SELECT id FROM communityevents WHERE userid = ? AND title = 'Expired Event' ORDER BY id DESC LIMIT 1", creatorID).Scan(&expiredID)
	assert.Greater(t, expiredID, uint64(0))
	db.Exec("INSERT INTO communityevents_groups (eventid, groupid) VALUES (?, ?)", expiredID, groupID)
	db.Exec("INSERT INTO communityevents_dates (eventid, `start`, `end`) VALUES (?, DATE_SUB(NOW(), INTERVAL 2 DAY), DATE_SUB(NOW(), INTERVAL 1 DAY))", expiredID)

	// Create a pending event with a FUTURE end date.
	db.Exec("INSERT INTO communityevents (userid, title, description, pending, deleted) VALUES (?, 'Future Event', 'Upcoming', 1, 0)", creatorID)
	var futureID uint64
	db.Raw("SELECT id FROM communityevents WHERE userid = ? AND title = 'Future Event' ORDER BY id DESC LIMIT 1", creatorID).Scan(&futureID)
	assert.Greater(t, futureID, uint64(0))
	db.Exec("INSERT INTO communityevents_groups (eventid, groupid) VALUES (?, ?)", futureID, groupID)
	db.Exec("INSERT INTO communityevents_dates (eventid, `start`, `end`) VALUES (?, DATE_ADD(NOW(), INTERVAL 1 DAY), DATE_ADD(NOW(), INTERVAL 2 DAY))", futureID)

	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/communityevent?pending=true&jwt="+modToken, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var ids []uint64
	json2.Unmarshal(rsp(resp), &ids)
	assert.Contains(t, ids, futureID, "Future pending event should appear")
	assert.NotContains(t, ids, expiredID, "Expired pending event should NOT appear")
}

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
