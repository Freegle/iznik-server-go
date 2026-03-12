package user

import (
	"github.com/freegle/iznik-server-go/auth"
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

