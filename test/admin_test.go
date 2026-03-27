package test

import (
	"bytes"
	json2 "encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

// createTestAdmin creates an admin record for testing and returns its ID.
func createTestAdmin(t *testing.T, createdby uint64, groupid uint64, subject string) uint64 {
	db := database.DBConn
	db.Exec("INSERT INTO admins (createdby, groupid, subject, text, created) VALUES (?, ?, ?, 'Test admin text', NOW())",
		createdby, groupid, subject)

	var id uint64
	db.Raw("SELECT id FROM admins WHERE createdby = ? AND subject = ? ORDER BY id DESC LIMIT 1",
		createdby, subject).Scan(&id)
	assert.Greater(t, id, uint64(0))
	return id
}

func TestListAdmins(t *testing.T) {
	prefix := uniquePrefix("adm_list")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	adminID := createTestAdmin(t, modID, groupID, "Test Admin "+prefix)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/modtools/admin?jwt=%s", modToken), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result []map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.GreaterOrEqual(t, len(result), 1)

	// Verify our admin is in the list.
	found := false
	for _, a := range result {
		if a["id"] == float64(adminID) {
			found = true
			break
		}
	}
	assert.True(t, found, "Created admin should be in the list")

	// Cleanup
	db := database.DBConn
	db.Exec("DELETE FROM admins WHERE id = ?", adminID)
}

func TestListAdminsNotMod(t *testing.T) {
	prefix := uniquePrefix("adm_listnm")
	userID := CreateTestUser(t, prefix, "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	createTestAdmin(t, userID, groupID, "Admin "+prefix)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/modtools/admin?jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result []map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	// Non-mod gets empty list (the INNER JOIN on memberships with mod role filters them out).
	assert.Equal(t, 0, len(result))

	// Cleanup
	db := database.DBConn
	db.Exec("DELETE FROM admins WHERE subject = ?", "Admin "+prefix)
}

func TestListAdminsSystemAdmin(t *testing.T) {
	prefix := uniquePrefix("adm_sysadm")
	// System Admin user with no group membership should see all admins.
	adminUserID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, adminToken := CreateTestSession(t, adminUserID)

	// Create a group and admin that the system admin is NOT a member of.
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")
	adminID := createTestAdmin(t, modID, groupID, "SysAdmin Test "+prefix)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/modtools/admin?jwt=%s", adminToken), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result []map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)

	// System admin should see admins from any group.
	found := false
	for _, a := range result {
		if a["id"] == float64(adminID) {
			found = true
			break
		}
	}
	assert.True(t, found, "System admin should see admins from any group")

	// Cleanup
	db := database.DBConn
	db.Exec("DELETE FROM admins WHERE id = ?", adminID)
}

func TestCreateAdmin(t *testing.T) {
	prefix := uniquePrefix("adm_create")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	body := fmt.Sprintf(`{"groupid":%d,"subject":"Test Subject %s","text":"Test text"}`, groupID, prefix)
	req := httptest.NewRequest("POST", "/api/modtools/admin?jwt="+modToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Greater(t, result["id"], float64(0))

	// Cleanup
	db := database.DBConn
	db.Exec("DELETE FROM admins WHERE id = ?", int(result["id"].(float64)))
}

func TestCreateAdminUnauthorized(t *testing.T) {
	body := `{"groupid":1,"subject":"Test","text":"Test"}`
	req := httptest.NewRequest("POST", "/api/modtools/admin", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestGetAdmin(t *testing.T) {
	prefix := uniquePrefix("adm_get")
	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	adminID := createTestAdmin(t, modID, groupID, "Get Test "+prefix)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/modtools/admin/%d?jwt=%s", adminID, modToken), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(adminID), result["id"])
	assert.Equal(t, "Get Test "+prefix, result["subject"])

	// Cleanup
	db := database.DBConn
	db.Exec("DELETE FROM admins WHERE id = ?", adminID)
}

func TestUpdateAdmin(t *testing.T) {
	prefix := uniquePrefix("adm_upd")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	adminID := createTestAdmin(t, modID, groupID, "Update Test "+prefix)

	body := fmt.Sprintf(`{"id":%d,"subject":"Updated Subject %s","text":"Updated text"}`, adminID, prefix)
	req := httptest.NewRequest("PATCH", "/api/modtools/admin?jwt="+modToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, true, result["success"])

	// Verify update.
	db := database.DBConn
	var subject string
	db.Raw("SELECT subject FROM admins WHERE id = ?", adminID).Scan(&subject)
	assert.Equal(t, "Updated Subject "+prefix, subject)

	// Verify edit tracking: editedat should be set, editedby should be the mod.
	var editedat *time.Time
	db.Raw("SELECT editedat FROM admins WHERE id = ?", adminID).Scan(&editedat)
	assert.NotNil(t, editedat, "editedat should be set after PATCH")

	var editedby *uint64
	db.Raw("SELECT editedby FROM admins WHERE id = ?", adminID).Scan(&editedby)
	assert.NotNil(t, editedby, "editedby should be set after PATCH")
	assert.Equal(t, modID, *editedby, "editedby should equal the mod's user ID")

	// Cleanup
	db.Exec("DELETE FROM admins WHERE id = ?", adminID)
}

func TestDeleteAdmin(t *testing.T) {
	prefix := uniquePrefix("adm_del")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	adminID := createTestAdmin(t, modID, groupID, "Delete Test "+prefix)

	body := fmt.Sprintf(`{"id":%d}`, adminID)
	req := httptest.NewRequest("DELETE", "/api/modtools/admin?jwt="+modToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, true, result["success"])

	// Verify deleted.
	db := database.DBConn
	var count int64
	db.Raw("SELECT COUNT(*) FROM admins WHERE id = ?", adminID).Scan(&count)
	assert.Equal(t, int64(0), count)
}

func TestHoldAdmin(t *testing.T) {
	prefix := uniquePrefix("adm_hold")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	adminID := createTestAdmin(t, modID, groupID, "Hold Test "+prefix)

	body := fmt.Sprintf(`{"id":%d,"action":"Hold"}`, adminID)
	req := httptest.NewRequest("POST", "/api/modtools/admin?jwt="+modToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify heldby is set.
	db := database.DBConn
	var heldby *uint64
	db.Raw("SELECT heldby FROM admins WHERE id = ?", adminID).Scan(&heldby)
	assert.NotNil(t, heldby)
	assert.Equal(t, modID, *heldby)

	// Cleanup
	db.Exec("DELETE FROM admins WHERE id = ?", adminID)
}

func TestReleaseAdmin(t *testing.T) {
	prefix := uniquePrefix("adm_rel")
	modID := CreateTestUser(t, prefix+"_mod", "User")
	groupID := CreateTestGroup(t, prefix)
	CreateTestMembership(t, modID, groupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	adminID := createTestAdmin(t, modID, groupID, "Release Test "+prefix)

	// Hold first.
	db := database.DBConn
	db.Exec("UPDATE admins SET heldby = ? WHERE id = ?", modID, adminID)

	// Release.
	body := fmt.Sprintf(`{"id":%d,"action":"Release"}`, adminID)
	req := httptest.NewRequest("POST", "/api/modtools/admin?jwt="+modToken, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify heldby is cleared.
	var heldby *uint64
	db.Raw("SELECT heldby FROM admins WHERE id = ?", adminID).Scan(&heldby)
	assert.Nil(t, heldby)

	// Cleanup
	db.Exec("DELETE FROM admins WHERE id = ?", adminID)
}

func TestAdminsOnlyForActiveModGroups(t *testing.T) {
	// V1 parity: admins listing should only show admins for groups where
	// the user is an active moderator (settings.active != 0).
	prefix := uniquePrefix("AdminActive")
	db := database.DBConn

	activeGroupID := CreateTestGroup(t, prefix+"_active")
	inactiveGroupID := CreateTestGroup(t, prefix+"_inactive")

	modID := CreateTestUser(t, prefix+"_mod", "Moderator")
	CreateTestMembership(t, modID, activeGroupID, "Moderator")
	CreateTestMembership(t, modID, inactiveGroupID, "Moderator")
	_, modToken := CreateTestSession(t, modID)

	// Mark the mod as inactive on one group.
	db.Exec("UPDATE memberships SET settings = ? WHERE userid = ? AND groupid = ?",
		`{"active":0}`, modID, inactiveGroupID)

	// Create admins for both groups.
	db.Exec("INSERT INTO admins (groupid, subject, text, createdby, pending) VALUES (?, ?, ?, ?, 0)",
		activeGroupID, prefix+"_active_admin", "text", modID)
	var activeAdminID uint64
	db.Raw("SELECT id FROM admins WHERE groupid = ? AND subject = ? ORDER BY id DESC LIMIT 1",
		activeGroupID, prefix+"_active_admin").Scan(&activeAdminID)

	db.Exec("INSERT INTO admins (groupid, subject, text, createdby, pending) VALUES (?, ?, ?, ?, 0)",
		inactiveGroupID, prefix+"_inactive_admin", "text", modID)
	var inactiveAdminID uint64
	db.Raw("SELECT id FROM admins WHERE groupid = ? AND subject = ? ORDER BY id DESC LIMIT 1",
		inactiveGroupID, prefix+"_inactive_admin").Scan(&inactiveAdminID)

	// Listing should only include the active group's admin.
	req := httptest.NewRequest("GET", "/api/modtools/admin?jwt="+modToken, nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var admins []map[string]interface{}
	json2.Unmarshal(rsp(resp), &admins)

	foundActive := false
	foundInactive := false
	for _, a := range admins {
		id := uint64(a["id"].(float64))
		if id == activeAdminID {
			foundActive = true
		}
		if id == inactiveAdminID {
			foundInactive = true
		}
	}
	assert.True(t, foundActive, "Should see admin for active group")
	assert.False(t, foundInactive, "Should NOT see admin for inactive group")

	// Cleanup.
	db.Exec("DELETE FROM admins WHERE id IN (?, ?)", activeAdminID, inactiveAdminID)
}
