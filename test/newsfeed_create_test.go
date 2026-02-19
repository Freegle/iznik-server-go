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

	// Set group location and name.
	db.Exec("UPDATE `groups` SET lat = 52.2, lng = -0.1, nameshort = ? WHERE id = ?", prefix+"_group", groupID)

	// Clear any existing newsfeed entries for this user to avoid duplicate protection interference.
	db.Exec("DELETE FROM newsfeed WHERE userid = ?", userID)

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
		Type     string  `json:"type"`
		Userid   uint64  `json:"userid"`
		Groupid  uint64  `json:"groupid"`
		Eventid  *uint64 `json:"eventid"`
		Location *string `json:"location"`
		Hidden   *string `json:"hidden"`
	}
	var entry NfEntry
	db.Raw("SELECT type, userid, groupid, eventid, location, hidden FROM newsfeed WHERE id = ?", nfID).Scan(&entry)

	assert.Equal(t, newsfeed.TypeCommunityEvent, entry.Type)
	assert.Equal(t, userID, entry.Userid)
	assert.Equal(t, groupID, entry.Groupid)
	assert.NotNil(t, entry.Eventid)
	assert.Equal(t, eventID, *entry.Eventid)

	// Verify location was set from group name.
	assert.NotNil(t, entry.Location)
	assert.Equal(t, prefix+"_group", *entry.Location)

	// Verify hidden is NULL for non-suppressed user.
	assert.Nil(t, entry.Hidden)

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

	// Clear any existing newsfeed entries for this user to avoid duplicate protection interference.
	db.Exec("DELETE FROM newsfeed WHERE userid = ?", userID)

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
	db.Exec("UPDATE users SET lastlocation = NULL WHERE id = ?", userID)

	// Create a real community event (needed for FK constraint).
	db.Exec("INSERT INTO communityevents (userid, title, description, location, pending, deleted) VALUES (?, ?, 'Test', 'Test', 0, 0)",
		userID, "NoLoc Event "+prefix)
	var eventID uint64
	db.Raw("SELECT id FROM communityevents WHERE userid = ? AND title = ? ORDER BY id DESC LIMIT 1",
		userID, "NoLoc Event "+prefix).Scan(&eventID)
	assert.NotZero(t, eventID)

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

	// Clean up.
	db.Exec("DELETE FROM communityevents WHERE id = ?", eventID)
}

func TestCreateNewsfeedEntrySuppressedUser(t *testing.T) {
	prefix := uniquePrefix("nfcr_supp")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_user", "User")
	groupID := CreateTestGroup(t, prefix+"_group")
	db.Exec("UPDATE `groups` SET lat = 51.5, lng = -0.1 WHERE id = ?", groupID)

	// Mark user as suppressed.
	db.Exec("UPDATE users SET newsfeedmodstatus = 'Suppressed' WHERE id = ?", userID)

	// Clear any existing newsfeed entries.
	db.Exec("DELETE FROM newsfeed WHERE userid = ?", userID)

	// Create a real community event for FK constraint.
	db.Exec("INSERT INTO communityevents (userid, title, description, location, pending, deleted) VALUES (?, ?, 'Test', 'Test', 0, 0)",
		userID, "Suppressed Event "+prefix)
	var eventID uint64
	db.Raw("SELECT id FROM communityevents WHERE userid = ? AND title = ? ORDER BY id DESC LIMIT 1",
		userID, "Suppressed Event "+prefix).Scan(&eventID)
	assert.NotZero(t, eventID)

	nfID, err := newsfeed.CreateNewsfeedEntry(
		newsfeed.TypeCommunityEvent,
		userID,
		groupID,
		&eventID,
		nil,
	)

	assert.NoError(t, err)
	assert.NotZero(t, nfID)

	// Verify hidden is set (not NULL).
	var hidden *string
	db.Raw("SELECT hidden FROM newsfeed WHERE id = ?", nfID).Scan(&hidden)
	assert.NotNil(t, hidden, "Suppressed user's newsfeed entry should have hidden set")

	// Clean up.
	db.Exec("DELETE FROM newsfeed WHERE id = ?", nfID)
	db.Exec("DELETE FROM communityevents WHERE id = ?", eventID)
}

func TestCreateNewsfeedEntrySpammerUser(t *testing.T) {
	prefix := uniquePrefix("nfcr_spam")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_user", "User")
	groupID := CreateTestGroup(t, prefix+"_group")
	db.Exec("UPDATE `groups` SET lat = 51.5, lng = -0.1 WHERE id = ?", groupID)

	// Add user to spam list.
	db.Exec("INSERT INTO spam_users (userid, collection, reason) VALUES (?, 'Spammer', 'Test spammer')", userID)

	// Clear any existing newsfeed entries.
	db.Exec("DELETE FROM newsfeed WHERE userid = ?", userID)

	// Create a real community event for FK constraint.
	db.Exec("INSERT INTO communityevents (userid, title, description, location, pending, deleted) VALUES (?, ?, 'Test', 'Test', 0, 0)",
		userID, "Spam Event "+prefix)
	var eventID uint64
	db.Raw("SELECT id FROM communityevents WHERE userid = ? AND title = ? ORDER BY id DESC LIMIT 1",
		userID, "Spam Event "+prefix).Scan(&eventID)
	assert.NotZero(t, eventID)

	nfID, err := newsfeed.CreateNewsfeedEntry(
		newsfeed.TypeCommunityEvent,
		userID,
		groupID,
		&eventID,
		nil,
	)

	assert.NoError(t, err)
	assert.NotZero(t, nfID)

	// Verify hidden is set (not NULL).
	var hidden *string
	db.Raw("SELECT hidden FROM newsfeed WHERE id = ?", nfID).Scan(&hidden)
	assert.NotNil(t, hidden, "Spammer user's newsfeed entry should have hidden set")

	// Clean up.
	db.Exec("DELETE FROM newsfeed WHERE id = ?", nfID)
	db.Exec("DELETE FROM communityevents WHERE id = ?", eventID)
	db.Exec("DELETE FROM spam_users WHERE userid = ?", userID)
}

func TestCreateNewsfeedEntryDuplicateProtection(t *testing.T) {
	prefix := uniquePrefix("nfcr_dup")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_user", "User")
	groupID := CreateTestGroup(t, prefix+"_group")
	db.Exec("UPDATE `groups` SET lat = 51.5, lng = -0.1 WHERE id = ?", groupID)

	// Clear any existing newsfeed entries.
	db.Exec("DELETE FROM newsfeed WHERE userid = ?", userID)

	// Create a real community event for FK constraint.
	db.Exec("INSERT INTO communityevents (userid, title, description, location, pending, deleted) VALUES (?, ?, 'Test', 'Test', 0, 0)",
		userID, "Dup Event "+prefix)
	var eventID uint64
	db.Raw("SELECT id FROM communityevents WHERE userid = ? AND title = ? ORDER BY id DESC LIMIT 1",
		userID, "Dup Event "+prefix).Scan(&eventID)
	assert.NotZero(t, eventID)

	// Create a real volunteering opportunity for FK constraint.
	db.Exec("INSERT INTO volunteering (userid, title, description, location, pending, expired, deleted) VALUES (?, ?, 'Test', 'Test', 0, 0, 0)",
		userID, "Dup Vol "+prefix)
	var volID uint64
	db.Raw("SELECT id FROM volunteering WHERE userid = ? AND title = ? ORDER BY id DESC LIMIT 1",
		userID, "Dup Vol "+prefix).Scan(&volID)
	assert.NotZero(t, volID)

	// First call should succeed.
	nfID1, err := newsfeed.CreateNewsfeedEntry(
		newsfeed.TypeCommunityEvent,
		userID,
		groupID,
		&eventID,
		nil,
	)
	assert.NoError(t, err)
	assert.NotZero(t, nfID1)

	// Second call with same type should be skipped (duplicate protection).
	nfID2, err := newsfeed.CreateNewsfeedEntry(
		newsfeed.TypeCommunityEvent,
		userID,
		groupID,
		&eventID,
		nil,
	)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), nfID2, "Duplicate entry should be skipped")

	// Third call with DIFFERENT type should succeed.
	nfID3, err := newsfeed.CreateNewsfeedEntry(
		newsfeed.TypeVolunteerOpportunity,
		userID,
		groupID,
		nil,
		&volID,
	)
	assert.NoError(t, err)
	assert.NotZero(t, nfID3, "Different type should not be considered duplicate")

	// Clean up.
	db.Exec("DELETE FROM newsfeed WHERE userid = ?", userID)
	db.Exec("DELETE FROM communityevents WHERE id = ?", eventID)
	db.Exec("DELETE FROM volunteering WHERE id = ?", volID)
}
