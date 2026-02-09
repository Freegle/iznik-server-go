package test

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

func TestCreateImageAttachment(t *testing.T) {
	prefix := uniquePrefix("CreateImage")
	userID := CreateTestUser(t, prefix, "Member")
	_, token := CreateTestSession(t, userID)

	body := `{"externaluid":"freegletusd-test-abc123","imgtype":"Message","externalmods":{"rotate":90}}`
	req := httptest.NewRequest("POST", "/api/image", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	respBody := rsp(resp)
	var result map[string]interface{}
	json.Unmarshal(respBody, &result)
	assert.NotZero(t, result["id"])
	assert.Equal(t, "freegletusd-test-abc123", result["uid"])
	assert.Equal(t, float64(0), result["ret"])
}

func TestCreateImageAttachmentWithParent(t *testing.T) {
	prefix := uniquePrefix("CreateImageParent")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "Member")
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	// Create a community event to use as parent.
	db := database.DBConn
	db.Exec("INSERT INTO communityevents (userid, title, description, location, pending, deleted) VALUES (?, ?, 'Test event', 'Test loc', 0, 0)",
		userID, "ImageTest "+prefix)
	var eventID uint64
	db.Raw("SELECT id FROM communityevents WHERE userid = ? AND title = ? ORDER BY id DESC LIMIT 1",
		userID, "ImageTest "+prefix).Scan(&eventID)
	assert.NotZero(t, eventID)

	body := fmt.Sprintf(`{"externaluid":"freegletusd-event-%s","imgtype":"CommunityEvent","communityevent":%d}`, prefix, eventID)
	req := httptest.NewRequest("POST", "/api/image", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	respBody := rsp(resp)
	var result map[string]interface{}
	json.Unmarshal(respBody, &result)
	assert.NotZero(t, result["id"])
	assert.Equal(t, fmt.Sprintf("freegletusd-event-%s", prefix), result["uid"])
}

func TestCreateImageUnauthorized(t *testing.T) {
	body := `{"externaluid":"freegletusd-test-noauth","imgtype":"Message"}`
	req := httptest.NewRequest("POST", "/api/image", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestCreateImageMissingUID(t *testing.T) {
	prefix := uniquePrefix("CreateImageNoUID")
	userID := CreateTestUser(t, prefix, "Member")
	_, token := CreateTestSession(t, userID)

	body := `{"imgtype":"Message"}`
	req := httptest.NewRequest("POST", "/api/image", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestCreateImageInvalidType(t *testing.T) {
	prefix := uniquePrefix("CreateImageBadType")
	userID := CreateTestUser(t, prefix, "Member")
	_, token := CreateTestSession(t, userID)

	body := `{"externaluid":"freegletusd-test-badtype","imgtype":"InvalidType"}`
	req := httptest.NewRequest("POST", "/api/image", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestCreateImageDefaultType(t *testing.T) {
	prefix := uniquePrefix("CreateImageDefault")
	userID := CreateTestUser(t, prefix, "Member")
	_, token := CreateTestSession(t, userID)

	// No imgtype - should default to Message.
	body := `{"externaluid":"freegletusd-test-default"}`
	req := httptest.NewRequest("POST", "/api/image", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	respBody := rsp(resp)
	var result map[string]interface{}
	json.Unmarshal(respBody, &result)
	assert.NotZero(t, result["id"])
}

func TestRotateImage(t *testing.T) {
	prefix := uniquePrefix("RotateImage")
	userID := CreateTestUser(t, prefix, "Member")
	_, token := CreateTestSession(t, userID)

	// First create an image.
	createBody := `{"externaluid":"freegletusd-test-rotate","imgtype":"Message"}`
	createReq := httptest.NewRequest("POST", "/api/image", strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+token)

	createResp, _ := getApp().Test(createReq)
	assert.Equal(t, fiber.StatusOK, createResp.StatusCode)

	createRespBody := rsp(createResp)
	var createResult map[string]interface{}
	json.Unmarshal(createRespBody, &createResult)
	imageID := createResult["id"].(float64)
	assert.NotZero(t, imageID)

	// Now rotate it using the same POST endpoint.
	rotateBody := fmt.Sprintf(`{"id":%d,"rotate":90,"type":"Message"}`, int(imageID))
	rotateReq := httptest.NewRequest("POST", "/api/image", strings.NewReader(rotateBody))
	rotateReq.Header.Set("Content-Type", "application/json")
	rotateReq.Header.Set("Authorization", "Bearer "+token)

	rotateResp, _ := getApp().Test(rotateReq)
	assert.Equal(t, fiber.StatusOK, rotateResp.StatusCode)

	rotateRespBody := rsp(rotateResp)
	var rotateResult map[string]interface{}
	json.Unmarshal(rotateRespBody, &rotateResult)
	assert.Equal(t, float64(0), rotateResult["ret"])
}

func TestRotateImageWithBooleanFlag(t *testing.T) {
	prefix := uniquePrefix("RotateBoolFlag")
	userID := CreateTestUser(t, prefix, "Member")
	_, token := CreateTestSession(t, userID)

	// Create a community event image.
	db := database.DBConn
	db.Exec("INSERT INTO communityevents (userid, title, description, location, pending, deleted) VALUES (?, ?, 'Test', 'Test', 0, 0)",
		userID, "RotateTest "+prefix)
	var eventID uint64
	db.Raw("SELECT id FROM communityevents WHERE userid = ? AND title = ? ORDER BY id DESC LIMIT 1",
		userID, "RotateTest "+prefix).Scan(&eventID)
	assert.NotZero(t, eventID)

	createBody := fmt.Sprintf(`{"externaluid":"freegletusd-rotate-bool-%s","imgtype":"CommunityEvent","communityevent":%d}`, prefix, eventID)
	createReq := httptest.NewRequest("POST", "/api/image", strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+token)

	createResp, _ := getApp().Test(createReq)
	assert.Equal(t, fiber.StatusOK, createResp.StatusCode)

	createRespBody := rsp(createResp)
	var createResult map[string]interface{}
	json.Unmarshal(createRespBody, &createResult)
	imageID := createResult["id"].(float64)

	// Rotate using boolean flag (how the client actually sends it).
	rotateBody := fmt.Sprintf(`{"id":%d,"rotate":90,"communityevent":true}`, int(imageID))
	rotateReq := httptest.NewRequest("POST", "/api/image", strings.NewReader(rotateBody))
	rotateReq.Header.Set("Content-Type", "application/json")
	rotateReq.Header.Set("Authorization", "Bearer "+token)

	rotateResp, _ := getApp().Test(rotateReq)
	assert.Equal(t, fiber.StatusOK, rotateResp.StatusCode)
}

func TestRotateImageUnauthorized(t *testing.T) {
	body := `{"id":1,"rotate":90,"type":"Message"}`
	req := httptest.NewRequest("POST", "/api/image", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}
