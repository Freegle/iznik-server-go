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
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "Member")
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	// Create a message to attach the image to (foreign key on messages_attachments.msgid).
	msgID := CreateTestMessage(t, userID, groupID, "Image test "+prefix, 55.9533, -3.1883)

	body := fmt.Sprintf(`{"externaluid":"freegletusd-test-%s","imgtype":"Message","msgid":%d,"externalmods":{"rotate":90}}`, prefix, msgID)
	req := httptest.NewRequest("POST", "/api/image?jwt="+token, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	respBody := rsp(resp)
	var result map[string]interface{}
	json.Unmarshal(respBody, &result)
	assert.NotZero(t, result["id"])
	assert.Equal(t, fmt.Sprintf("freegletusd-test-%s", prefix), result["uid"])
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
	req := httptest.NewRequest("POST", "/api/image?jwt="+token, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

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
	req := httptest.NewRequest("POST", "/api/image?jwt="+token, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestCreateImageInvalidType(t *testing.T) {
	prefix := uniquePrefix("CreateImageBadType")
	userID := CreateTestUser(t, prefix, "Member")
	_, token := CreateTestSession(t, userID)

	body := `{"externaluid":"freegletusd-test-badtype","imgtype":"InvalidType"}`
	req := httptest.NewRequest("POST", "/api/image?jwt="+token, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestCreateImageDefaultType(t *testing.T) {
	prefix := uniquePrefix("CreateImageDefault")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "Member")
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	// Create a message (default type is Message, so needs valid msgid).
	msgID := CreateTestMessage(t, userID, groupID, "Default type test "+prefix, 55.9533, -3.1883)

	// No imgtype - should default to Message.
	body := fmt.Sprintf(`{"externaluid":"freegletusd-test-default-%s","msgid":%d}`, prefix, msgID)
	req := httptest.NewRequest("POST", "/api/image?jwt="+token, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := getApp().Test(req)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	respBody := rsp(resp)
	var result map[string]interface{}
	json.Unmarshal(respBody, &result)
	assert.NotZero(t, result["id"])
}

func TestRotateImage(t *testing.T) {
	prefix := uniquePrefix("RotateImage")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "Member")
	CreateTestMembership(t, userID, groupID, "Member")
	_, token := CreateTestSession(t, userID)

	// Create a message for the image attachment.
	msgID := CreateTestMessage(t, userID, groupID, "Rotate test "+prefix, 55.9533, -3.1883)

	// First create an image.
	createBody := fmt.Sprintf(`{"externaluid":"freegletusd-test-rotate-%s","imgtype":"Message","msgid":%d}`, prefix, msgID)
	createReq := httptest.NewRequest("POST", "/api/image?jwt="+token, strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")

	createResp, _ := getApp().Test(createReq)
	assert.Equal(t, fiber.StatusOK, createResp.StatusCode)

	createRespBody := rsp(createResp)
	var createResult map[string]interface{}
	json.Unmarshal(createRespBody, &createResult)
	assert.NotNil(t, createResult["id"], "create response should include id")
	imageID := createResult["id"].(float64)
	assert.NotZero(t, imageID)

	// Now rotate it using the same POST endpoint.
	rotateBody := fmt.Sprintf(`{"id":%d,"rotate":90,"type":"Message"}`, int(imageID))
	rotateReq := httptest.NewRequest("POST", "/api/image?jwt="+token, strings.NewReader(rotateBody))
	rotateReq.Header.Set("Content-Type", "application/json")

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
	createReq := httptest.NewRequest("POST", "/api/image?jwt="+token, strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")

	createResp, _ := getApp().Test(createReq)
	assert.Equal(t, fiber.StatusOK, createResp.StatusCode)

	createRespBody := rsp(createResp)
	var createResult map[string]interface{}
	json.Unmarshal(createRespBody, &createResult)
	assert.NotNil(t, createResult["id"], "create response should include id")
	imageID := createResult["id"].(float64)

	// Rotate using boolean flag (how the client actually sends it).
	rotateBody := fmt.Sprintf(`{"id":%d,"rotate":90,"communityevent":true}`, int(imageID))
	rotateReq := httptest.NewRequest("POST", "/api/image?jwt="+token, strings.NewReader(rotateBody))
	rotateReq.Header.Set("Content-Type", "application/json")

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
