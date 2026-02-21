package user

import (
	"github.com/freegle/iznik-server-go/database"
)

// IsAdminOrSupport checks if the user has Admin or Support system role.
func IsAdminOrSupport(myid uint64) bool {
	db := database.DBConn
	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)
	return systemrole == "Support" || systemrole == "Admin"
}

// IsModOfGroup checks if the user is a Moderator or Owner of the given group, or is Admin/Support.
func IsModOfGroup(myid uint64, groupid uint64) bool {
	if IsAdminOrSupport(myid) {
		return true
	}

	if groupid == 0 {
		return false
	}

	db := database.DBConn
	var role string
	db.Raw("SELECT role FROM memberships WHERE userid = ? AND groupid = ?", myid, groupid).Scan(&role)
	return role == "Moderator" || role == "Owner"
}

// IsModOfAnyGroup checks if the user is a Moderator or Owner of any group, or is Admin/Support.
func IsModOfAnyGroup(myid uint64) bool {
	if IsAdminOrSupport(myid) {
		return true
	}

	db := database.DBConn
	var count int64
	db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND role IN ('Moderator', 'Owner')", myid).Scan(&count)
	return count > 0
}
