package test

import (
	"bytes"
	json2 "encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	volunteering2 "github.com/freegle/iznik-server-go/volunteering"
	"github.com/stretchr/testify/assert"
)

func TestVolunteering(t *testing.T) {
	// Create test data for this test
	prefix := uniquePrefix("vol")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")
	volunteeringID := CreateTestVolunteering(t, userID, groupID)

	// Get non-existent volunteering - should return 404 (use very high ID guaranteed not to exist)
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/volunteering/999999999", nil))
	assert.Equal(t, 404, resp.StatusCode)

	// Get the volunteering we created
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/volunteering/"+fmt.Sprint(volunteeringID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var volunteering volunteering2.Volunteering
	json2.Unmarshal(rsp(resp), &volunteering)
	assert.Greater(t, volunteering.ID, uint64(0))
	assert.Greater(t, len(volunteering.Title), 0)
	assert.Greater(t, len(volunteering.Dates), 0)

	// List volunteering requires auth
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/volunteering", nil))
	assert.Equal(t, 401, resp.StatusCode)

	// Create a full test user with all relationships for authenticated requests
	_, token := CreateFullTestUser(t, prefix+"_auth")
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/volunteering?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var ids []uint64
	json2.Unmarshal(rsp(resp), &ids)
	assert.Greater(t, len(ids), 0)

	// Get volunteering by group
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/volunteering/group/"+fmt.Sprint(groupID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	json2.Unmarshal(rsp(resp), &ids)
	assert.Greater(t, len(ids), 0)
}

func TestVolunteering_InvalidID(t *testing.T) {
	// Non-integer ID should return 404
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/volunteering/notanint", nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestVolunteering_InvalidGroupID(t *testing.T) {
	// Non-existent group should return empty array
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/volunteering/group/999999999", nil))
	assert.Equal(t, 200, resp.StatusCode)
}

func TestVolunteering_V2Path(t *testing.T) {
	// Verify v2 paths work
	prefix := uniquePrefix("volv2")
	_, token := CreateFullTestUser(t, prefix)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/apiv2/volunteering?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)
}

func TestVolunteering_PendingList(t *testing.T) {
	prefix := uniquePrefix("volpend")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	// Create a regular user who creates a pending volunteering
	creatorID := CreateTestUser(t, prefix+"_creator", "User")
	CreateTestMembership(t, creatorID, groupID, "Member")

	db.Exec("INSERT INTO volunteering (userid, title, description, pending, deleted, expired) VALUES (?, 'Pending Vol', 'Pending desc', 1, 0, 0)", creatorID)
	var pendingID uint64
	db.Raw("SELECT id FROM volunteering WHERE userid = ? AND pending = 1 ORDER BY id DESC LIMIT 1", creatorID).Scan(&pendingID)
	assert.Greater(t, pendingID, uint64(0))
	db.Exec("INSERT INTO volunteering_groups (volunteeringid, groupid) VALUES (?, ?)", pendingID, groupID)

	// Create a moderator user for the same group
	modID := CreateTestUser(t, prefix+"_mod", "User")
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	// Moderator should see pending volunteering
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/volunteering?pending=true&jwt="+modToken, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var ids []uint64
	json2.Unmarshal(rsp(resp), &ids)
	assert.Contains(t, ids, pendingID)

	// Regular member should NOT see pending volunteering
	memberID := CreateTestUser(t, prefix+"_member", "User")
	CreateTestMembership(t, memberID, groupID, "Member")
	_, memberToken := CreateTestSession(t, memberID)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/volunteering?pending=true&jwt="+memberToken, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var memberIds []uint64
	json2.Unmarshal(rsp(resp), &memberIds)
	assert.NotContains(t, memberIds, pendingID)

	// Verify single fetch returns pending field
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/volunteering/"+fmt.Sprint(pendingID), nil))
	assert.Equal(t, 200, resp.StatusCode)
	var vol volunteering2.Volunteering
	json2.Unmarshal(rsp(resp), &vol)
	assert.True(t, vol.Pending)
}

func TestVolunteering_PendingListAdmin(t *testing.T) {
	prefix := uniquePrefix("voladm")
	db := database.DBConn
	groupID := CreateTestGroup(t, prefix)

	// Create a pending volunteering
	creatorID := CreateTestUser(t, prefix+"_creator", "User")
	CreateTestMembership(t, creatorID, groupID, "Member")
	db.Exec("INSERT INTO volunteering (userid, title, description, pending, deleted, expired) VALUES (?, 'Admin Pending Vol', 'Admin test', 1, 0, 0)", creatorID)
	var pendingID uint64
	db.Raw("SELECT id FROM volunteering WHERE userid = ? AND pending = 1 ORDER BY id DESC LIMIT 1", creatorID).Scan(&pendingID)
	db.Exec("INSERT INTO volunteering_groups (volunteeringid, groupid) VALUES (?, ?)", pendingID, groupID)

	// Admin should see all pending volunteering
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, adminToken := CreateTestSession(t, adminID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/volunteering?pending=true&jwt="+adminToken, nil))
	assert.Equal(t, 200, resp.StatusCode)
	var ids []uint64
	json2.Unmarshal(rsp(resp), &ids)
	assert.Contains(t, ids, pendingID)
}

func TestVolunteeringCreate(t *testing.T) {
	prefix := uniquePrefix("volwr_create")
	userID := CreateTestUser(t, prefix, "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	body := fmt.Sprintf(`{"title":"Test Vol %s","location":"Edinburgh","description":"A test volunteering opportunity","contactname":"Test","contactemail":"test@test.com","groupid":%d}`, prefix, groupID)
	req := httptest.NewRequest("POST", "/api/volunteering?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Greater(t, result["id"], float64(0))
}

func TestVolunteeringCreateUnauthorized(t *testing.T) {
	body := `{"title":"Test","location":"Edinburgh","description":"Test"}`
	req := httptest.NewRequest("POST", "/api/volunteering", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestVolunteeringCreateMissingFields(t *testing.T) {
	prefix := uniquePrefix("volwr_miss")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	// Missing title
	body := `{"location":"Edinburgh","description":"Test"}`
	req := httptest.NewRequest("POST", "/api/volunteering?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)

	// Missing location
	body = `{"title":"Test","description":"Test"}`
	req = httptest.NewRequest("POST", "/api/volunteering?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)

	// Missing description
	body = `{"title":"Test","location":"Edinburgh"}`
	req = httptest.NewRequest("POST", "/api/volunteering?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = getApp().Test(req)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestVolunteeringSave(t *testing.T) {
	prefix := uniquePrefix("volwr_save")
	userID := CreateTestUser(t, prefix, "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Member")
	volunteeringID := CreateTestVolunteering(t, userID, groupID)
	_, token := CreateTestSession(t, userID)

	// Update title and description
	body := fmt.Sprintf(`{"id":%d,"title":"Updated Title","description":"Updated Description"}`, volunteeringID)
	req := httptest.NewRequest("PATCH", "/api/volunteering?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, true, result["success"])

	// Verify the update by fetching
	resp, _ = getApp().Test(httptest.NewRequest("GET", fmt.Sprintf("/api/volunteering/%d", volunteeringID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var vol map[string]interface{}
	json2.Unmarshal(rsp(resp), &vol)
	assert.Equal(t, "Updated Title", vol["title"])
	assert.Equal(t, "Updated Description", vol["description"])
}

func TestVolunteeringSaveUnauthorized(t *testing.T) {
	body := `{"id":1,"title":"Updated"}`
	req := httptest.NewRequest("PATCH", "/api/volunteering", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestVolunteeringSaveNonOwner(t *testing.T) {
	prefix := uniquePrefix("volwr_noown")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	otherID := CreateTestUser(t, prefix+"_other", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, ownerID, groupID, "Member")
	CreateTestMembership(t, otherID, groupID, "Member")
	volunteeringID := CreateTestVolunteering(t, ownerID, groupID)
	_, otherToken := CreateTestSession(t, otherID)

	body := fmt.Sprintf(`{"id":%d,"title":"Hacked"}`, volunteeringID)
	req := httptest.NewRequest("PATCH", "/api/volunteering?jwt="+otherToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestVolunteeringSaveByModerator(t *testing.T) {
	prefix := uniquePrefix("volwr_mod")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, ownerID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	volunteeringID := CreateTestVolunteering(t, ownerID, groupID)
	_, modToken := CreateTestSession(t, modID)

	body := fmt.Sprintf(`{"id":%d,"title":"Mod Updated"}`, volunteeringID)
	req := httptest.NewRequest("PATCH", "/api/volunteering?jwt="+modToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestVolunteeringAddGroup(t *testing.T) {
	prefix := uniquePrefix("volwr_addg")
	userID := CreateTestUser(t, prefix, "User")
	groupID := CreateTestGroup(t, prefix)
	group2ID := CreateTestGroup(t, prefix+"_2")
	CreateTestMembership(t, userID, groupID, "Member")
	volunteeringID := CreateTestVolunteering(t, userID, groupID)
	_, token := CreateTestSession(t, userID)

	body := fmt.Sprintf(`{"id":%d,"action":"AddGroup","groupid":%d}`, volunteeringID, group2ID)
	req := httptest.NewRequest("PATCH", "/api/volunteering?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify background task was queued for push_notify_group_mods
	db := database.DBConn
	var taskCount int64
	db.Raw("SELECT COUNT(*) FROM background_tasks WHERE task_type = 'push_notify_group_mods' AND processed_at IS NULL AND data LIKE ?",
		fmt.Sprintf("%%\"group_id\": %d%%", group2ID)).Scan(&taskCount)
	assert.Equal(t, int64(1), taskCount, "Expected push_notify_group_mods task to be queued for volunteering AddGroup")
}

func TestVolunteeringRemoveGroup(t *testing.T) {
	prefix := uniquePrefix("volwr_remg")
	userID := CreateTestUser(t, prefix, "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Member")
	volunteeringID := CreateTestVolunteering(t, userID, groupID)
	_, token := CreateTestSession(t, userID)

	body := fmt.Sprintf(`{"id":%d,"action":"RemoveGroup","groupid":%d}`, volunteeringID, groupID)
	req := httptest.NewRequest("PATCH", "/api/volunteering?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestVolunteeringAddDate(t *testing.T) {
	prefix := uniquePrefix("volwr_addd")
	userID := CreateTestUser(t, prefix, "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Member")
	volunteeringID := CreateTestVolunteering(t, userID, groupID)
	_, token := CreateTestSession(t, userID)

	body := fmt.Sprintf(`{"id":%d,"action":"AddDate","start":"2026-03-01T10:00:00Z","end":"2026-03-15T18:00:00Z"}`, volunteeringID)
	req := httptest.NewRequest("PATCH", "/api/volunteering?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestVolunteeringRemoveDate(t *testing.T) {
	prefix := uniquePrefix("volwr_remd")
	userID := CreateTestUser(t, prefix, "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Member")
	volunteeringID := CreateTestVolunteering(t, userID, groupID)
	_, token := CreateTestSession(t, userID)

	// Get existing dates
	db := database.DBConn
	var dateID uint64
	db.Raw("SELECT id FROM volunteering_dates WHERE volunteeringid = ? LIMIT 1", volunteeringID).Scan(&dateID)

	body := fmt.Sprintf(`{"id":%d,"action":"RemoveDate","dateid":%d}`, volunteeringID, dateID)
	req := httptest.NewRequest("PATCH", "/api/volunteering?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestVolunteeringSetPhoto(t *testing.T) {
	prefix := uniquePrefix("volwr_photo")
	userID := CreateTestUser(t, prefix, "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Member")
	volunteeringID := CreateTestVolunteering(t, userID, groupID)
	_, token := CreateTestSession(t, userID)

	// Create a test image
	db := database.DBConn
	db.Exec("INSERT INTO volunteering_images (opportunityid, contenttype) VALUES (?, 'image/jpeg')", volunteeringID)
	var photoID uint64
	db.Raw("SELECT id FROM volunteering_images WHERE opportunityid = ? ORDER BY id DESC LIMIT 1", volunteeringID).Scan(&photoID)

	body := fmt.Sprintf(`{"id":%d,"action":"SetPhoto","photoid":%d}`, volunteeringID, photoID)
	req := httptest.NewRequest("PATCH", "/api/volunteering?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestVolunteeringRenew(t *testing.T) {
	prefix := uniquePrefix("volwr_renew")
	userID := CreateTestUser(t, prefix, "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Member")
	volunteeringID := CreateTestVolunteering(t, userID, groupID)
	_, token := CreateTestSession(t, userID)

	body := fmt.Sprintf(`{"id":%d,"action":"Renew"}`, volunteeringID)
	req := httptest.NewRequest("PATCH", "/api/volunteering?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify renewed timestamp and expired = 0
	db := database.DBConn
	var expired int
	var renewed *string
	db.Raw("SELECT expired, renewed FROM volunteering WHERE id = ?", volunteeringID).Row().Scan(&expired, &renewed)
	assert.Equal(t, 0, expired)
	assert.NotNil(t, renewed)
}

func TestVolunteeringExpire(t *testing.T) {
	prefix := uniquePrefix("volwr_expire")
	userID := CreateTestUser(t, prefix, "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Member")
	volunteeringID := CreateTestVolunteering(t, userID, groupID)
	_, token := CreateTestSession(t, userID)

	body := fmt.Sprintf(`{"id":%d,"action":"Expire"}`, volunteeringID)
	req := httptest.NewRequest("PATCH", "/api/volunteering?jwt="+token, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify expired = 1
	db := database.DBConn
	var expired int
	db.Raw("SELECT expired FROM volunteering WHERE id = ?", volunteeringID).Scan(&expired)
	assert.Equal(t, 1, expired)
}

func TestVolunteeringDelete(t *testing.T) {
	prefix := uniquePrefix("volwr_del")
	userID := CreateTestUser(t, prefix, "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Member")
	volunteeringID := CreateTestVolunteering(t, userID, groupID)
	_, token := CreateTestSession(t, userID)

	resp, _ := getApp().Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/volunteering/%d?jwt=%s", volunteeringID, token), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, true, result["success"])

	// Verify soft-deleted
	db := database.DBConn
	var deleted int
	db.Raw("SELECT deleted FROM volunteering WHERE id = ?", volunteeringID).Scan(&deleted)
	assert.Equal(t, 1, deleted)
}

func TestVolunteeringDeleteUnauthorized(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("DELETE", "/api/volunteering/1", nil))
	assert.Equal(t, 401, resp.StatusCode)
}

func TestVolunteeringDeleteNonOwner(t *testing.T) {
	prefix := uniquePrefix("volwr_dno")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	otherID := CreateTestUser(t, prefix+"_other", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, ownerID, groupID, "Member")
	CreateTestMembership(t, otherID, groupID, "Member")
	volunteeringID := CreateTestVolunteering(t, ownerID, groupID)
	_, otherToken := CreateTestSession(t, otherID)

	resp, _ := getApp().Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/volunteering/%d?jwt=%s", volunteeringID, otherToken), nil))
	assert.Equal(t, 403, resp.StatusCode)
}

func TestVolunteeringHold(t *testing.T) {
	prefix := uniquePrefix("volwr_hold")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, ownerID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	volunteeringID := CreateTestVolunteering(t, ownerID, groupID)
	_, modToken := CreateTestSession(t, modID)

	body := fmt.Sprintf(`{"id":%d,"action":"Hold"}`, volunteeringID)
	req := httptest.NewRequest("PATCH", "/api/volunteering?jwt="+modToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify heldby is set to the moderator
	db := database.DBConn
	var heldby *uint64
	db.Raw("SELECT heldby FROM volunteering WHERE id = ?", volunteeringID).Scan(&heldby)
	assert.NotNil(t, heldby)
	assert.Equal(t, modID, *heldby)
}

func TestVolunteeringRelease(t *testing.T) {
	prefix := uniquePrefix("volwr_rel")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, ownerID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	volunteeringID := CreateTestVolunteering(t, ownerID, groupID)
	_, modToken := CreateTestSession(t, modID)

	// First hold it
	db := database.DBConn
	db.Exec("UPDATE volunteering SET heldby = ? WHERE id = ?", modID, volunteeringID)

	// Then release it
	body := fmt.Sprintf(`{"id":%d,"action":"Release"}`, volunteeringID)
	req := httptest.NewRequest("PATCH", "/api/volunteering?jwt="+modToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify heldby is NULL
	var heldby *uint64
	db.Raw("SELECT heldby FROM volunteering WHERE id = ?", volunteeringID).Scan(&heldby)
	assert.Nil(t, heldby)
}

func TestVolunteeringHoldNonModerator(t *testing.T) {
	prefix := uniquePrefix("volwr_holdnm")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, ownerID, groupID, "Member")
	volunteeringID := CreateTestVolunteering(t, ownerID, groupID)
	_, ownerToken := CreateTestSession(t, ownerID)

	// Owner (non-mod) tries to hold - should succeed (canModify passes) but heldby should NOT be set
	body := fmt.Sprintf(`{"id":%d,"action":"Hold"}`, volunteeringID)
	req := httptest.NewRequest("PATCH", "/api/volunteering?jwt="+ownerToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify heldby is still NULL (isModerator check failed)
	db := database.DBConn
	var heldby *uint64
	db.Raw("SELECT heldby FROM volunteering WHERE id = ?", volunteeringID).Scan(&heldby)
	assert.Nil(t, heldby)
}

func TestVolunteeringPending(t *testing.T) {
	prefix := uniquePrefix("volwr_pend")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, ownerID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	volunteeringID := CreateTestVolunteering(t, ownerID, groupID)
	_, modToken := CreateTestSession(t, modID)

	// Set pending = 1
	db := database.DBConn
	db.Exec("UPDATE volunteering SET pending = 1 WHERE id = ?", volunteeringID)

	// Approve it (set pending = 0)
	pending := 0
	body := fmt.Sprintf(`{"id":%d,"pending":%d}`, volunteeringID, pending)
	req := httptest.NewRequest("PATCH", "/api/volunteering?jwt="+modToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify pending = 0
	var pendingVal int
	db.Raw("SELECT pending FROM volunteering WHERE id = ?", volunteeringID).Scan(&pendingVal)
	assert.Equal(t, 0, pendingVal)
}

func TestVolunteeringDeleteByModerator(t *testing.T) {
	prefix := uniquePrefix("volwr_dmod")
	ownerID := CreateTestUser(t, prefix+"_owner", "User")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, ownerID, groupID, "Member")
	CreateTestMembership(t, modID, groupID, "Moderator")
	volunteeringID := CreateTestVolunteering(t, ownerID, groupID)
	_, modToken := CreateTestSession(t, modID)

	resp, _ := getApp().Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/volunteering/%d?jwt=%s", volunteeringID, modToken), nil))
	assert.Equal(t, 200, resp.StatusCode)
}
