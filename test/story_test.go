package test

import (
	json2 "encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

func createTestStory(t *testing.T, userID uint64, groupID uint64) uint64 {
	db := database.DBConn

	result := db.Exec("INSERT INTO users_stories (userid, date, public, headline, story, reviewed) "+
		"VALUES (?, NOW(), 1, 'Test Headline', 'Test story text', 1)", userID)

	if result.Error != nil {
		t.Fatalf("ERROR: Failed to create story: %v", result.Error)
	}

	var storyID uint64
	db.Raw("SELECT id FROM users_stories WHERE userid = ? ORDER BY id DESC LIMIT 1", userID).Scan(&storyID)

	if storyID == 0 {
		t.Fatalf("ERROR: Story was created but ID not found")
	}

	// Link story to group
	db.Exec("INSERT INTO users_stories_groups (storyid, groupid) VALUES (?, ?)", storyID, groupID)

	return storyID
}

func TestStory(t *testing.T) {
	// Get non-existent story - should return 404
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/story/1", nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestStory_ValidStory(t *testing.T) {
	prefix := uniquePrefix("storyval")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")
	storyID := createTestStory(t, userID, groupID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/story/"+fmt.Sprint(storyID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	type StoryResponse struct {
		ID       uint64 `json:"id"`
		Headline string `json:"headline"`
	}
	var story StoryResponse
	json2.Unmarshal(rsp(resp), &story)
	assert.Equal(t, storyID, story.ID)
	assert.Equal(t, "Test Headline", story.Headline)
}

func TestStory_InvalidID(t *testing.T) {
	// Non-integer ID
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/story/notanint", nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestListStory(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/story", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var ids []uint64
	json2.Unmarshal(rsp(resp), &ids)
}

func TestGroupStory(t *testing.T) {
	// Group 0 - should return empty
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/story/group/0", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var ids []uint64
	json2.Unmarshal(rsp(resp), &ids)
}

func TestGroupStory_WithData(t *testing.T) {
	prefix := uniquePrefix("storygrp")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")
	createTestStory(t, userID, groupID)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/story/group/"+fmt.Sprint(groupID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var ids []uint64
	json2.Unmarshal(rsp(resp), &ids)
	assert.Greater(t, len(ids), 0)
}

func TestStory_V2Path(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/apiv2/story", nil))
	assert.Equal(t, 200, resp.StatusCode)
}
