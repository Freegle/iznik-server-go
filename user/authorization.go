package user

import (
	"log"

	"github.com/freegle/iznik-server-go/auth"
	"github.com/freegle/iznik-server-go/database"
)

// IsAdminOrSupport checks if the user has Admin or Support system role.
// Delegates to auth.IsAdminOrSupport to avoid circular imports.
func IsAdminOrSupport(myid uint64) bool {
	return auth.IsAdminOrSupport(myid)
}

// IsModOfGroup checks if the user is a Moderator or Owner of the given group, or is Admin/Support.
func IsModOfGroup(myid uint64, groupid uint64) bool {
	return auth.IsModOfGroup(myid, groupid)
}

// IsModOfUser checks if myid is Admin/Support or a Moderator/Owner of any
// group that targetid also belongs to.
func IsModOfUser(myid, targetid uint64) bool {
	if auth.IsAdminOrSupport(myid) {
		return true
	}
	db := database.DBConn
	var count int64
	result := db.Raw("SELECT COUNT(*) FROM memberships m1 "+
		"INNER JOIN memberships m2 ON m2.groupid = m1.groupid "+
		"WHERE m1.userid = ? AND m2.userid = ? "+
		"AND m1.role IN ('Moderator', 'Owner')",
		myid, targetid).Scan(&count)
	if result.Error != nil {
		log.Printf("Failed to check IsModOfUser for user %d target %d: %v", myid, targetid, result.Error)
		return false
	}
	return count > 0
}

