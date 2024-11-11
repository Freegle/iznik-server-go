package test

import (
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/group"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestListGroups(t *testing.T) {
	// List groups
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/group", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var groups []group.GroupEntry
	json2.Unmarshal(rsp(resp), &groups)

	assert.Greater(t, len(groups), 1)
	assert.Greater(t, groups[0].ID, uint64(0))
	assert.Greater(t, len(groups[0].Nameshort), 0)
	assert.Equal(t, groups[0].Showjoin, 0)

	pg := GetGroup(getApp(), "FreeglePlayground")

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/group/"+fmt.Sprint(pg.ID), nil))
	assert.Equal(t, 200, resp.StatusCode)
	var group group.Group
	json2.Unmarshal(rsp(resp), &group)

	assert.Equal(t, group.Nameshort, pg.Nameshort)
	assert.Equal(t, group.Showjoin, 0)

	// Check that it has volunteers.
	assert.Greater(t, len(group.GroupVolunteers), 0)

	// Get the another group.
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/group/"+fmt.Sprint(groups[1].ID), nil))
	assert.Equal(t, 200, resp.StatusCode)
	json2.Unmarshal(rsp(resp), &group)

	assert.Equal(t, group.Nameshort, groups[1].Nameshort)

	// Get an invalid group.
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/group/1", nil))
	assert.Equal(t, 404, resp.StatusCode)
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/group/notanint", nil))
	assert.Equal(t, 404, resp.StatusCode)
}
