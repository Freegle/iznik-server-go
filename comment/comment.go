package comment

import (
	"strconv"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

type CreateRequest struct {
	Userid  uint64  `json:"userid"`
	Groupid *uint64 `json:"groupid"`
	User1   *string `json:"user1"`
	User2   *string `json:"user2"`
	User3   *string `json:"user3"`
	User4   *string `json:"user4"`
	User5   *string `json:"user5"`
	User6   *string `json:"user6"`
	User7   *string `json:"user7"`
	User8   *string `json:"user8"`
	User9   *string `json:"user9"`
	User10  *string `json:"user10"`
	User11  *string `json:"user11"`
	Flag    bool    `json:"flag"`
}

type PatchRequest struct {
	ID     uint64  `json:"id"`
	User1  *string `json:"user1"`
	User2  *string `json:"user2"`
	User3  *string `json:"user3"`
	User4  *string `json:"user4"`
	User5  *string `json:"user5"`
	User6  *string `json:"user6"`
	User7  *string `json:"user7"`
	User8  *string `json:"user8"`
	User9  *string `json:"user9"`
	User10 *string `json:"user10"`
	User11 *string `json:"user11"`
	Flag   *bool   `json:"flag"`
}

// canModerate checks if the user is a moderator/owner of the group, or admin/support.
func canModerate(myid uint64, groupid *uint64) bool {
	db := database.DBConn

	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)

	if systemrole == "Support" || systemrole == "Admin" {
		return true
	}

	if groupid == nil || *groupid == 0 {
		return false
	}

	var role string
	db.Raw("SELECT role FROM memberships WHERE userid = ? AND groupid = ? AND collection = 'Approved'", myid, *groupid).Scan(&role)

	return role == "Moderator" || role == "Owner"
}

// canModerateComment checks if the user can modify a specific existing comment.
func canModerateComment(myid uint64, commentID uint64) bool {
	db := database.DBConn

	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)

	if systemrole == "Support" || systemrole == "Admin" {
		return true
	}

	var groupid *uint64
	db.Raw("SELECT groupid FROM users_comments WHERE id = ?", commentID).Scan(&groupid)

	if groupid == nil || *groupid == 0 {
		return false
	}

	var role string
	db.Raw("SELECT role FROM memberships WHERE userid = ? AND groupid = ? AND collection = 'Approved'", myid, *groupid).Scan(&role)

	return role == "Moderator" || role == "Owner"
}

// Create handles POST /api/comment
func Create(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req CreateRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.Userid == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "userid is required")
	}

	if !canModerate(myid, req.Groupid) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator of this group")
	}

	db := database.DBConn

	var flag int
	if req.Flag {
		flag = 1
	}

	result := db.Exec(
		"INSERT INTO users_comments (userid, groupid, byuserid, user1, user2, user3, user4, user5, user6, user7, user8, user9, user10, user11, flag) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		req.Userid, req.Groupid, myid,
		req.User1, req.User2, req.User3, req.User4, req.User5,
		req.User6, req.User7, req.User8, req.User9, req.User10,
		req.User11, flag,
	)

	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create comment")
	}

	var id uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&id)

	return c.JSON(fiber.Map{
		"id": id,
	})
}

// Edit handles PATCH /api/comment
func Edit(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req PatchRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "id is required")
	}

	if !canModerateComment(myid, req.ID) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator of this group")
	}

	db := database.DBConn

	db.Exec(
		"UPDATE users_comments SET user1 = ?, user2 = ?, user3 = ?, user4 = ?, user5 = ?, user6 = ?, user7 = ?, user8 = ?, user9 = ?, user10 = ?, user11 = ?, flag = COALESCE(?, flag), byuserid = ?, reviewed = NOW() WHERE id = ?",
		req.User1, req.User2, req.User3, req.User4, req.User5,
		req.User6, req.User7, req.User8, req.User9, req.User10,
		req.User11, req.Flag, myid, req.ID,
	)

	return c.JSON(fiber.Map{
		"success": true,
	})
}

// Delete handles DELETE /api/comment/:id
func Delete(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil || id == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid comment ID")
	}

	if !canModerateComment(myid, id) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator of this group")
	}

	db := database.DBConn
	db.Exec("DELETE FROM users_comments WHERE id = ?", id)

	return c.JSON(fiber.Map{
		"success": true,
	})
}
