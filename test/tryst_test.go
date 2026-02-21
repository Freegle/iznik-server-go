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

func TestCreateTryst(t *testing.T) {
	prefix := uniquePrefix("Tryst")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	_, token := CreateTestSession(t, user1ID)

	// CreateTryst requires a chat room between the users.
	CreateTestChatRoom(t, user1ID, &user2ID, nil, "User2User")

	body := fmt.Sprintf(`{"user1":%d,"user2":%d,"arrangedfor":"2038-01-19T03:14:06+00:00"}`, user1ID, user2ID)
	req := httptest.NewRequest("PUT", fmt.Sprintf("/api/tryst?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Greater(t, result["id"].(float64), float64(0))
}

func TestGetTrystList(t *testing.T) {
	prefix := uniquePrefix("TrystList")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	_, token := CreateTestSession(t, user1ID)

	db := database.DBConn
	db.Exec("INSERT INTO trysts (user1, user2, arrangedfor) VALUES (?, ?, '2038-01-19 03:14:06')",
		user1ID, user2ID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/tryst?jwt=%s", token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Contains(t, result, "trysts")
}

func TestGetTrystSingle(t *testing.T) {
	prefix := uniquePrefix("TrystSingle")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	_, token := CreateTestSession(t, user1ID)

	db := database.DBConn
	db.Exec("INSERT INTO trysts (user1, user2, arrangedfor) VALUES (?, ?, '2038-01-19 03:14:06')",
		user1ID, user2ID)

	var trystID uint64
	db.Raw("SELECT id FROM trysts WHERE user1 = ? ORDER BY id DESC LIMIT 1", user1ID).Scan(&trystID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/tryst?id=%d&jwt=%s", trystID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
	assert.Contains(t, result, "tryst")

	tryst := result["tryst"].(map[string]interface{})
	assert.Equal(t, float64(trystID), tryst["id"])
}

func TestPatchTryst(t *testing.T) {
	prefix := uniquePrefix("TrystPatch")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	_, token := CreateTestSession(t, user1ID)

	db := database.DBConn
	db.Exec("INSERT INTO trysts (user1, user2, arrangedfor) VALUES (?, ?, '2038-01-19 03:14:06')",
		user1ID, user2ID)

	var trystID uint64
	db.Raw("SELECT id FROM trysts WHERE user1 = ? ORDER BY id DESC LIMIT 1", user1ID).Scan(&trystID)

	body := fmt.Sprintf(`{"id":%d,"arrangedfor":"2038-01-20T10:00:00+00:00"}`, trystID)
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/tryst?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])
}

func TestConfirmTryst(t *testing.T) {
	prefix := uniquePrefix("TrystConf")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	_, token := CreateTestSession(t, user1ID)

	db := database.DBConn
	db.Exec("INSERT INTO trysts (user1, user2, arrangedfor) VALUES (?, ?, '2038-01-19 03:14:06')",
		user1ID, user2ID)

	var trystID uint64
	db.Raw("SELECT id FROM trysts WHERE user1 = ? ORDER BY id DESC LIMIT 1", user1ID).Scan(&trystID)

	body := fmt.Sprintf(`{"id":%d,"confirm":true}`, trystID)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/tryst?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	// Verify user1confirmed is set.
	var confirmed *string
	db.Raw("SELECT user1confirmed FROM trysts WHERE id = ?", trystID).Scan(&confirmed)
	assert.NotNil(t, confirmed)
}

func TestDeclineTryst(t *testing.T) {
	prefix := uniquePrefix("TrystDecl")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	_, token := CreateTestSession(t, user2ID)

	db := database.DBConn
	db.Exec("INSERT INTO trysts (user1, user2, arrangedfor) VALUES (?, ?, '2038-01-19 03:14:06')",
		user1ID, user2ID)

	var trystID uint64
	db.Raw("SELECT id FROM trysts WHERE user1 = ? ORDER BY id DESC LIMIT 1", user1ID).Scan(&trystID)

	body := fmt.Sprintf(`{"id":%d,"decline":true}`, trystID)
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/tryst?jwt=%s", token), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	var declined *string
	db.Raw("SELECT user2declined FROM trysts WHERE id = ?", trystID).Scan(&declined)
	assert.NotNil(t, declined)
}

func TestDeleteTryst(t *testing.T) {
	prefix := uniquePrefix("TrystDel")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	_, token := CreateTestSession(t, user1ID)

	db := database.DBConn
	db.Exec("INSERT INTO trysts (user1, user2, arrangedfor) VALUES (?, ?, '2038-01-19 03:14:06')",
		user1ID, user2ID)

	var trystID uint64
	db.Raw("SELECT id FROM trysts WHERE user1 = ? ORDER BY id DESC LIMIT 1", user1ID).Scan(&trystID)

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/tryst?id=%d&jwt=%s", trystID, token), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json2.Unmarshal(rsp(resp), &result)
	assert.Equal(t, float64(0), result["ret"])

	var count int64
	db.Raw("SELECT COUNT(*) FROM trysts WHERE id = ?", trystID).Scan(&count)
	assert.Equal(t, int64(0), count)
}

func TestTrystPermissionDenied(t *testing.T) {
	prefix := uniquePrefix("TrystPerm")
	user1ID := CreateTestUser(t, prefix+"_u1", "User")
	user2ID := CreateTestUser(t, prefix+"_u2", "User")
	otherID := CreateTestUser(t, prefix+"_other", "User")
	_, otherToken := CreateTestSession(t, otherID)

	db := database.DBConn
	db.Exec("INSERT INTO trysts (user1, user2, arrangedfor) VALUES (?, ?, '2038-01-19 03:14:06')",
		user1ID, user2ID)

	var trystID uint64
	db.Raw("SELECT id FROM trysts WHERE user1 = ? ORDER BY id DESC LIMIT 1", user1ID).Scan(&trystID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/tryst?id=%d&jwt=%s", trystID, otherToken), nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestGetTrystV2Path(t *testing.T) {
	req := httptest.NewRequest("GET", "/apiv2/tryst", nil)
	resp, _ := getApp().Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}
