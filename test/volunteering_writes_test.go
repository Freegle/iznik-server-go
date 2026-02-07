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
