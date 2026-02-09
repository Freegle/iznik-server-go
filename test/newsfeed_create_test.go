package test

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/newsfeed"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCreateNewsfeedEntryForCommunityEvent(t *testing.T) {
	prefix := uniquePrefix("nfcr_event")
	db := database.DBConn

	// Create a test user and group with known locations.
	userID := CreateTestUser(t, prefix+"_user", "User")
	groupID := CreateTestGroup(t, prefix+"_group")

	// Set group location.
	db.Exec("UPDATE `groups` SET lat = 52.2, lng = -0.1 WHERE id = ?", groupID)

	// Create a test community event.
	db.Exec("INSERT INTO communityevents (userid, title, description, location, pending, deleted) VALUES (?, ?, 'Test event', 'Test location', 0, 0)",
		userID, "Test Event "+prefix)
	var eventID uint64
	db.Raw("SELECT id FROM communityevents WHERE userid = ? AND title = ? ORDER BY id DESC LIMIT 1",
		userID, "Test Event "+prefix).Scan(&eventID)
	assert.NotZero(t, eventID)

	// Create newsfeed entry.
	nfID, err := newsfeed.CreateNewsfeedEntry(
		newsfeed.TypeCommunityEvent,
		userID,
		groupID,
		&eventID,
		nil,
	)

	assert.NoError(t, err)
	assert.NotZero(t, nfID)

	// Verify the newsfeed entry.
	type NfEntry struct {
		Type    string  `json:"type"`
		Userid  uint64  `json:"userid"`
		Groupid uint64  `json:"groupid"`
		Eventid *uint64 `json:"eventid"`
	}
	var entry NfEntry
	db.Raw("SELECT type, userid, groupid, eventid FROM newsfeed WHERE id = ?", nfID).Scan(&entry)

	assert.Equal(t, newsfeed.TypeCommunityEvent, entry.Type)
	assert.Equal(t, userID, entry.Userid)
	assert.Equal(t, groupID, entry.Groupid)
	assert.NotNil(t, entry.Eventid)
	assert.Equal(t, eventID, *entry.Eventid)

	// Clean up.
	db.Exec("DELETE FROM newsfeed WHERE id = ?", nfID)
	db.Exec("DELETE FROM communityevents WHERE id = ?", eventID)
}

func TestCreateNewsfeedEntryForVolunteering(t *testing.T) {
	prefix := uniquePrefix("nfcr_vol")
	db := database.DBConn

	// Create a test user and group with known locations.
	userID := CreateTestUser(t, prefix+"_user", "User")
	groupID := CreateTestGroup(t, prefix+"_group")

	// Set group location.
	db.Exec("UPDATE `groups` SET lat = 53.5, lng = -1.5 WHERE id = ?", groupID)

	// Create a test volunteering opportunity.
	db.Exec("INSERT INTO volunteering (userid, title, description, location, pending, expired, deleted) VALUES (?, ?, 'Help needed', 'Test location', 0, 0, 0)",
		userID, "Test Volunteering "+prefix)
	var volID uint64
	db.Raw("SELECT id FROM volunteering WHERE userid = ? AND title = ? ORDER BY id DESC LIMIT 1",
		userID, "Test Volunteering "+prefix).Scan(&volID)
	assert.NotZero(t, volID)

	// Create newsfeed entry.
	nfID, err := newsfeed.CreateNewsfeedEntry(
		newsfeed.TypeVolunteerOpportunity,
		userID,
		groupID,
		nil,
		&volID,
	)

	assert.NoError(t, err)
	assert.NotZero(t, nfID)

	// Verify the newsfeed entry.
	type NfEntry struct {
		Type           string  `json:"type"`
		Userid         uint64  `json:"userid"`
		Groupid        uint64  `json:"groupid"`
		Volunteeringid *uint64 `json:"volunteeringid"`
	}
	var entry NfEntry
	db.Raw("SELECT type, userid, groupid, volunteeringid FROM newsfeed WHERE id = ?", nfID).Scan(&entry)

	assert.Equal(t, newsfeed.TypeVolunteerOpportunity, entry.Type)
	assert.Equal(t, userID, entry.Userid)
	assert.Equal(t, groupID, entry.Groupid)
	assert.NotNil(t, entry.Volunteeringid)
	assert.Equal(t, volID, *entry.Volunteeringid)

	// Clean up.
	db.Exec("DELETE FROM newsfeed WHERE id = ?", nfID)
	db.Exec("DELETE FROM volunteering WHERE id = ?", volID)
}

func TestCreateNewsfeedEntryNoLocation(t *testing.T) {
	prefix := uniquePrefix("nfcr_noloc")
	db := database.DBConn

	// Create user and group with NO location.
	userID := CreateTestUser(t, prefix+"_user", "User")
	groupID := CreateTestGroup(t, prefix+"_group")

	// Ensure no location is set.
	db.Exec("UPDATE `groups` SET lat = NULL, lng = NULL WHERE id = ?", groupID)
	db.Exec("UPDATE users SET lat = NULL, lng = NULL WHERE id = ?", userID)

	var eventID uint64 = 999999
	nfID, err := newsfeed.CreateNewsfeedEntry(
		newsfeed.TypeCommunityEvent,
		userID,
		groupID,
		&eventID,
		nil,
	)

	// Should return 0 without error - can't create without location.
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), nfID)
}
