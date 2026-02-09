package newsfeed

import (
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/utils"
	"log"
)

// Newsfeed type constants matching PHP Newsfeed::TYPE_* values.
const (
	TypeCommunityEvent      = "CommunityEvent"
	TypeVolunteerOpportunity = "VolunteerOpportunity"
)

// CreateNewsfeedEntry creates a newsfeed entry for side effects like addGroup.
//
// For community events and volunteering, the PHP code creates a newsfeed entry
// when an item is added to a group. The position is derived from the group's
// lat/lng or the user's lat/lng.
func CreateNewsfeedEntry(nfType string, userid uint64, groupid uint64, eventid *uint64, volunteeringid *uint64) (uint64, error) {
	db := database.DBConn

	// Get position from group or user.
	var lat, lng *float64

	// Try group location first.
	if groupid > 0 {
		type GroupLoc struct {
			Lat *float64
			Lng *float64
		}
		var gl GroupLoc
		db.Raw("SELECT lat, lng FROM `groups` WHERE id = ?", groupid).Scan(&gl)
		lat = gl.Lat
		lng = gl.Lng
	}

	// Fall back to user location.
	if lat == nil && userid > 0 {
		type UserLoc struct {
			Lat *float64
			Lng *float64
		}
		var ul UserLoc
		db.Raw("SELECT lat, lng FROM users WHERE id = ?", userid).Scan(&ul)
		lat = ul.Lat
		lng = ul.Lng
	}

	if lat == nil || lng == nil {
		// Can't create a newsfeed entry without a location.
		return 0, nil
	}

	pos := fmt.Sprintf("ST_GeomFromText('POINT(%f %f)', %d)", *lng, *lat, utils.SRID)

	result := db.Exec(
		fmt.Sprintf("INSERT INTO newsfeed (`type`, userid, groupid, eventid, volunteeringid, position, hidden, deleted, reviewrequired, pinned) "+
			"VALUES (?, ?, ?, ?, ?, %s, NULL, NULL, 0, 0)", pos),
		nfType, userid, groupid, eventid, volunteeringid,
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
