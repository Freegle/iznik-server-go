package test

import (
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/communityevent"
	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestCommunityEvent(t *testing.T) {
	// Get logged out.
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/communityevents/1", nil))
	assert.Equal(t, 404, resp.StatusCode)

	var id []uint64

	db := database.DBConn

	db.Raw("SELECT communityevents.id FROM communityevents "+
		"INNER JOIN communityevents_dates ON communityevents_dates.eventid = communityevents.id "+
		"WHERE pending = 0 AND deleted = 0 AND heldby IS NULL "+
		"ORDER BY id DESC LIMIT 1").Pluck("id", &id)
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/communityevent/"+fmt.Sprint(id[0]), nil))
	assert.Equal(t, 200, resp.StatusCode)

	var communityevent communityevent.CommunityEvent
	json2.Unmarshal(rsp(resp), &communityevent)
	assert.Greater(t, communityevent.ID, uint64(0))
	assert.Greater(t, len(communityevent.Title), 0)
	assert.Greater(t, len(communityevent.Dates), 0)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/communityevent", nil))
	assert.Equal(t, 401, resp.StatusCode)

	_, token := GetUserWithToken(t)
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/communityevent?jwt="+token, nil))
	assert.Equal(t, 200, resp.StatusCode)

	var ids []uint64
	json2.Unmarshal(rsp(resp), &ids)
	assert.Greater(t, len(ids), 0)

	_, token = GetUserWithToken(t)
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/communityevent/group/"+fmt.Sprint(communityevent.Groups[0]), nil))
	assert.Equal(t, 200, resp.StatusCode)

	json2.Unmarshal(rsp(resp), &ids)
	assert.Greater(t, len(ids), 0)
}
