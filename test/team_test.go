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

func createTestTeam(t *testing.T, name string) uint64 {
	db := database.DBConn
	result := db.Exec("INSERT INTO teams (name, description, type) VALUES (?, 'Test team', 'Team')", name)
	assert.NoError(t, result.Error)

	var id uint64
	db.Raw("SELECT id FROM teams WHERE name = ?", name).Scan(&id)
	assert.Greater(t, id, uint64(0))
	return id
}

func TestGetTeamList(t *testing.T) {
	prefix := uniquePrefix("TeamList")
	createTestTeam(t, prefix+"_A")
	createTestTeam(t, prefix+"_B")

	req := httptest.NewRequest("GET", "/api/team", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Contains(t, result, "teams")

	teams := result["teams"].([]interface{})
	assert.GreaterOrEqual(t, len(teams), 2)
}

func TestGetTeamSingle(t *testing.T) {
	prefix := uniquePrefix("TeamSingle")
	teamID := createTestTeam(t, prefix+"_team")

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/team?id=%d", teamID), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Contains(t, result, "team")

	team := result["team"].(map[string]interface{})
	assert.Equal(t, float64(teamID), team["id"])
	assert.Contains(t, team, "members")
}

func TestGetTeamVolunteers(t *testing.T) {
	// Volunteers is a pseudo-team.
	req := httptest.NewRequest("GET", "/api/team?name=Volunteers", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Contains(t, result, "team")

	team := result["team"].(map[string]interface{})
	assert.Equal(t, "Volunteers", team["name"])
	assert.Contains(t, team, "members")
}

func TestPostTeam(t *testing.T) {
	prefix := uniquePrefix("TeamPost")
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, adminID)

	body := fmt.Sprintf(`{"name":"%s_newteam","description":"A new team"}`, prefix)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/team?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Greater(t, result["id"].(float64), float64(0))
}

func TestPostTeamNotAdmin(t *testing.T) {
	prefix := uniquePrefix("TeamPostNA")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	body := `{"name":"ShouldFail","description":"Nope"}`
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/team?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(2), result["ret"])
}

func TestPatchTeamAddMember(t *testing.T) {
	prefix := uniquePrefix("TeamPatch")
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, adminID)

	teamID := createTestTeam(t, prefix+"_team")
	memberID := CreateTestUser(t, prefix+"_member", "User")

	body := fmt.Sprintf(`{"id":%d,"action":"Add","userid":%d,"description":"Board member"}`, teamID, memberID)
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/team?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify member was added.
	db := database.DBConn
	var count int64
	db.Raw("SELECT COUNT(*) FROM teams_members WHERE teamid = ? AND userid = ?", teamID, memberID).Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestPatchTeamRemoveMember(t *testing.T) {
	prefix := uniquePrefix("TeamRemove")
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, adminID)

	teamID := createTestTeam(t, prefix+"_team")
	memberID := CreateTestUser(t, prefix+"_member", "User")

	db := database.DBConn
	db.Exec("INSERT INTO teams_members (userid, teamid) VALUES (?, ?)", memberID, teamID)

	body := fmt.Sprintf(`{"id":%d,"action":"Remove","userid":%d}`, teamID, memberID)
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/team?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	var count int64
	db.Raw("SELECT COUNT(*) FROM teams_members WHERE teamid = ? AND userid = ?", teamID, memberID).Scan(&count)
	assert.Equal(t, int64(0), count)
}

func TestDeleteTeam(t *testing.T) {
	prefix := uniquePrefix("TeamDel")
	adminID := CreateTestUser(t, prefix+"_admin", "Admin")
	_, token := CreateTestSession(t, adminID)

	teamID := createTestTeam(t, prefix+"_team")

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/team?id=%d&jwt=%s", teamID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	db := database.DBConn
	var count int64
	db.Raw("SELECT COUNT(*) FROM teams WHERE id = ?", teamID).Scan(&count)
	assert.Equal(t, int64(0), count)
}

func TestDeleteTeamNotAdmin(t *testing.T) {
	prefix := uniquePrefix("TeamDelNA")
	userID := CreateTestUser(t, prefix, "User")
	_, token := CreateTestSession(t, userID)

	teamID := createTestTeam(t, prefix+"_team")

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/team?id=%d&jwt=%s", teamID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(2), result["ret"])
}

func TestGetTeamV2Path(t *testing.T) {
	req := httptest.NewRequest("GET", "/apiv2/team", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)
}
