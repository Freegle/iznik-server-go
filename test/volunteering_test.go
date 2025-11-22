package test

import (
	json2 "encoding/json"
	"fmt"
	volunteering2 "github.com/freegle/iznik-server-go/volunteering"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestVolunteering(t *testing.T) {
	// Create test data for this test
	prefix := uniquePrefix("vol")
	groupID := CreateTestGroup(t, prefix)
	userID := CreateTestUser(t, prefix, "User")
	CreateTestMembership(t, userID, groupID, "Member")
	volunteeringID := CreateTestVolunteering(t, userID, groupID)

	// Get non-existent volunteering - should return 404
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/volunteering/1", nil))
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
