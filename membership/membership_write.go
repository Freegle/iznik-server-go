package membership

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

// PutMembershipsRequest is the body for PUT /memberships (join group).
type PutMembershipsRequest struct {
	Userid  uint64 `json:"userid"`
	Groupid uint64 `json:"groupid"`
	Manual  *bool  `json:"manual"`
}

// PutMemberships handles PUT /memberships - user joins a group.
// FD sends: {userid, groupid, manual}
// Only self-join is supported here (userid must match authenticated user).
func PutMemberships(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req PutMembershipsRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.Groupid == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "groupid is required")
	}

	// Default userid to the authenticated user if not provided.
	userid := req.Userid
	if userid == 0 {
		userid = myid
	}

	// FD only does self-join. Non-self joins require moderator permissions which
	// we leave on v1 for now.
	if userid != myid {
		return fiber.NewError(fiber.StatusForbidden, "Cannot add another user")
	}

	db := database.DBConn

	// Check the group exists.
	var groupExists int64
	db.Raw("SELECT COUNT(*) FROM `groups` WHERE id = ?", req.Groupid).Scan(&groupExists)
	if groupExists == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Group not found")
	}

	// Check if already a member.
	var existingRole string
	db.Raw("SELECT role FROM memberships WHERE userid = ? AND groupid = ?",
		userid, req.Groupid).Scan(&existingRole)

	if existingRole != "" {
		// Already a member - just return success (joining shouldn't demote).
		return c.JSON(fiber.Map{"ret": 0, "status": "Success", "addedto": "Approved"})
	}

	// Check if banned - unban on explicit join.
	var bannedCount int64
	db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND groupid = ? AND collection = 'Banned'",
		userid, req.Groupid).Scan(&bannedCount)
	if bannedCount > 0 {
		db.Exec("DELETE FROM memberships WHERE userid = ? AND groupid = ? AND collection = 'Banned'",
			userid, req.Groupid)
	}

	// Get an email ID for the user.
	var emailid uint64
	db.Raw("SELECT id FROM users_emails WHERE userid = ? ORDER BY preferred DESC, id ASC LIMIT 1",
		userid).Scan(&emailid)

	// Insert membership as approved member.
	db.Exec("INSERT INTO memberships (userid, groupid, role, collection) VALUES (?, ?, 'Member', 'Approved')",
		userid, req.Groupid)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "addedto": "Approved"})
}

// DeleteMembershipsRequest is for DELETE /memberships (leave group).
type DeleteMembershipsRequest struct {
	Userid  uint64 `json:"userid"`
	Groupid uint64 `json:"groupid"`
}

// DeleteMemberships handles DELETE /memberships - user leaves a group.
// FD sends: {userid, groupid}
func DeleteMemberships(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req DeleteMembershipsRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.Groupid == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "groupid is required")
	}

	userid := req.Userid
	if userid == 0 {
		userid = myid
	}

	// FD only does self-leave. Non-self removals require moderator permissions.
	if userid != myid {
		return fiber.NewError(fiber.StatusForbidden, "Cannot remove another user")
	}

	db := database.DBConn

	// Remove the membership.
	result := db.Exec("DELETE FROM memberships WHERE userid = ? AND groupid = ? AND collection = 'Approved'",
		userid, req.Groupid)

	if result.RowsAffected == 0 {
		// Not a member - still return success (idempotent).
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// PatchMembershipsRequest is for PATCH /memberships (update settings).
type PatchMembershipsRequest struct {
	Userid              uint64 `json:"userid"`
	Groupid             uint64 `json:"groupid"`
	Emailfrequency      *int   `json:"emailfrequency"`
	Eventsallowed       *int   `json:"eventsallowed"`
	Volunteeringallowed *int   `json:"volunteeringallowed"`
}

// PatchMemberships handles PATCH /memberships - update membership settings.
// FD sends: {userid, groupid, emailfrequency|eventsallowed|volunteeringallowed}
func PatchMemberships(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req PatchMembershipsRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.Groupid == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "groupid is required")
	}

	userid := req.Userid
	if userid == 0 {
		userid = myid
	}

	// Users can update their own settings. Moderator updates stay on v1.
	if userid != myid {
		return fiber.NewError(fiber.StatusForbidden, "Cannot modify another user's settings")
	}

	db := database.DBConn

	// Verify the membership exists.
	var membershipExists int64
	db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND groupid = ? AND collection = 'Approved'",
		userid, req.Groupid).Scan(&membershipExists)
	if membershipExists == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Not a member of this group")
	}

	// Update whichever settings were provided.
	if req.Emailfrequency != nil {
		db.Exec("UPDATE memberships SET emailfrequency = ? WHERE userid = ? AND groupid = ?",
			*req.Emailfrequency, userid, req.Groupid)
	}

	if req.Eventsallowed != nil {
		db.Exec("UPDATE memberships SET eventsallowed = ? WHERE userid = ? AND groupid = ?",
			*req.Eventsallowed, userid, req.Groupid)
	}

	if req.Volunteeringallowed != nil {
		db.Exec("UPDATE memberships SET volunteeringallowed = ? WHERE userid = ? AND groupid = ?",
			*req.Volunteeringallowed, userid, req.Groupid)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}
