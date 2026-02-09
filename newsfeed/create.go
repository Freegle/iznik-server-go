package newsfeed

import (
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/utils"
	"log"
)

// Newsfeed type constants.
const (
	TypeCommunityEvent      = "CommunityEvent"
	TypeVolunteerOpportunity = "VolunteerOpportunity"
)

// CreateNewsfeedEntry creates a newsfeed entry for side effects like addGroup.
//
// When a community event or volunteering opportunity is added to a group, a
// newsfeed entry is created so nearby users see it. Position is derived from
// the user's lat/lng, falling back to the group's lat/lng.
//
// Behaviour:
// - Checks spam/suppression status (sets hidden=NOW() for suppressed/spammer users)
// - Duplicate protection (skips if last entry by user has same type)
// - Sets location display name from the group
func CreateNewsfeedEntry(nfType string, userid uint64, groupid uint64, eventid *uint64, volunteeringid *uint64) (uint64, error) {
	db := database.DBConn

	// Get position: try user location first, fall back to group.
	var lat, lng *float64

	// Try user location first (via lastlocation FK to locations table).
	if userid > 0 {
		type UserLoc struct {
			Lat *float64
			Lng *float64
		}
		var ul UserLoc
		db.Raw("SELECT l.lat, l.lng FROM users u LEFT JOIN locations l ON l.id = u.lastlocation WHERE u.id = ?", userid).Scan(&ul)
		lat = ul.Lat
		lng = ul.Lng
	}

	// Fall back to group location.
	if lat == nil && groupid > 0 {
		type GroupLoc struct {
			Lat *float64
			Lng *float64
		}
		var gl GroupLoc
		db.Raw("SELECT lat, lng FROM `groups` WHERE id = ?", groupid).Scan(&gl)
		lat = gl.Lat
		lng = gl.Lng
	}

	if lat == nil || lng == nil {
		// Can't create a newsfeed entry without a location.
		return 0, nil
	}

	// If user is suppressed or a known spammer, set hidden=NOW() so only they can see it.
	hidden := "NULL"
	if userid > 0 {
		var modStatus string
		db.Raw("SELECT COALESCE(newsfeedmodstatus, 'Unmoderated') FROM users WHERE id = ?", userid).Scan(&modStatus)

		var spamCount int64
		db.Raw("SELECT COUNT(*) FROM spam_users WHERE userid = ? AND collection = 'Spammer'", userid).Scan(&spamCount)

		if modStatus == "Suppressed" || spamCount > 0 {
			hidden = "NOW()"
		}
	}

	// Duplicate protection: skip if last entry by this user was the same type.
	if userid > 0 {
		type LastEntry struct {
			Type *string
		}
		var last LastEntry
		db.Raw("SELECT `type` FROM newsfeed WHERE userid = ? ORDER BY id DESC LIMIT 1", userid).Scan(&last)

		if last.Type != nil && *last.Type == nfType {
			// Last entry by this user was the same type - skip to prevent duplicate.
			return 0, nil
		}
	}

	// Set location display name from the group.
	var location *string
	if groupid > 0 {
		var groupName string
		db.Raw("SELECT nameshort FROM `groups` WHERE id = ?", groupid).Scan(&groupName)
		if groupName != "" {
			location = &groupName
		}
	}

	pos := fmt.Sprintf("ST_GeomFromText('POINT(%f %f)', %d)", *lng, *lat, utils.SRID)

	result := db.Exec(
		fmt.Sprintf("INSERT INTO newsfeed (`type`, userid, groupid, eventid, volunteeringid, position, location, hidden, deleted, reviewrequired, pinned) "+
			"VALUES (?, ?, ?, ?, ?, %s, ?, %s, NULL, 0, 0)", pos, hidden),
		nfType, userid, groupid, eventid, volunteeringid, location,
	)

	if result.Error != nil {
		log.Printf("Failed to create newsfeed entry: %v", result.Error)
		return 0, result.Error
	}

	// Get the inserted ID.
	var id uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&id)

	return id, nil
}
